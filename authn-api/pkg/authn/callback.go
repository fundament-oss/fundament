package authn

import (
	"fmt"
	"net/http"

	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandleCallback handles the OIDC provider redirect after login.
func (s *AuthnServer) HandleCallback(w http.ResponseWriter, r *http.Request, params authnhttp.HandleCallbackParams) {
	if params.Error != nil {
		s.handleOIDCError(w, params)
		return
	}

	state := params.State
	code := params.Code

	verifier, valid, err := s.sessionStore.VerifyStateAndGetVerifier(w, r, state)
	if err != nil {
		s.logger.Error("failed to verify state", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if !valid {
		s.logger.Warn("invalid or expired state", "state", state)
		s.writeErrorJSON(w, http.StatusBadRequest, "Invalid or expired state")
		return
	}

	token, err := s.oauth2Config.Exchange(r.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		s.logger.Error("token exchange failed", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Token exchange failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in response")
		s.writeErrorJSON(w, http.StatusInternalServerError, "Missing ID token")
		return
	}

	claims, err := s.verifyAndParseIDToken(r.Context(), rawIDToken)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Invalid ID token")
		return
	}

	user, accessToken, err := s.processOIDCLogin(r.Context(), claims, "oidc")
	if err != nil {
		s.writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("user logged in",
		"user_id", user.ID,
		"organization_id", user.OrganizationID,
		"name", user.Name,
		"groups", claims.Groups,
	)

	redirectURL := s.getRedirectURL(state)
	http.SetCookie(w, s.buildAuthCookie(accessToken))
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// handleOIDCError handles error responses from the OIDC provider.
func (s *AuthnServer) handleOIDCError(w http.ResponseWriter, params authnhttp.HandleCallbackParams) {
	errDesc := ""
	if params.ErrorDescription != nil {
		errDesc = *params.ErrorDescription
	}
	s.logger.Warn("OIDC provider returned error", "error", *params.Error, "description", errDesc)
	s.writeErrorJSON(w, http.StatusBadRequest, fmt.Sprintf("Authentication failed: %s", errDesc))
}

// getRedirectURL returns the redirect URL from state, or the default frontend URL.
func (s *AuthnServer) getRedirectURL(state string) string {
	stateData, err := parseState(state)
	if err != nil {
		s.logger.Warn("failed to parse state for return_to", "error", err)
		return s.config.FrontendURL
	}
	if stateData.ReturnTo != "" {
		return stateData.ReturnTo
	}
	return s.config.FrontendURL
}
