package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/f0rkz/iptrack/internal/discovery"
	"github.com/f0rkz/iptrack/internal/ipam"
	"github.com/f0rkz/iptrack/web"
)

type Options struct{ DiscoveryWorkers, MaxDiscoveryHosts int }
type server struct {
	store       ipam.Repository
	discoveries *discovery.Manager
}

func New(store ipam.Repository, opts Options) http.Handler {
	s := &server{store: store, discoveries: discovery.NewManager(store, opts.DiscoveryWorkers, opts.MaxDiscoveryHosts)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if err := store.Health(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/stats", s.stats)
	mux.HandleFunc("GET /api/v1/networks", s.listNetworks)
	mux.HandleFunc("POST /api/v1/networks", s.createNetwork)
	mux.HandleFunc("GET /api/v1/networks/{id}", s.getNetwork)
	mux.HandleFunc("PUT /api/v1/networks/{id}", s.updateNetwork)
	mux.HandleFunc("DELETE /api/v1/networks/{id}", s.deleteNetwork)
	mux.HandleFunc("POST /api/v1/networks/{id}/allocate", s.allocate)
	mux.HandleFunc("GET /api/v1/addresses", s.listAddresses)
	mux.HandleFunc("POST /api/v1/addresses", s.createAddress)
	mux.HandleFunc("GET /api/v1/addresses/{id}", s.getAddress)
	mux.HandleFunc("PUT /api/v1/addresses/{id}", s.updateAddress)
	mux.HandleFunc("DELETE /api/v1/addresses/{id}", s.deleteAddress)
	mux.HandleFunc("GET /api/v1/discoveries", s.listDiscoveries)
	mux.HandleFunc("POST /api/v1/discoveries", s.createDiscovery)
	mux.HandleFunc("GET /api/v1/discoveries/{id}", s.getDiscovery)
	assets, _ := fs.Sub(web.Files, ".")
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	return middleware(mux)
}

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) stats(w http.ResponseWriter, _ *http.Request) {
	snapshot, err := s.store.Snapshot()
	if err != nil {
		writeError(w, err)
		return
	}
	discovered := 0
	for _, a := range snapshot.Addresses {
		if a.Status == ipam.StatusDiscovered {
			discovered++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"networks": len(snapshot.Networks), "addresses": len(snapshot.Addresses), "discovered": discovered})
}
func (s *server) listNetworks(w http.ResponseWriter, _ *http.Request) {
	items, err := s.store.Networks()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *server) createNetwork(w http.ResponseWriter, r *http.Request) {
	var in ipam.NetworkInput
	if !decode(w, r, &in) {
		return
	}
	n, err := s.store.CreateNetwork(in)
	respond(w, n, err, http.StatusCreated)
}
func (s *server) getNetwork(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.Network(r.PathValue("id"))
	respond(w, n, err, http.StatusOK)
}
func (s *server) updateNetwork(w http.ResponseWriter, r *http.Request) {
	var in ipam.NetworkInput
	if !decode(w, r, &in) {
		return
	}
	n, err := s.store.UpdateNetwork(r.PathValue("id"), in)
	respond(w, n, err, http.StatusOK)
}
func (s *server) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteNetwork(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) listAddresses(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Addresses(r.URL.Query().Get("network_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *server) createAddress(w http.ResponseWriter, r *http.Request) {
	var in ipam.AddressInput
	if !decode(w, r, &in) {
		return
	}
	a, err := s.store.CreateAddress(in)
	respond(w, a, err, http.StatusCreated)
}
func (s *server) getAddress(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Address(r.PathValue("id"))
	respond(w, a, err, http.StatusOK)
}
func (s *server) updateAddress(w http.ResponseWriter, r *http.Request) {
	var in ipam.AddressInput
	if !decode(w, r, &in) {
		return
	}
	a, err := s.store.UpdateAddress(r.PathValue("id"), in)
	respond(w, a, err, http.StatusOK)
}
func (s *server) deleteAddress(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteAddress(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) allocate(w http.ResponseWriter, r *http.Request) {
	var in ipam.AllocationInput
	if !decode(w, r, &in) {
		return
	}
	in.NetworkID = r.PathValue("id")
	a, err := s.store.Allocate(in)
	respond(w, a, err, http.StatusCreated)
}
func (s *server) listDiscoveries(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.discoveries.List()})
}
func (s *server) createDiscovery(w http.ResponseWriter, r *http.Request) {
	var in discovery.Request
	if !decode(w, r, &in) {
		return
	}
	j, err := s.discoveries.Start(in)
	respond(w, j, err, http.StatusAccepted)
}
func (s *server) getDiscovery(w http.ResponseWriter, r *http.Request) {
	j, err := s.discoveries.Get(r.PathValue("id"))
	respond(w, j, err, http.StatusOK)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "bad_request", "message": err.Error()}})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "bad_request", "message": "request body must contain one JSON object"}})
		return false
	}
	return true
}
func respond(w http.ResponseWriter, value any, err error, success int) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, success, value)
}
func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, ipam.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, ipam.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, ipam.ErrInvalid):
		status, code = http.StatusUnprocessableEntity, "invalid_input"
	}
	message := err.Error()
	if status == 500 {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			fmt.Printf("encode response: %v\n", err)
		}
	}
}
