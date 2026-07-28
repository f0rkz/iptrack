package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	endpoint, token string
	http            *http.Client
}
type Network struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	CIDR        string            `json:"cidr"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
}
type Address struct {
	ID        string            `json:"id,omitempty"`
	NetworkID string            `json:"network_id"`
	IP        string            `json:"ip"`
	Hostname  string            `json:"hostname"`
	Status    string            `json:"status"`
	MAC       string            `json:"mac"`
	Vendor    string            `json:"vendor"`
	Metadata  map[string]string `json:"metadata"`
}

func New(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("endpoint must be an absolute HTTP(S) URL")
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), token: token, http: http.DefaultClient}, nil
}
func (c *Client) Network(ctx context.Context, id string) (Network, error) {
	var out Network
	err := c.do(ctx, "GET", "/api/v1/networks/"+url.PathEscape(id), nil, &out)
	return out, err
}
func (c *Client) CreateNetwork(ctx context.Context, in Network) (Network, error) {
	var out Network
	in.ID = ""
	err := c.do(ctx, "POST", "/api/v1/networks", in, &out)
	return out, err
}
func (c *Client) UpdateNetwork(ctx context.Context, id string, in Network) (Network, error) {
	var out Network
	in.ID = ""
	err := c.do(ctx, "PUT", "/api/v1/networks/"+url.PathEscape(id), in, &out)
	return out, err
}
func (c *Client) DeleteNetwork(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/networks/"+url.PathEscape(id), nil, nil)
}
func (c *Client) Address(ctx context.Context, id string) (Address, error) {
	var out Address
	err := c.do(ctx, "GET", "/api/v1/addresses/"+url.PathEscape(id), nil, &out)
	return out, err
}
func (c *Client) CreateAddress(ctx context.Context, in Address) (Address, error) {
	var out Address
	in.ID = ""
	err := c.do(ctx, "POST", "/api/v1/addresses", in, &out)
	return out, err
}
func (c *Client) AllocateAddress(ctx context.Context, in Address) (Address, error) {
	var out Address
	body := struct {
		Hostname string            `json:"hostname,omitempty"`
		Status   string            `json:"status,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}{Hostname: in.Hostname, Status: in.Status, Metadata: in.Metadata}
	err := c.do(ctx, "POST", "/api/v1/networks/"+url.PathEscape(in.NetworkID)+"/allocate", body, &out)
	return out, err
}
func (c *Client) UpdateAddress(ctx context.Context, id string, in Address) (Address, error) {
	var out Address
	in.ID = ""
	err := c.do(ctx, "PUT", "/api/v1/addresses/"+url.PathEscape(id), in, &out)
	return out, err
}
func (c *Client) DeleteAddress(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/addresses/"+url.PathEscape(id), nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var envelope struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&envelope)
		if envelope.Error.Message == "" {
			envelope.Error.Message = resp.Status
		}
		return &APIError{StatusCode: resp.StatusCode, Message: envelope.Error.Message}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string { return e.Message }
