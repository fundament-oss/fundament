package auth

import "net/http"

// CookieBuilder creates auth cookies with consistent settings.
type CookieBuilder struct {
	domain string
	secure bool
}

// NewCookieBuilder creates a new CookieBuilder.
// For localhost, pass "localhost" as domain - it will be converted to empty string
// since browsers handle localhost cookies better without an explicit domain.
func NewCookieBuilder(domain string, secure bool) *CookieBuilder {
	// Don't set domain for localhost - browsers handle it better without explicit domain
	if domain == "localhost" {
		domain = ""
	}
	return &CookieBuilder{
		domain: domain,
		secure: secure,
	}
}

// Build creates an auth cookie with the given token and max age.
func (b *CookieBuilder) Build(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
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
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		Domain:   b.domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   b.secure,
		SameSite: http.SameSiteStrictMode,
	}
}
