package auth

import "net/http"

// CookieBuilder creates auth cookies with consistent settings.
type CookieBuilder struct {
	domain     string
	secure     bool
	cookieName string
	path       string
}

// NewCookieBuilder creates a new CookieBuilder.
// For localhost, pass "localhost" as domain - it will be converted to empty string
// since browsers handle localhost cookies better without an explicit domain.
func NewCookieBuilder(domain string, secure bool, cookieName string) *CookieBuilder {
	// Don't set domain for localhost - browsers handle it better without explicit domain
	if domain == "localhost" {
		domain = ""
	}
	return &CookieBuilder{
		domain:     domain,
		secure:     secure,
		cookieName: cookieName,
		path:       "/",
	}
}

// NewHostOnlyCookieBuilder creates a CookieBuilder for a cookie the browser
// sends to one host and one path only: no Domain attribute, so no sibling
// subdomain ever receives it, and the given path, so the rest of this service's
// endpoints do not either.
//
// This is the refresh token's shape (FUN-23). A cookie built by
// NewCookieBuilder is set on the whole cookie domain on purpose — every service
// validates the access token, so every service must receive it — which is also
// why that token is short-lived. The credential that mints those tokens for a
// day travels to exactly the endpoint that spends it.
func NewHostOnlyCookieBuilder(secure bool, cookieName, path string) *CookieBuilder {
	return &CookieBuilder{
		secure:     secure,
		cookieName: cookieName,
		path:       path,
	}
}

// Build creates an auth cookie with the given token and max age.
func (b *CookieBuilder) Build(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     b.cookieName,
		Value:    token,
		Path:     b.path,
		Domain:   b.domain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   b.secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// BuildClear creates a cookie that clears the auth cookie.
func (b *CookieBuilder) BuildClear() *http.Cookie {
	return &http.Cookie{
		Name:     b.cookieName,
		Value:    "",
		Path:     b.path,
		Domain:   b.domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   b.secure,
		SameSite: http.SameSiteStrictMode,
	}
}
