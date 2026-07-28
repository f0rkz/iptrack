package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRequestPayloadsExcludeComputedFields(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		if _, present := body["id"]; present {
			t.Errorf("%s payload included computed id", r.URL.Path)
		}
		var response string
		switch r.URL.Path {
		case "/api/v1/networks":
			response = `{"id":"net_1","name":"lab","cidr":"10.0.0.0/24"}`
		case "/api/v1/networks/net_1/allocate":
			for _, forbidden := range []string{"network_id", "ip", "mac", "vendor"} {
				if _, present := body[forbidden]; present {
					t.Errorf("allocation payload included %s", forbidden)
				}
			}
			response = `{"id":"ip_1","network_id":"net_1","ip":"10.0.0.1","status":"assigned"}`
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
	})}
	c := &Client{endpoint: "http://iptrack.test", http: httpClient}
	if _, err := c.CreateNetwork(context.Background(), Network{ID: "computed", Name: "lab", CIDR: "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AllocateAddress(context.Background(), Address{ID: "computed", NetworkID: "net_1", Hostname: "node", Status: "assigned", MAC: "not-for-this-endpoint"}); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
