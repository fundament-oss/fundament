package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
)

// Ensure AuthnServer implements ServerInterface
var _ authnhttp.ServerInterface = (*AuthnServer)(nil)

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
			Id:             claims.UserID.String(),
			OrganizationId: claims.OrganizationID.String(),
			Name:           claims.Name,
			Groups:         claims.Groups,
		},
	}), nil
}

// HandleLogin initiates the OIDC login flow by redirecting to the provider.
func (s *AuthnServer) HandleLogin(w http.ResponseWriter, r *http.Request, params authnhttp.HandleLoginParams) {
	var returnTo string
	if params.ReturnTo != nil {
		returnTo = *params.ReturnTo
	}

	state, err := generateState(returnTo)
	if err != nil {
		s.logger.Error("failed to generate state", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Internal server error"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	// Generate PKCE verifier
	verifier := oauth2.GenerateVerifier()

	// Store state and PKCE verifier in session
	if err := s.sessionStore.SetOAuthParams(w, r, state, verifier); err != nil {
		s.logger.Error("failed to store OAuth params in session", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Internal server error"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	// Add PKCE challenge to auth URL
	authURL := s.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	s.logger.Debug("redirecting to OIDC provider", "url", authURL, "return_to", returnTo)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback handles the OIDC provider redirect after login.
func (s *AuthnServer) HandleCallback(w http.ResponseWriter, r *http.Request, params authnhttp.HandleCallbackParams) {
	// Check for error from OIDC provider
	if params.Error != nil {
		errDesc := ""
		if params.ErrorDescription != nil {
			errDesc = *params.ErrorDescription
		}
		s.logger.Warn("OIDC provider returned error", "error", *params.Error, "description", errDesc)
		if err := s.writeJSON(w, http.StatusBadRequest, authnhttp.ErrorResponse{Error: fmt.Sprintf("Authentication failed: %s", errDesc)}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	state := params.State
	code := params.Code

	verifier, valid, err := s.sessionStore.VerifyStateAndGetVerifier(w, r, state)
	if err != nil {
		s.logger.Error("failed to verify state", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Internal server error"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	if !valid {
		s.logger.Warn("invalid or expired state", "state", state)
		if err := s.writeJSON(w, http.StatusBadRequest, authnhttp.ErrorResponse{Error: "Invalid or expired state"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	// Exchange code for tokens with PKCE verifier
	token, err := s.oauth2Config.Exchange(r.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		s.logger.Error("token exchange failed", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Token exchange failed"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in response")
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Missing ID token"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	idToken, err := s.oidcVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.logger.Info("ID token verification failed", "error", err)
		if err := s.writeJSON(w, http.StatusUnauthorized, authnhttp.ErrorResponse{Error: "Invalid ID token"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.logger.Error("failed to parse claims", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Failed to parse claims"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	user, groups, accessToken, err := s.processOIDCLogin(r.Context(), &claims, "oidc")
	if err != nil {
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: err.Error()}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	s.logger.Info("user logged in",
		"user_id", user.ID,
		"organization_id", user.OrganizationID,
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

	// Set auth cookie before redirect
	http.SetCookie(w, s.buildAuthCookie(accessToken))
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// HandleRefresh refreshes the JWT token.
func (s *AuthnServer) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validateRequestOrCookie(r.Header)
	if err != nil {
		if err := s.writeJSON(w, http.StatusUnauthorized, authnhttp.ErrorResponse{Error: "Unauthorized"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	user := &db.TenantUser{
		ID:             claims.UserID,
		OrganizationID: claims.OrganizationID,
		Name:           claims.Name,
	}

	accessToken, err := s.generateJWT(user, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Failed to generate token"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	if err := s.writeJSON(w, http.StatusOK, authnhttp.RefreshResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.config.TokenExpiry.Seconds()),
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// HandleLogout clears the auth cookie.
func (s *AuthnServer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("user logged out")

	http.SetCookie(w, s.buildClearAuthCookie())
	if err := s.writeJSON(w, http.StatusOK, authnhttp.StatusResponse{Status: ptr("ok")}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// HandlePasswordLogin handles direct password-based authentication.
func (s *AuthnServer) HandlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	var req authnhttp.PasswordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := s.writeJSON(w, http.StatusBadRequest, authnhttp.ErrorResponse{Error: "Invalid request body"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	email := string(req.Email)
	password := req.Password

	if email == "" || password == "" {
		if err := s.writeJSON(w, http.StatusBadRequest, authnhttp.ErrorResponse{Error: "Email and password are required"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	// Exchange email/password for token using Dex's token endpoint with password grant
	token, err := s.authenticateWithPassword(r.Context(), email, password)
	if err != nil {
		s.logger.Warn("password authentication failed", "email", email, "error", err)
		if err := s.writeJSON(w, http.StatusUnauthorized, authnhttp.ErrorResponse{Error: "Authentication failed"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing ID token in password grant response")
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Missing ID token"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	idToken, err := s.oidcVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.logger.Info("ID token verification failed", "error", err)
		if err := s.writeJSON(w, http.StatusUnauthorized, authnhttp.ErrorResponse{Error: "Invalid ID token"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.logger.Error("failed to parse claims", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: "Failed to parse claims"}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	user, groups, accessToken, err := s.processOIDCLogin(r.Context(), &claims, "password")
	if err != nil {
		s.logger.Error("process oidc login", "error", err)
		if err := s.writeJSON(w, http.StatusInternalServerError, authnhttp.ErrorResponse{Error: err.Error()}); err != nil {
			s.logger.Error("failed to write JSON response", "error", err)
		}
		return
	}

	s.logger.Info("user logged in via password",
		"user_id", user.ID,
		"organization_id", user.OrganizationID,
		"name", user.Name,
		"groups", groups,
	)

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	if err := s.writeJSON(w, http.StatusOK, authnhttp.TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.config.TokenExpiry.Seconds()),
		User: authnhttp.User{
			Id:             user.ID,
			OrganizationId: user.OrganizationID,
			Name:           user.Name,
			Groups:         groups,
		},
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// oidcClaims represents the claims extracted from an OIDC ID token.
type oidcClaims struct {
	Groups        []string `json:"groups"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Sub           string   `json:"sub"`
	EmailVerified bool     `json:"email_verified"`
}

// processOIDCLogin handles the common logic for processing an OIDC login,
// including user lookup/creation and JWT generation.
// Returns the user, groups, and access token on success.
func (s *AuthnServer) processOIDCLogin(ctx context.Context, claims *oidcClaims, loginMethod string) (*db.TenantUser, []string, string, error) {
	// Dex staticPasswords doesn't support groups, so fall back to email-based mapping for dev
	groups := claims.Groups
	if len(groups) == 0 {
		groups = getDevGroups(claims.Email)
	}

	// Check if user exists to determine if we need to create an organization
	var organizationID uuid.UUID
	existingUser, err := s.queries.UserGetByExternalID(ctx, db.UserGetByExternalIDParams{
		ExternalID: claims.Sub,
	})
	isNewUser := errors.Is(err, pgx.ErrNoRows)

	if err != nil && !isNewUser {
		s.logger.Error("failed to get user", "error", err)
		return nil, nil, "", fmt.Errorf("database error")
	}

	if isNewUser {
		// New user - create organization first
		organizationName := claims.Name
		if organizationName == "" {
			organizationName = claims.Email
		}

		organization, err := s.queries.OrganizationCreate(ctx, db.OrganizationCreateParams{
			Name: organizationName,
		})
		if err != nil {
			s.logger.Error("failed to create organization", "error", err)
			return nil, nil, "", fmt.Errorf("failed to create organization")
		}
		organizationID = organization.ID
	} else {
		organizationID = existingUser.OrganizationID
	}

	params := db.UserUpsertParams{
		OrganizationID: organizationID,
		Name:           claims.Name,
		ExternalID:     claims.Sub,
	}

	// Upsert user (creates if new, updates name if existing)
	user, err := s.queries.UserUpsert(ctx, params)
	if err != nil {
		s.logger.Error("failed to upsert user", "error", err)
		return nil, nil, "", fmt.Errorf("failed to upsert user")
	}

	if isNewUser {
		s.logger.Info("new user registered",
			"login_method", loginMethod,
			"user_id", user.ID,
			"organization_id", organizationID,
			"name", user.Name,
		)
	}

	// Generate JWT
	accessToken, err := s.generateJWT(&user, groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, nil, "", fmt.Errorf("failed to generate token")
	}

	return &user, groups, accessToken, nil
}

// writeJSON writes a JSON response with the given status code.
func (s *AuthnServer) writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encoding JSON response: %w", err)
	}

	return nil
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}
