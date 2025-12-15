package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	authnv1 "github.com/fundament-oss/fundament/authn-api/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/sqlc/db"
)

// GetUserInfo is the RPC handler for getting user information from a valid JWT.
func (s *AuthnServer) GetUserInfo(
	ctx context.Context,
	req *connect.Request[authnv1.GetUserInfoRequest],
) (*connect.Response[authnv1.GetUserInfoResponse], error) {
	claims, err := s.validateRequestOrCookie(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return connect.NewResponse(&authnv1.GetUserInfoResponse{
		User: &authnv1.User{
			Id:       claims.UserID.String(),
			TenantId: claims.TenantID.String(),
			Name:     claims.Name,
			Groups:   claims.Groups,
		},
	}), nil
}

// HTTP handlers for authentication flow

// handleLogin initiates the OIDC login flow by redirecting to the provider.
// Accepts an optional "return_to" query parameter to redirect after successful login.
func (s *AuthnServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")

	state, err := generateState(returnTo)
	if err != nil {
		s.logger.Error("failed to generate state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Generate PKCE verifier
	verifier := oauth2.GenerateVerifier()

	// Store state and PKCE verifier in session
	if err := s.sessionStore.SetOAuthParams(w, r, state, verifier); err != nil {
		s.logger.Error("failed to store OAuth params in session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Add PKCE challenge to auth URL
	authURL := s.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	s.logger.Debug("redirecting to OIDC provider", "url", authURL, "return_to", returnTo)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleCallback handles the OIDC provider redirect after login.
func (s *AuthnServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for error from OIDC provider
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		s.logger.Warn("OIDC provider returned error", "error", errMsg, "description", errDesc)
		http.Error(w, fmt.Sprintf("Authentication failed: %s", errDesc), http.StatusBadRequest)
		return
	}

	// Verify state from query against stored state
	state := r.URL.Query().Get("state")
	if state == "" {
		s.logger.Warn("missing state parameter")
		http.Error(w, "Missing state parameter", http.StatusBadRequest)
		return
	}

	verifier, valid, err := s.sessionStore.VerifyStateAndGetVerifier(w, r, state)
	if err != nil {
		s.logger.Error("failed to verify state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !valid {
		s.logger.Warn("invalid or expired state", "state", state)
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		s.logger.Warn("missing authorization code")
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens with PKCE verifier
	token, err := s.oauth2Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		s.logger.Error("token exchange failed", "error", err)
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in response")
		http.Error(w, "Missing ID token", http.StatusInternalServerError)
		return
	}

	idToken, err := s.oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		s.logger.Error("ID token verification failed", "error", err)
		http.Error(w, "Invalid ID token", http.StatusUnauthorized)
		return
	}

	var oidcClaims struct {
		Groups        []string `json:"groups"`
		Email         string   `json:"email"`
		Name          string   `json:"name"`
		Sub           string   `json:"sub"`
		EmailVerified bool     `json:"email_verified"`
	}
	if err := idToken.Claims(&oidcClaims); err != nil {
		s.logger.Error("failed to parse claims", "error", err)
		http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Dex staticPasswords doesn't support groups, so fall back to email-based mapping for dev
	groups := oidcClaims.Groups
	if len(groups) == 0 {
		groups = getDevGroups(oidcClaims.Email)
	}

	// Try to get existing user
	user, err := s.queries.UserGetByExternalID(ctx, oidcClaims.Sub)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("failed to get user", "error", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// New user - create tenant and user
		tenantName := oidcClaims.Name
		if tenantName == "" {
			tenantName = oidcClaims.Email
		}
		tenant, err := s.queries.TenantCreate(ctx, tenantName)
		if err != nil {
			s.logger.Error("failed to create tenant", "error", err)
			http.Error(w, "Failed to create tenant", http.StatusInternalServerError)
			return
		}

		user, err = s.queries.UserCreate(ctx, db.UserCreateParams{
			TenantID:   tenant.ID,
			Name:       oidcClaims.Name,
			ExternalID: oidcClaims.Sub,
		})
		if err != nil {
			s.logger.Error("failed to create user", "error", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		s.logger.Info("new user registered",
			"user_id", user.ID,
			"tenant_id", tenant.ID,
			"name", user.Name,
		)
	} else if user.Name != oidcClaims.Name {
		// Existing user - update name if changed
		user, err = s.queries.UserUpdate(ctx, db.UserUpdateParams{
			ExternalID: oidcClaims.Sub,
			Name:       oidcClaims.Name,
		})
		if err != nil {
			s.logger.Error("failed to update user", "error", err)
			http.Error(w, "Failed to update user", http.StatusInternalServerError)
			return
		}
	}

	// Generate JWT
	accessToken, err := s.generateJWT(&user, groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Set auth cookie
	s.setAuthCookie(w, accessToken)

	s.logger.Info("user logged in",
		"user_id", user.ID,
		"tenant_id", user.TenantID,
		"name", user.Name,
		"groups", groups,
	)

	// Parse state to get return URL
	stateData, err := parseState(state)
	if err != nil {
		s.logger.Warn("failed to parse state for return_to", "error", err)
	}

	// Redirect to return_to URL if provided, otherwise default to frontend
	redirectURL := s.config.FrontendURL
	if stateData != nil && stateData.ReturnTo != "" {
		redirectURL = stateData.ReturnTo
	}

	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// handleRefresh refreshes the JWT token.
func (s *AuthnServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, err := s.validateRequestOrCookie(r.Header)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user := &db.OrganizationUser{
		ID:       claims.UserID,
		TenantID: claims.TenantID,
		Name:     claims.Name,
	}

	accessToken, err := s.generateJWT(user, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Update auth cookie
	s.setAuthCookie(w, accessToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int64(s.config.TokenExpiry.Seconds()),
	})
}

// handleLogout clears the auth cookie.
func (s *AuthnServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.clearAuthCookie(w)
	s.logger.Debug("user logged out")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
