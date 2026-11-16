// Package discovery reads the document a Fundament installation publishes at
// /.well-known/fundament.json on its root host, which names the URL of each of
// its APIs. Clients configure the root URL and discover the rest.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Path is where an installation serves its discovery document.
const Path = "/.well-known/fundament.json"

const (
	maxBodyBytes   = 64 << 10
	maxRedirects   = 10
	defaultTimeout = 10 * time.Second
)

// ErrNotFound means the installation serves no discovery document.
var ErrNotFound = errors.New("no discovery document")

// Document names the URLs of one installation. An empty field means the
// installation does not offer that API.
type Document struct {
	// Issuer is the root URL the document is served from.
	Issuer                 string `json:"issuer"`
	OrganizationAPI        string `json:"organization_api,omitempty"`
	AuthnAPI               string `json:"authn_api,omitempty"`
	KubeAPIProxy           string `json:"kube_api_proxy,omitempty"`
	MarketplaceCatalogAPI  string `json:"marketplace_catalog_api,omitempty"`
	MarketplaceRegistryAPI string `json:"marketplace_registry_api,omitempty"`
}

// Fetch reads and validates the discovery document of the installation at
// baseURL. No credentials are sent. A nil httpClient uses a default one.
func Fetch(ctx context.Context, httpClient *http.Client, baseURL string) (*Document, error) {
	base, err := parseBase(baseURL)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: defaultTimeout}
	if httpClient != nil {
		copied := *httpClient
		client = &copied
		// Cookies of an authenticated client are credentials too.
		client.Jar = nil
	}
	// The document decides where the API key is sent, so it must come from
	// the configured origin and nowhere else.
	baseOrigin := canonicalOrigin(base)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Setting CheckRedirect replaces the client's default limit.
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		// net/http turns userinfo into a Basic Authorization header.
		if req.URL.User != nil {
			return errors.New("redirect with credentials refused")
		}
		if canonicalOrigin(req.URL) != baseOrigin {
			return fmt.Errorf("redirect to another origin (%s) refused", canonicalOrigin(req.URL))
		}
		return nil
	}

	docURL := base.String() + Path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", docURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing to do about a failed close of a read body

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: %w", docURL, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %s", docURL, resp.Status)
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, fmt.Errorf("%s: content type %q is not application/json", docURL, resp.Header.Get("Content-Type"))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", docURL, err)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("%s: document is larger than %d bytes", docURL, maxBodyBytes)
	}

	doc := &Document{}
	err = json.Unmarshal(body, doc)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", docURL, err)
	}

	issuer, err := url.Parse(strings.TrimRight(doc.Issuer, "/"))
	if err != nil || checkOrigin(issuer, true) != nil || canonicalOrigin(issuer) != baseOrigin || issuer.Path != "" {
		return nil, fmt.Errorf("%s: issuer %q does not match %q", docURL, doc.Issuer, base.String())
	}

	// Plain http to this machine is only for a local installation: a remote
	// document must not point the API key at a local listener.
	allowLoopbackHTTP := isLoopback(base.Hostname())

	fields := map[string]*string{
		"organization_api":         &doc.OrganizationAPI,
		"authn_api":                &doc.AuthnAPI,
		"kube_api_proxy":           &doc.KubeAPIProxy,
		"marketplace_catalog_api":  &doc.MarketplaceCatalogAPI,
		"marketplace_registry_api": &doc.MarketplaceRegistryAPI,
	}
	for name, value := range fields {
		if *value == "" {
			continue
		}
		u, err := parseServiceURL(*value, allowLoopbackHTTP)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", docURL, name, err)
		}
		*value = u
	}

	return doc, nil
}

// parseBase validates the root URL of an installation: an origin without a
// path, since well-known URIs exist only at the top of a host (RFC 8615).
func parseBase(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid installation URL %q: %w", raw, err)
	}
	err = checkOrigin(u, true)
	if err != nil {
		return nil, fmt.Errorf("invalid installation URL %q: %w", raw, err)
	}
	if u.Path != "" {
		return nil, fmt.Errorf("invalid installation URL %q: must not have a path", raw)
	}
	return u, nil
}

// parseServiceURL validates a service URL of the document. Services may live
// on other hosts than the root, since the chart gives each API its own
// subdomain; the configured root is the trust anchor for all of them.
func parseServiceURL(raw string, allowLoopbackHTTP bool) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	err = checkOrigin(u, allowLoopbackHTTP)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// checkOrigin requires https, or http to this machine for local development
// when allowLoopbackHTTP is set.
func checkOrigin(u *url.URL, allowLoopbackHTTP bool) error {
	if u.Host == "" {
		return errors.New("must be an absolute URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("must not have credentials, a query or a fragment")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowLoopbackHTTP && isLoopback(u.Hostname()) {
			return nil
		}
		return errors.New("http is only allowed for localhost, from a localhost installation")
	default:
		return errors.New("scheme must be https")
	}
}

func isLoopback(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// canonicalOrigin is the scheme, host and port of u in a form that compares
// equal for equivalent URLs: lower case, without the scheme's default port.
func canonicalOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" || (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}
