// Package controllerclient is a typed client for the OpenFSC Controller
// Administration API (served on container port 8444 / service port 9444 over
// mTLS). The operator uses its read endpoints to observe the inways and
// outways that have registered with the Controller.
package controllerclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Option func(*tls.Config) error

func WithClientCertificatePEM(certPEM, keyPEM string) Option {
	return func(c *tls.Config) error {
		cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return fmt.Errorf("parse client keypair PEM: %w", err)
		}
		c.Certificates = append(c.Certificates, cert)
		return nil
	}
}

func WithCACertificatePEM(caPEM string) Option {
	return func(c *tls.Config) error {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return fmt.Errorf("no certificates found in CA PEM")
		}
		c.RootCAs = pool
		return nil
	}
}

// WithInsecureSkipVerify disables server certificate verification (dev only).
func WithInsecureSkipVerify() Option {
	return func(c *tls.Config) error {
		c.InsecureSkipVerify = true
		return nil
	}
}

// WithServerName overrides the name verified against the server certificate, so
// a client dialing a cross-namespace FQDN can still verify against a cert whose
// SAN is the short in-cluster service name.
func WithServerName(name string) Option {
	return func(c *tls.Config) error {
		c.ServerName = name
		return nil
	}
}

// Inway and Outway mirror the Administration API schemas.
type Inway struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Outway struct {
	Name                string   `json:"name"`
	PublicKeyThumbprint string   `json:"public_key_thumbprint"`
	DomainNames         []string `json:"domain_names"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, opts ...Option) (*Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	for _, opt := range opts {
		if err := opt(tlsCfg); err != nil {
			return nil, err
		}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

func (c *Client) ListInways(ctx context.Context) ([]Inway, error) {
	var out struct {
		Inways []Inway `json:"inways"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/inways", nil, &out); err != nil {
		return nil, err
	}
	return out.Inways, nil
}

func (c *Client) ListOutways(ctx context.Context) ([]Outway, error) {
	var out struct {
		Outways []Outway `json:"outways"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/outways", nil, &out); err != nil {
		return nil, err
	}
	return out.Outways, nil
}

// do issues a JSON request to path (relative to the base URL). body, if non-nil,
// is JSON-encoded; out, if non-nil, receives the decoded response. A non-2xx
// status is returned as an error with the body.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
