// Package proxy queries Go module proxies for version publish times.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// Client queries Go module proxies for version metadata.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a proxy client with the given HTTP client.
// If httpClient is nil, a default client with 10s timeout is used.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

type versionInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
}

// GetVersionTime queries the proxy chain for the publish time of a module version.
func (c *Client) GetVersionTime(ctx context.Context, proxyURL, module, version string) (time.Time, error) {
	proxies := parseProxyChain(proxyURL)

	var lastErr error
	for _, p := range proxies {
		if p.url == "direct" || p.url == "off" {
			break
		}

		t, err := c.queryProxy(ctx, p.url, module, version)
		if err == nil {
			return t, nil
		}
		lastErr = err

		// Comma-separated: only fall through on 404/410
		// Pipe-separated: fall through on any error
		if !p.lenient && !isNotFound(err) {
			return time.Time{}, err
		}
	}

	if lastErr != nil {
		return time.Time{}, fmt.Errorf("proxy error: %w", lastErr)
	}
	return time.Time{}, fmt.Errorf("no proxy available for %s@%s", module, version)
}

func (c *Client) queryProxy(ctx context.Context, baseURL, module, version string) (time.Time, error) {
	encodedPath := EncodePath(module)
	encodedVersion := EncodePath(version)
	url := fmt.Sprintf("%s/%s/@v/%s.info", baseURL, encodedPath, encodedVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return time.Time{}, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("querying proxy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return time.Time{}, &NotFoundError{Module: module, Version: version, StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("proxy returned HTTP %d for %s@%s", resp.StatusCode, module, version)
	}

	var info versionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return time.Time{}, fmt.Errorf("decoding response: %w", err)
	}

	if info.Time.IsZero() {
		return time.Time{}, fmt.Errorf("proxy returned no publish time for %s@%s", module, version)
	}

	return info.Time, nil
}

// NotFoundError indicates a module version was not found on the proxy.
type NotFoundError struct {
	Module     string
	Version    string
	StatusCode int
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("module %s@%s not found (HTTP %d)", e.Module, e.Version, e.StatusCode)
}

func isNotFound(err error) bool {
	var nfe *NotFoundError
	return errors.As(err, &nfe)
}

type proxyEntry struct {
	url     string
	lenient bool // pipe-separated entries are lenient (fall through on any error)
}

// parseProxyChain expands a GOPROXY value into an ordered list of entries
// to try, marking each entry's failure semantics.
//
// Go's GOPROXY supports two separators with different fall-through rules:
//   - comma  (a,b): only fall through to b on 404/410, abort on any other error
//   - pipe   (a|b): fall through to b on any error
//
// We split on pipe first, then comma within each pipe segment. The boundary
// between two pipe segments is the only place where pipe semantics matter:
// the last comma entry of a pipe group is the one whose failure must roll
// over into the next pipe segment, so it gets marked lenient. (For the very
// last pipe group there is nothing to roll over into, so the lenient flag on
// its trailing entry is harmless.)
func parseProxyChain(goproxy string) []proxyEntry {
	if goproxy == "" {
		goproxy = "https://proxy.golang.org,direct"
	}

	hasPipe := strings.Contains(goproxy, "|")
	var entries []proxyEntry
	for _, pipePart := range strings.Split(goproxy, "|") {
		commaParts := strings.Split(pipePart, ",")
		for i, part := range commaParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lenient := hasPipe && i == len(commaParts)-1
			entries = append(entries, proxyEntry{url: part, lenient: lenient})
		}
	}
	return entries
}

// EncodePath encodes a module path for use in proxy URLs.
// Uppercase letters are encoded as ! followed by the lowercase letter.
func EncodePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
