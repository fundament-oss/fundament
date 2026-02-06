package authn

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
)

// oidcClaims represents the claims extracted from an OIDC ID token.
type oidcClaims struct {
	Groups        []string `json:"groups"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Sub           string   `json:"sub"`
	EmailVerified bool     `json:"email_verified"`
}

// verifyAndParseIDToken verifies an ID token and extracts the claims.
func (s *AuthnServer) verifyAndParseIDToken(ctx context.Context, rawIDToken string) (*oidcClaims, error) {
	idToken, err := s.oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		s.logger.Info("ID token verification failed", "error", err)
		return nil, err
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.logger.Error("failed to parse claims", "error", err)
		return nil, err
	}

	return &claims, nil
}

// processOIDCLogin handles the common logic for processing an OIDC login,
// including user lookup/creation and JWT generation.
// Returns the user, groups, and access token on success.
func (s *AuthnServer) processOIDCLogin(ctx context.Context, claims *oidcClaims, loginMethod string) (*user, string, error) {
	// Try by external ID
	existingUser, err := s.queries.UserGetByExternalID(ctx, db.UserGetByExternalIDParams{
		ExternalID: pgtype.Text{String: claims.Sub, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("failed to get user by external_id", "error", err)
		return nil, "", fmt.Errorf("looking up user: %w", err)
	}
	if err == nil {
		return s.handleExistingUser(ctx, claims, &existingUser, loginMethod)
	}

	// Try invited user by email
	if claims.Email != "" {
		invitedUser, err := s.queries.UserGetByEmail(ctx, db.UserGetByEmailParams{
			Email: pgtype.Text{String: claims.Email, Valid: true},
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("failed to check for invited user", "error", err)
			return nil, "", fmt.Errorf("looking up invited user: %w", err)
		}
		if err == nil {
			return s.handleInvitedUser(ctx, claims, &invitedUser, loginMethod)
		}
	}

	// Create new user with new organization
	return s.handleNewUser(ctx, claims, loginMethod)
}

// handleExistingUser handles login for users with a matching external_id.
func (s *AuthnServer) handleExistingUser(ctx context.Context, claims *oidcClaims, existingUser *db.UserGetByExternalIDRow, loginMethod string) (*user, string, error) {
	params := db.UserUpsertParams{
		OrganizationID: existingUser.OrganizationID,
		Name:           claims.Name,
		ExternalID:     pgtype.Text{String: claims.Sub, Valid: true},
		Email:          pgtype.Text{String: claims.Email, Valid: claims.Email != ""},
	}
	row, err := s.queries.UserUpsert(ctx, params)
	if err != nil {
		s.logger.Error("failed to upsert user", "error", err)
		return nil, "", fmt.Errorf("upserting user: %w", err)
	}

	u, accessToken, err := s.generateAccessToken(&row, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", err
	}

	s.logger.Info("existing user logged in",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_id", u.OrganizationID,
	)

	return u, accessToken, nil
}

// handleInvitedUser handles login for users who were invited by email.
func (s *AuthnServer) handleInvitedUser(ctx context.Context, claims *oidcClaims, invitedUser *db.UserGetByEmailRow, loginMethod string) (*user, string, error) {
	err := s.queries.UserSetExternalID(ctx, db.UserSetExternalIDParams{
		ID:         invitedUser.ID,
		ExternalID: pgtype.Text{String: claims.Sub, Valid: true},
		Name:       claims.Name,
	})
	if err != nil {
		s.logger.Error("failed to set external_id for invited user", "error", err)
		return nil, "", fmt.Errorf("claiming invited user: %w", err)
	}

	row, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: invitedUser.ID})
	if err != nil {
		s.logger.Error("failed to fetch updated user", "error", err)
		return nil, "", fmt.Errorf("fetching updated user: %w", err)
	}

	u := userFromGetByIDRow(&row)
	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}

	s.logger.Info("invited user claimed account",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_id", u.OrganizationID,
		"name", u.Name,
		"email", claims.Email,
	)

	return u, accessToken, nil
}

// handleNewUser creates a new organization and user for first-time registration.
func (s *AuthnServer) handleNewUser(ctx context.Context, claims *oidcClaims, loginMethod string) (*user, string, error) {
	organizationName := claims.Name
	if organizationName == "" {
		organizationName = claims.Email
	}

	organization, err := s.queries.OrganizationCreate(ctx, db.OrganizationCreateParams{
		Name: organizationName,
	})
	if err != nil {
		s.logger.Error("failed to create organization", "error", err)
		return nil, "", fmt.Errorf("creating organization: %w", err)
	}

	params := db.UserUpsertParams{
		OrganizationID: organization.ID,
		Name:           claims.Name,
		ExternalID:     pgtype.Text{String: claims.Sub, Valid: true},
		Email:          pgtype.Text{String: claims.Email, Valid: claims.Email != ""},
	}

	row, err := s.queries.UserUpsert(ctx, params)
	if err != nil {
		s.logger.Error("failed to upsert user", "error", err)
		return nil, "", fmt.Errorf("creating user: %w", err)
	}

	u, accessToken, err := s.generateAccessToken(&row, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", err
	}

	s.logger.Info("new user registered",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_id", organization.ID,
		"name", u.Name,
	)

	return u, accessToken, nil
}

// generateAccessToken creates a user from a db row and generates a JWT.
func (s *AuthnServer) generateAccessToken(row *db.UserUpsertRow, groups []string) (*user, string, error) {
	u := userFromUpsertRow(row)
	accessToken, err := s.generateJWT(u, groups)
	if err != nil {
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}
	return u, accessToken, nil
}

// userFromUpsertRow converts a db.UserUpsertRow to *user.
func userFromUpsertRow(row *db.UserUpsertRow) *user {
	return &user{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		ExternalID:     row.ExternalID.String,
	}
}

// userFromGetByIDRow converts a db.UserGetByIDRow to *user.
func userFromGetByIDRow(row *db.UserGetByIDRow) *user {
	return &user{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		ExternalID:     row.ExternalID.String,
	}
}
