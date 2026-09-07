package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/f0rkz/iptrack/internal/ipam"
)

func TestNetworkAndAddressAPI(t *testing.T) {
	store, _ := ipam.Open(filepath.Join(t.TempDir(), "state.json"))
	handler := New(store, Options{DiscoveryWorkers: 2, MaxDiscoveryHosts: 16})
	network := request[ipam.Network](t, handler, "POST", "/api/v1/networks", map[string]any{"name": "test", "cidr": "198.51.100.0/29"}, http.StatusCreated)
	address := request[ipam.Address](t, handler, "POST", "/api/v1/networks/"+network.ID+"/allocate", map[string]any{"hostname": "node-1"}, http.StatusCreated)
	if address.IP != "198.51.100.1" || address.NetworkID != network.ID {
		t.Fatalf("unexpected allocation: %#v", address)
	}
	address = request[ipam.Address](t, handler, "PUT", "/api/v1/addresses/"+address.ID, map[string]any{"network_id": network.ID, "ip": address.IP, "hostname": "node-edited", "mac": "AA-BB-CC-DD-EE-FF", "status": "assigned"}, http.StatusOK)
	if address.Hostname != "node-edited" || address.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("identity update failed: %#v", address)
	}
	request[map[string]any](t, handler, "PUT", "/api/v1/addresses/"+address.ID, map[string]any{"network_id": network.ID, "ip": address.IP, "mac": "invalid", "status": "assigned"}, http.StatusUnprocessableEntity)
	request[map[string]any](t, handler, "POST", "/api/v1/addresses/missing/probe", nil, http.StatusNotFound)
	request[map[string]any](t, handler, "POST", "/api/v1/addresses", map[string]any{"network_id": network.ID, "ip": "203.0.113.2"}, http.StatusUnprocessableEntity)
	request[map[string]any](t, handler, "DELETE", "/api/v1/networks/"+network.ID, nil, http.StatusConflict)
	request[map[string]any](t, handler, "DELETE", "/api/v1/addresses/"+address.ID, nil, http.StatusNoContent)
	request[map[string]any](t, handler, "DELETE", "/api/v1/networks/"+network.ID, nil, http.StatusNoContent)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("iptrack")) {
		t.Fatalf("web UI unavailable: %d", recorder.Code)
	}
	style := httptest.NewRecorder()
	handler.ServeHTTP(style, httptest.NewRequest("GET", "/identity.css", nil))
	if style.Code != http.StatusOK || !bytes.Contains(style.Body.Bytes(), []byte(".row-actions")) {
		t.Fatalf("identity UI styles unavailable: %d", style.Code)
	}
}

func request[T any](t *testing.T, handler http.Handler, method, path string, body any, status int) T {
	t.Helper()
	raw, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewReader(raw)))
	if recorder.Code != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, recorder.Code, status, recorder.Body.String())
	}
	var out T
	_ = json.Unmarshal(recorder.Body.Bytes(), &out)
	return out
}
