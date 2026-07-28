package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version integer PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS networks (
  id text PRIMARY KEY,
  name text NOT NULL UNIQUE CHECK (btrim(name) <> ''),
  cidr cidr NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  tags jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS addresses (
  id text PRIMARY KEY,
  network_id text NOT NULL REFERENCES networks(id) ON DELETE RESTRICT,
  ip inet NOT NULL UNIQUE,
  hostname text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('reserved', 'assigned', 'discovered')),
  mac text NOT NULL DEFAULT '',
  vendor text NOT NULL DEFAULT '',
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS addresses_network_id_idx ON addresses(network_id);
CREATE INDEX IF NOT EXISTS addresses_last_seen_at_idx ON addresses(last_seen_at) WHERE last_seen_at IS NOT NULL;
INSERT INTO schema_migrations(version) VALUES (1) ON CONFLICT DO NOTHING;
`

type PostgresStore struct{ db *sql.DB }

var _ Repository = (*PostgresStore)(nil)

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	store := &PostgresStore{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Health() error {
	ctx, cancel := queryContext()
	defer cancel()
	return dbError(s.db.PingContext(ctx))
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(479011)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply database schema: %w", err)
	}
	return tx.Commit()
}

func (s *PostgresStore) Snapshot() (Snapshot, error) {
	networks, err := s.Networks()
	if err != nil {
		return Snapshot{}, err
	}
	addresses, err := s.Addresses("")
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Networks: networks, Addresses: addresses}, nil
}

func (s *PostgresStore) Networks() ([]Network, error) {
	ctx, cancel := queryContext()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,cidr::text,description,tags,created_at,updated_at FROM networks ORDER BY name`)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	out := []Network{}
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, dbError(rows.Err())
}

func (s *PostgresStore) Network(id string) (Network, error) {
	ctx, cancel := queryContext()
	defer cancel()
	return scanNetwork(s.db.QueryRowContext(ctx, `SELECT id,name,cidr::text,description,tags,created_at,updated_at FROM networks WHERE id=$1`, id))
}

func (s *PostgresStore) CreateNetwork(in NetworkInput) (Network, error) {
	in.Name = strings.TrimSpace(in.Name)
	prefix, err := parsePrefix(in.CIDR)
	if err != nil || in.Name == "" {
		return Network{}, fmt.Errorf("%w: name and a valid CIDR are required", ErrInvalid)
	}
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Network{}, dbError(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(479012)`); err != nil {
		return Network{}, dbError(err)
	}
	var overlap bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM networks WHERE cidr && $1::cidr)`, prefix.String()).Scan(&overlap); err != nil {
		return Network{}, dbError(err)
	}
	if overlap {
		return Network{}, fmt.Errorf("%w: network CIDR overlaps an existing network", ErrConflict)
	}
	now := time.Now().UTC()
	tags, _ := json.Marshal(nonNilMap(in.Tags))
	n, err := scanNetwork(tx.QueryRowContext(ctx, `INSERT INTO networks(id,name,cidr,description,tags,created_at,updated_at) VALUES($1,$2,$3::cidr,$4,$5::jsonb,$6,$6) RETURNING id,name,cidr::text,description,tags,created_at,updated_at`, newID("net"), in.Name, prefix.String(), strings.TrimSpace(in.Description), tags, now))
	if err != nil {
		return Network{}, err
	}
	if err = tx.Commit(); err != nil {
		return Network{}, dbError(err)
	}
	return n, nil
}

func (s *PostgresStore) UpdateNetwork(id string, in NetworkInput) (Network, error) {
	in.Name = strings.TrimSpace(in.Name)
	prefix, err := parsePrefix(in.CIDR)
	if err != nil || in.Name == "" {
		return Network{}, fmt.Errorf("%w: name and a valid CIDR are required", ErrInvalid)
	}
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Network{}, dbError(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(479012)`); err != nil {
		return Network{}, dbError(err)
	}
	var overlap bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM networks WHERE id<>$1 AND cidr && $2::cidr)`, id, prefix.String()).Scan(&overlap); err != nil {
		return Network{}, dbError(err)
	}
	if overlap {
		return Network{}, fmt.Errorf("%w: network CIDR overlaps an existing network", ErrConflict)
	}
	var excluded string
	err = tx.QueryRowContext(ctx, `SELECT host(ip) FROM addresses WHERE network_id=$1 AND NOT (ip <<= $2::cidr) LIMIT 1`, id, prefix.String()).Scan(&excluded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Network{}, dbError(err)
	}
	if excluded != "" {
		return Network{}, fmt.Errorf("%w: new CIDR excludes existing address %s", ErrConflict, excluded)
	}
	tags, _ := json.Marshal(nonNilMap(in.Tags))
	now := time.Now().UTC()
	n, err := scanNetwork(tx.QueryRowContext(ctx, `UPDATE networks SET name=$2,cidr=$3::cidr,description=$4,tags=$5::jsonb,updated_at=$6 WHERE id=$1 RETURNING id,name,cidr::text,description,tags,created_at,updated_at`, id, in.Name, prefix.String(), strings.TrimSpace(in.Description), tags, now))
	if err != nil {
		return Network{}, err
	}
	if err = tx.Commit(); err != nil {
		return Network{}, dbError(err)
	}
	return n, nil
}

