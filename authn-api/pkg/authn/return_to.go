package authn

import (
	"net/url"
	"strings"
)

// The login endpoint takes a `return_to` and the callback redirects the browser
// to it once the session cookie is set, which is what lets a surface other than
// the console — the marketplace developer portal (FUN-20) — start a login and
// come back to the page the visitor asked for. An unchecked value would make
// that an open redirect on an authentication endpoint, so a return URL is only
// honoured when its origin is one this deployment serves.
//
// The allowlist is the CORS origin list plus the default frontend URL (see
// cmd/fun-authn-api). Those are exactly the browser origins already trusted to
// read authn's responses with the session cookie attached, so a redirect to one
// grants nothing new, and reusing the list keeps the two from drifting apart:
// an origin that may not call authn has no business being returned to either.

// normalizeOrigin reduces a URL to a comparable scheme://host[:port] origin.
// It reports false for anything that is not an absolute http(s) URL, and for
// URLs carrying userinfo — a redirect target is not a place to hand out
// credentials, and `https://console.example.com@evil.example` reads like one
// origin while naming another.
func normalizeOrigin(raw string) (string, bool) {
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

// isSafeReturnTo reports whether returnTo is a trusted post-login redirect
// target. It is dcim-authn-api's check of the same name, widened from the one
// configured frontend to a list, because the console is not the only surface
// that starts a login here.
func (s *AuthnServer) isSafeReturnTo(returnTo string) bool {
	origin, ok := normalizeOrigin(returnTo)
	if !ok {
		return false
	}

	for _, allowed := range s.config.AllowedReturnOrigins {
		allowedOrigin, ok := normalizeOrigin(allowed)
		if ok && allowedOrigin == origin {
			return true
		}
	}

	return false
}
