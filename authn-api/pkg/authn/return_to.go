package authn

// The login endpoint takes a `return_to` and the callback redirects the browser
// to it once the session cookie is set, which is what lets a surface other than
// the console — the marketplace developer portal (FUN-20) — start a login and
// come back to the page the visitor asked for. An unchecked value would make
// that an open redirect on an authentication endpoint, so a return URL is only
// honoured when its origin is one this deployment serves (auth.ReturnOrigins,
// shared with dcim-authn-api).
//
// The allowlist is the CORS origin list plus the default frontend URL (see
// cmd/fun-authn-api). Those are exactly the browser origins already trusted to
// read authn's responses with the session cookie attached, so a redirect to one
// grants nothing new, and reusing the list keeps the two from drifting apart:
// an origin that may not call authn has no business being returned to either.

// isSafeReturnTo reports whether returnTo is a trusted post-login redirect
// target. A list rather than dcim-authn-api's one frontend, because the console
// is not the only surface that starts a login here.
func (s *AuthnServer) isSafeReturnTo(returnTo string) bool {
	return s.allowedReturnOrigins.Allows(returnTo)
}
