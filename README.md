# iptrack

[![CI](https://github.com/f0rkz/iptrack/actions/workflows/ci.yml/badge.svg)](https://github.com/f0rkz/iptrack/actions/workflows/ci.yml)

`iptrack` is a small, automation-first IP address manager. It provides a JSON HTTP API, a built-in web interface, active host discovery, and a Terraform provider for managing networks and addresses.

## What is included

- IPv4 and IPv6 network inventory with overlap protection
- Address assignment, reservation, metadata, and atomic next-free allocation
- Asynchronous discovery constrained to a configured network
- TCP port, ICMP echo, reverse-DNS, and local ARP-neighbor probes
- Responsive web dashboard embedded in the server binary
- Terraform resources with full create/read/update/delete and import support
- PostgreSQL persistence with automatic schema setup and pooled connections

## Start it

The simplest startup path is Docker Compose, which runs both iptrack and PostgreSQL 18:

```sh
docker compose up --build -d
```

Then open <http://localhost:8080>. PostgreSQL data is retained in the `postgres-data` named volume.

Released application images are published to GHCR:

```sh
docker pull ghcr.io/f0rkz/iptrack:latest
```

To run the Go service directly, use Go 1.25 or newer and provide a PostgreSQL URL:

```sh
DATABASE_URL='postgres://iptrack:iptrack@localhost:5432/iptrack?sslmode=disable' make run
```

`DATABASE_URL` is required in production. The shown credentials are development defaults from Compose; set `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` in your environment or `.env` before deployment.

## API

All endpoints are under `/api/v1`:

| Method | Path | Purpose |
|---|---|---|
| `GET`, `POST` | `/networks` | List or create networks |
| `GET`, `PUT`, `DELETE` | `/networks/{id}` | Read, update, or delete a network |
| `POST` | `/networks/{id}/allocate` | Atomically allocate the next free address |
| `GET`, `POST` | `/addresses` | List or create addresses (`?network_id=...` filters) |
| `GET`, `PUT`, `DELETE` | `/addresses/{id}` | Read, update, or delete an address |
| `GET`, `POST` | `/discoveries` | List or start discovery jobs |
| `GET` | `/discoveries/{id}` | Poll discovery progress |
| `GET` | `/stats` | Dashboard totals |

Example:

```sh
curl -X POST http://localhost:8080/api/v1/networks \
  -H 'content-type: application/json' \
  -d '{"name":"lab","cidr":"10.20.0.0/24","description":"Home lab"}'
```

Errors have a consistent envelope: `{"error":{"code":"conflict","message":"..."}}`. Invalid input returns 422, missing records return 404, and uniqueness or lifecycle conflicts return 409.

The machine-readable contract is in [`docs/openapi.yaml`](./docs/openapi.yaml).

## Discovery

Start a scan by passing a network ID. Ports and per-probe timeout are optional:

```sh
curl -X POST http://localhost:8080/api/v1/discoveries \
  -H 'content-type: application/json' \
  -d '{"network_id":"net_...","ports":[22,80,443],"timeout_ms":300}'
```

The service checks TCP ports, runs a single ICMP echo when the `ping` executable is available, reads Linux's ARP neighbor table, and performs reverse DNS for reachable hosts. Existing records are refreshed without changing their assigned/reserved status. New active hosts are recorded as `discovered`.

Scans are capped at 4,096 usable addresses and 32 ports per job. Put the service on a network segment that can route to the ranges it scans. Only scan networks you own or are authorized to assess.

## Terraform provider

Build the provider with `make provider`. The module is in [`terraform-provider-iptrack`](./terraform-provider-iptrack) and uses HashiCorp's Terraform Plugin Framework.

```hcl
terraform {
  required_providers {
    iptrack = {
      source = "f0rkz/iptrack"
    }
  }
}

provider "iptrack" {
  endpoint = "http://localhost:8080"
}

resource "iptrack_network" "lab" {
  name        = "lab"
  cidr        = "10.20.0.0/24"
  description = "Home lab"
  tags        = { site = "home" }
}

resource "iptrack_address" "router" {
  network_id = iptrack_network.lab.id
  ip         = "10.20.0.1"
  hostname   = "router.lab"
  status     = "assigned"
}

# Omitting ip atomically claims the next available address.
resource "iptrack_address" "next_worker" {
  network_id = iptrack_network.lab.id
  hostname   = "worker-01.lab"
}
```

Both resources import using their API IDs:

```sh
terraform import iptrack_network.lab net_0123456789abcdef
terraform import iptrack_address.router ip_0123456789abcdef
```

Existing objects can also be read with the `iptrack_network` and `iptrack_address` data sources by setting their `id` attribute.

Release automation and the current Terraform Registry repository-name limitation are documented in [`docs/RELEASING.md`](./docs/RELEASING.md).

`IPTRACK_ENDPOINT` and `IPTRACK_TOKEN` can replace the provider attributes. The token is sent as a bearer token for deployments protected by an authenticating reverse proxy.

## Database and deployment notes

On startup, iptrack connects to PostgreSQL and applies its versioned schema under an advisory lock. Network creation and updates serialize overlap checks, and next-free address allocation uses a per-network transaction lock so competing automation cannot receive the same address. Foreign keys prevent deletion of networks that still contain addresses.

Back up the `postgres-data` volume using normal PostgreSQL backup tooling. The application intentionally has no built-in identity system, so do not expose it directly to an untrusted network: place it behind TLS and an authenticating reverse proxy or private access gateway. Discovery jobs remain in memory; inventory and discovered host metadata are durable, but job progress history resets when the service restarts.
