package authn

import (
	"encoding/json"
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandlePasswordLogin handles direct password-based authentication.
func (s *AuthnServer) HandlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	var req authnhttp.PasswordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorJSON(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	email := string(req.Email)
	password := req.Password

	if email == "" || password == "" {
		s.writeErrorJSON(w, http.StatusBadRequest, "Email and password are required")
		return
	}

	token, err := s.authenticateWithPassword(r.Context(), email, password)
	if err != nil {
		s.logger.Warn("password authentication failed", "email", email, "error", err)
		s.writeErrorJSON(w, http.StatusUnauthorized, "Authentication failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in password grant response")
		s.writeErrorJSON(w, http.StatusInternalServerError, "Missing ID token")
		return
	}

	claims, err := s.verifyAndParseIDToken(r.Context(), rawIDToken)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Invalid ID token")
		return
	}

	user, accessToken, err := s.processOIDCLogin(r.Context(), claims, "password")
	if err != nil {
		s.logger.Error("process oidc login", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("user logged in via password",
		"user_id", user.ID,
		"organization_ids", user.OrganizationIDs,
		"name", user.Name,
		"groups", claims.Groups,
	)

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	if err := s.writeJSON(w, http.StatusOK, authnhttp.TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.config.TokenExpiry.Seconds()),
		User:        httpUserFromUser(user, claims.Groups),
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// httpUserFromUser converts user and groups to an authnhttp.User.
func httpUserFromUser(u *user, groups []string) authnhttp.User {
	return authnhttp.User{
		Id:              u.ID,
		OrganizationIds: u.OrganizationIDs,
		Name:            u.Name,
		Groups:          groups,
	}
}