func (s *PostgresStore) DeleteNetwork(id string) error {
	ctx, cancel := queryContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM networks WHERE id=$1`, id)
	if err != nil {
		return dbError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return dbError(err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) Addresses(networkID string) ([]Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	query := `SELECT id,network_id,host(ip),hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at FROM addresses`
	args := []any{}
	if networkID != "" {
		query += ` WHERE network_id=$1`
		args = append(args, networkID)
	}
	query += ` ORDER BY ip`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	out := []Address{}
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, dbError(rows.Err())
}

func (s *PostgresStore) Address(id string) (Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	return scanAddress(s.db.QueryRowContext(ctx, `SELECT id,network_id,host(ip),hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at FROM addresses WHERE id=$1`, id))
}

func (s *PostgresStore) CreateAddress(in AddressInput) (Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Address{}, dbError(err)
	}
	defer tx.Rollback()
	a, err := createAddressSQL(ctx, tx, in, newID("ip"))
	if err != nil {
		return Address{}, err
	}
	if err = tx.Commit(); err != nil {
		return Address{}, dbError(err)
	}
	return a, nil
}

func (s *PostgresStore) UpdateAddress(id string, in AddressInput) (Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Address{}, dbError(err)
	}
	defer tx.Rollback()
	if in.NetworkID == "" {
		if err := tx.QueryRowContext(ctx, `SELECT network_id FROM addresses WHERE id=$1`, id).Scan(&in.NetworkID); err != nil {
			return Address{}, dbError(err)
		}
	}
	if err := validateAddressSQL(ctx, tx, in); err != nil {
		return Address{}, err
	}
	if in.Status == "" {
		in.Status = StatusAssigned
	}
	metadata, _ := json.Marshal(nonNilMap(in.Metadata))
	now := time.Now().UTC()
	a, err := scanAddress(tx.QueryRowContext(ctx, `UPDATE addresses SET network_id=$2,ip=$3::inet,hostname=$4,status=$5,mac=$6,vendor=$7,metadata=$8::jsonb,updated_at=$9 WHERE id=$1 RETURNING id,network_id,host(ip),hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at`, id, in.NetworkID, strings.TrimSpace(in.IP), strings.TrimSpace(in.Hostname), in.Status, strings.ToLower(strings.TrimSpace(in.MAC)), strings.TrimSpace(in.Vendor), metadata, now))
	if err != nil {
		return Address{}, err
	}
	if err = tx.Commit(); err != nil {
		return Address{}, dbError(err)
	}
	return a, nil
}

func (s *PostgresStore) DeleteAddress(id string) error {
	ctx, cancel := queryContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM addresses WHERE id=$1`, id)
	if err != nil {
		return dbError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return dbError(err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) Allocate(in AllocationInput) (Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Address{}, dbError(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, in.NetworkID); err != nil {
		return Address{}, dbError(err)
	}
	var cidr string
	if err = tx.QueryRowContext(ctx, `SELECT cidr::text FROM networks WHERE id=$1`, in.NetworkID).Scan(&cidr); err != nil {
		return Address{}, dbError(err)
	}
	prefix, _ := netip.ParsePrefix(cidr)
	used := map[netip.Addr]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT host(ip) FROM addresses WHERE network_id=$1`, in.NetworkID)
	if err != nil {
		return Address{}, dbError(err)
	}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return Address{}, dbError(err)
		}
		ip, _ := netip.ParseAddr(raw)
		used[ip] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Address{}, dbError(err)
	}
	rows.Close()
	for ip := prefix.Addr().Next(); ip.IsValid() && prefix.Contains(ip); ip = ip.Next() {
		if isBoundary(prefix, ip) || used[ip] {
			continue
		}
		a, err := createAddressSQL(ctx, tx, AddressInput{NetworkID: in.NetworkID, IP: ip.String(), Hostname: in.Hostname, Status: in.Status, Metadata: in.Metadata}, newID("ip"))
		if err != nil {
			return Address{}, err
		}
		if err = tx.Commit(); err != nil {
			return Address{}, dbError(err)
		}
		return a, nil
	}
	return Address{}, fmt.Errorf("%w: network has no available addresses", ErrConflict)
}

func (s *PostgresStore) RecordDiscovery(networkID, ip, hostname, mac string, metadata map[string]string) (Address, error) {
	ctx, cancel := queryContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Address{}, dbError(err)
	}
	defer tx.Rollback()
	in := AddressInput{NetworkID: networkID, IP: ip, Hostname: hostname, Status: StatusDiscovered, MAC: mac, Metadata: metadata}
	if err := validateAddressSQL(ctx, tx, in); err != nil {
		return Address{}, err
	}
	now := time.Now().UTC()
	raw, _ := json.Marshal(nonNilMap(metadata))
	a, err := scanAddress(tx.QueryRowContext(ctx, `INSERT INTO addresses(id,network_id,ip,hostname,status,mac,metadata,last_seen_at,created_at,updated_at) VALUES($1,$2,$3::inet,$4,'discovered',$5,$6::jsonb,$7,$7,$7) ON CONFLICT(ip) DO UPDATE SET hostname=COALESCE(NULLIF(EXCLUDED.hostname,''),addresses.hostname),mac=COALESCE(NULLIF(EXCLUDED.mac,''),addresses.mac),metadata=addresses.metadata||EXCLUDED.metadata,last_seen_at=EXCLUDED.last_seen_at,updated_at=EXCLUDED.updated_at RETURNING id,network_id,host(ip),hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at`, newID("ip"), networkID, ip, strings.TrimSpace(hostname), strings.ToLower(strings.TrimSpace(mac)), raw, now))
	if err != nil {
		return Address{}, err
	}
	if err = tx.Commit(); err != nil {
		return Address{}, dbError(err)
	}
	return a, nil
}

type sqlScanner interface{ Scan(...any) error }

func createAddressSQL(ctx context.Context, tx *sql.Tx, in AddressInput, id string) (Address, error) {
	if err := validateAddressSQL(ctx, tx, in); err != nil {
		return Address{}, err
	}
	if in.Status == "" {
		in.Status = StatusAssigned
	}
	now := time.Now().UTC()
	metadata, _ := json.Marshal(nonNilMap(in.Metadata))
	var lastSeen any
	if in.Status == StatusDiscovered {
		lastSeen = now
	}
	return scanAddress(tx.QueryRowContext(ctx, `INSERT INTO addresses(id,network_id,ip,hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at) VALUES($1,$2,$3::inet,$4,$5,$6,$7,$8::jsonb,$9,$10,$10) RETURNING id,network_id,host(ip),hostname,status,mac,vendor,metadata,last_seen_at,created_at,updated_at`, id, in.NetworkID, strings.TrimSpace(in.IP), strings.TrimSpace(in.Hostname), in.Status, strings.ToLower(strings.TrimSpace(in.MAC)), strings.TrimSpace(in.Vendor), metadata, lastSeen, now))
}

func validateAddressSQL(ctx context.Context, tx *sql.Tx, in AddressInput) error {
	var cidr string
	if err := tx.QueryRowContext(ctx, `SELECT cidr::text FROM networks WHERE id=$1`, in.NetworkID).Scan(&cidr); err != nil {
		return dbError(err)
	}
	prefix, _ := netip.ParsePrefix(cidr)
	ip, err := netip.ParseAddr(strings.TrimSpace(in.IP))
	if err != nil || !prefix.Contains(ip) || ip.IsUnspecified() || ip.IsMulticast() || isBoundary(prefix, ip) {
		return fmt.Errorf("%w: IP must be a usable address in %s", ErrInvalid, cidr)
	}
	if in.Status != "" && !validStatus(in.Status) {
		return fmt.Errorf("%w: invalid address status", ErrInvalid)
	}
	return nil
}

func scanNetwork(row sqlScanner) (Network, error) {
	var n Network
	var tags []byte
	if err := row.Scan(&n.ID, &n.Name, &n.CIDR, &n.Description, &tags, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return Network{}, dbError(err)
	}
	if err := json.Unmarshal(tags, &n.Tags); err != nil {
		return Network{}, fmt.Errorf("decode network tags: %w", err)
	}
	return n, nil
}

func scanAddress(row sqlScanner) (Address, error) {
	var a Address
	var status string
	var metadata []byte
	var lastSeen sql.NullTime
	if err := row.Scan(&a.ID, &a.NetworkID, &a.IP, &a.Hostname, &status, &a.MAC, &a.Vendor, &metadata, &lastSeen, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return Address{}, dbError(err)
	}
	a.Status = AddressStatus(status)
	if lastSeen.Valid {
		value := lastSeen.Time
		a.LastSeenAt = &value
	}
	if err := json.Unmarshal(metadata, &a.Metadata); err != nil {
		return Address{}, fmt.Errorf("decode address metadata: %w", err)
	}
	return a, nil
}

func dbError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: record already exists", ErrConflict)
		case "23503":
			return fmt.Errorf("%w: record is still referenced", ErrConflict)
		case "23514", "22P02":
			return fmt.Errorf("%w: %s", ErrInvalid, pgErr.Message)
		}
	}
	return err
}
func queryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
func nonNilMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	return in
}
