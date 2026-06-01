package dcimauthn

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// HandlePasswordLogin handles direct password-based authentication via dex.
func (s *Server) HandlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		s.writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	token, err := s.authenticateWithPassword(r, req.Email, req.Password)
	if err != nil {
		s.logger.Warn("password authentication failed", "error", err)
		s.writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in password grant response")
		s.writeError(w, http.StatusInternalServerError, "missing ID token")
		return
	}

	claims, err := s.verifyAndParseIDToken(r, rawIDToken)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid ID token")
		return
	}

	accessToken, err := s.generateJWT(claims)
	if err != nil {
		s.logger.Error("failed to generate JWT", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logger.Info("user logged in via password", "subject", subjectUUID(claims.Sub).String())

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	s.writeJSON(w, http.StatusOK, map[string]any{
		"token_type": "Bearer",
		"expires_in": int(s.config.TokenExpiry.Seconds()),
	})
}

// HandleLogin initiates the OIDC login flow by redirecting to dex.
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")

	state, err := generateState(returnTo)
	if err != nil {
		s.logger.Error("failed to generate state", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	verifier := oauth2.GenerateVerifier()

	if err := s.sessionStore.SetOAuthParams(w, r, state, verifier); err != nil {
		s.logger.Error("failed to store OAuth params in session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	authURL := s.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	s.logger.Debug("redirecting to OIDC provider", "url", authURL, "return_to", returnTo)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback handles the OIDC provider redirect after login.
func (s *Server) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		s.logger.Warn("OIDC provider returned error", "error", errParam, "description", errDesc)
		s.writeError(w, http.StatusBadRequest, "authentication failed: "+errDesc)
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	verifier, valid, err := s.sessionStore.VerifyStateAndGetVerifier(w, r, state)
	if err != nil {
		s.logger.Error("failed to verify state", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !valid {
		s.logger.Warn("invalid or expired state", "state", state)
		s.writeError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	token, err := s.oauth2Config.Exchange(r.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		s.logger.Error("token exchange failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "token exchange failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in response")
		s.writeError(w, http.StatusInternalServerError, "missing ID token")
		return
	}

	claims, err := s.verifyAndParseIDToken(r, rawIDToken)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid ID token")
		return
	}

	accessToken, err := s.generateJWT(claims)
	if err != nil {
		s.logger.Error("failed to generate JWT", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logger.Info("user logged in via OIDC", "subject", subjectUUID(claims.Sub).String())

	redirectURL := s.getRedirectURL(state)
	http.SetCookie(w, s.buildAuthCookie(accessToken))
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// HandleRefresh re-issues the auth cookie from a valid existing token, up to the
// absolute session lifetime. Beyond that the user must authenticate again.
func (s *Server) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// Enforce the absolute session lifetime measured from the original login.
	// Tokens without an auth_time predate this claim and cannot be refreshed.
	if claims.AuthTime == nil {
		s.writeError(w, http.StatusUnauthorized, "session cannot be refreshed, please log in again")
		return
	}
	authTime := claims.AuthTime.Time
	if s.config.MaxSessionAge > 0 && time.Since(authTime) > s.config.MaxSessionAge {
		s.logger.Info("refresh rejected: session exceeded max lifetime", "subject", claims.Subject)
		s.writeError(w, http.StatusUnauthorized, "session expired, please log in again")
		return
	}

	// Re-sign with fresh expiry using the same identity, preserving auth_time.
	accessToken, err := s.mintJWT(claims.Subject, claims.Name, authTime)
	if err != nil {
		s.logger.Error("failed to generate JWT on refresh", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	s.writeJSON(w, http.StatusOK, map[string]any{
		"token_type": "Bearer",
		"expires_in": int(s.config.TokenExpiry.Seconds()),
	})
}

// HandleLogout clears the auth cookie.
func (s *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.buildClearAuthCookie())
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// HandleUserInfo returns the user info from the current JWT.
func (s *Server) HandleUserInfo(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"id":   claims.Subject,
		"name": claims.Name,
	})
}
