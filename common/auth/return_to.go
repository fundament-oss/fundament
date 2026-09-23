package auth

import (
	"log/slog"
	"net/url"
	"slices"
	"strings"
)

// A login endpoint that takes a `return_to` and redirects the browser to it
// once the session cookie is set would, unchecked, be an open redirect on an
// authentication endpoint. ReturnOrigins is the check: a return URL is only
// honoured when its origin is one the deployment serves. authn-api and
// dcim-authn-api both use it, so a hardening made here lands in both.

// NormalizeOrigin reduces a URL to a comparable scheme://host[:port] origin.
// It reports false for anything that is not an absolute http(s) URL, and for
// URLs carrying userinfo — a redirect target is not a place to hand out
// credentials, and `https://console.example.com@evil.example` reads like one
// origin while naming another.
func NormalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" {
		return "", false
	}

	host := strings.ToLower(parsed.Host)
	// A default port and no port name the same origin. Both sides of the
	// comparison run through here, so stripping it settles the mismatch
	// between a configured "https://console.example.com" and a return URL
	// that spells out ":443".
	switch {
	case scheme == "http" && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	case scheme == "https" && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	}

	return scheme + "://" + host, true
}

// ReturnOrigins is an allowlist of post-login redirect origins, held in the
// form NormalizeOrigin produces.
type ReturnOrigins []string

// NewReturnOrigins folds a configured allowlist into the comparable form
// Allows matches against, once at startup rather than on every login.
//
// An entry that is not an absolute http(s) URL can never match anything, so it
// is dropped — and logged, because the symptom is otherwise a refused login
// whose only line names the caller's return_to and says nothing about the
// configuration that refused it. An empty entry is the trailing comma in a
// comma-separated environment variable and is not worth a line.
func NewReturnOrigins(logger *slog.Logger, configured []string) ReturnOrigins {
	origins := make(ReturnOrigins, 0, len(configured))

	for _, entry := range configured {
		if strings.TrimSpace(entry) == "" {
			continue
		}

		origin, ok := NormalizeOrigin(entry)
		if !ok {
			logger.Error("ignoring allowed return origin that is not an absolute http(s) URL",
				"origin", entry)
			continue
		}

		if !slices.Contains(origins, origin) {
			origins = append(origins, origin)
		}
	}

	if len(origins) == 0 {
		logger.Error("no usable allowed return origins: every login naming a return_to will be refused")
	}

	return origins
}

// Allows reports whether returnTo is a trusted post-login redirect target.
func (o ReturnOrigins) Allows(returnTo string) bool {
	origin, ok := NormalizeOrigin(returnTo)
	if !ok {
		return false
	}

	return slices.Contains(o, origin)
}
