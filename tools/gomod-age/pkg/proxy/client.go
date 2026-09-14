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

const (
	// defaultTimeout bounds a single request. The public proxy is occasionally
	// slow to send headers, so this is generous rather than tight.
	defaultTimeout = 20 * time.Second
	// defaultMaxAttempts is the number of tries per proxy, including the first.
	defaultMaxAttempts = 3
	// defaultRetryDelay is the base backoff, doubled after each failed attempt.
	defaultRetryDelay = 500 * time.Millisecond
)

// Options configures a Client. Zero values fall back to defaults.
type Options struct {
	HTTPClient  *http.Client
	MaxAttempts int
	RetryDelay  time.Duration
}

// Client queries Go module proxies for version metadata.
type Client struct {
	httpClient  *http.Client
	maxAttempts int
	retryDelay  time.Duration
}

// NewClient creates a proxy client from opts, filling in defaults.
func NewClient(opts Options) *Client {
	c := &Client{
		httpClient:  opts.HTTPClient,
		maxAttempts: opts.MaxAttempts,
		retryDelay:  opts.RetryDelay,
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: defaultTimeout}
	}
	if c.maxAttempts < 1 {
		c.maxAttempts = defaultMaxAttempts
	}
	if c.retryDelay <= 0 {
		c.retryDelay = defaultRetryDelay
	}
	return c
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

		t, err := c.queryProxyWithRetry(ctx, p.url, module, version)
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

// queryProxyWithRetry retries transient failures (network errors, timeouts,
// 429 and 5xx) with exponential backoff. A single slow response from the
// public proxy should not fail an entire CI run.
func (c *Client) queryProxyWithRetry(ctx context.Context, baseURL, module, version string) (time.Time, error) {
	delay := c.retryDelay
	var lastErr error
	for attempt := 1; ; attempt++ {
		t, err := c.queryProxy(ctx, baseURL, module, version)
		if err == nil {
			return t, nil
		}
		lastErr = err

		if attempt >= c.maxAttempts || !isRetryable(err) {
			break
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return time.Time{}, lastErr
		case <-timer.C:
		}
		delay *= 2
	}

	if c.maxAttempts > 1 && isRetryable(lastErr) {
		return time.Time{}, fmt.Errorf("after %d attempts: %w", c.maxAttempts, lastErr)
	}
	return time.Time{}, lastErr
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
		// A cancelled caller context is final; anything else is a transient
		// transport failure worth another try.
		if ctx.Err() != nil {
			return time.Time{}, fmt.Errorf("querying proxy: %w", err)
		}
		return time.Time{}, &retryableError{err: fmt.Errorf("querying proxy: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return time.Time{}, &NotFoundError{Module: module, Version: version, StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("proxy returned HTTP %d for %s@%s", resp.StatusCode, module, version)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return time.Time{}, &retryableError{err: err}
		}
		return time.Time{}, err
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

// retryableError marks a failure that may succeed on a later attempt.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var re *retryableError
	return errors.As(err, &re)
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
