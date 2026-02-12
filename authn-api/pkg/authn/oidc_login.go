package authn

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/rollback"
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
	// Try by external_ref
	_, err := s.queries.UserGetByExternalRef(ctx, db.UserGetByExternalRefParams{
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("failed to get user by external_ref", "error", err)
		return nil, "", fmt.Errorf("looking up user: %w", err)
	}
	if err == nil {
		return s.handleExistingUser(ctx, claims, loginMethod)
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

// handleExistingUser handles login for users with a matching external_ref.
func (s *AuthnServer) handleExistingUser(ctx context.Context, claims *oidcClaims, loginMethod string) (*user, string, error) {
	params := db.UserUpsertParams{
		Name:        claims.Name,
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
		Email:       pgtype.Text{String: claims.Email, Valid: claims.Email != ""},
	}
	row, err := s.queries.UserUpsert(ctx, params)
	if err != nil {
		s.logger.Error("failed to upsert user", "error", err)
		return nil, "", fmt.Errorf("upserting user: %w", err)
	}

	organizationIDs, err := s.getUserOrganizationIDs(ctx, row.ID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		return nil, "", fmt.Errorf("getting user organizations: %w", err)
	}

	u := &user{
		ID:              row.ID,
		OrganizationIDs: organizationIDs,
		Name:            row.Name,
		ExternalRef:     row.ExternalRef.String,
	}

	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}

	s.logger.Info("existing user logged in",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_ids", u.OrganizationIDs,
	)

	return u, accessToken, nil
}

// handleInvitedUser handles login for users who were invited by email.
func (s *AuthnServer) handleInvitedUser(ctx context.Context, claims *oidcClaims, invitedUser *db.UserGetByEmailRow, loginMethod string) (*user, string, error) {
	err := s.queries.UserSetExternalRef(ctx, db.UserSetExternalRefParams{
		ID:          invitedUser.ID,
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
		Name:        claims.Name,
	})
	if err != nil {
		s.logger.Error("failed to set external_ref for invited user", "error", err)
		return nil, "", fmt.Errorf("claiming invited user: %w", err)
	}

	// Transition pending invitations to accepted
	err = s.queries.OrganizationUserAccept(ctx, db.OrganizationUserAcceptParams{UserID: invitedUser.ID})
	if err != nil {
		s.logger.Error("failed to accept organization memberships", "error", err)
		return nil, "", fmt.Errorf("accepting memberships: %w", err)
	}

	organizationIDs, err := s.getUserOrganizationIDs(ctx, invitedUser.ID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		return nil, "", fmt.Errorf("getting user organizations: %w", err)
	}

	u := &user{
		ID:              invitedUser.ID,
		OrganizationIDs: organizationIDs,
		Name:            claims.Name,
		ExternalRef:     claims.Sub,
	}

	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}

	s.logger.Info("invited user claimed account",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_ids", u.OrganizationIDs,
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

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to tbegin transaction"))
	}

	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	organization, err := qtx.OrganizationCreate(ctx, db.OrganizationCreateParams{
		Name: organizationName,
	})
	if err != nil {
		s.logger.Error("failed to create organization", "error", err)
		return nil, "", fmt.Errorf("creating organization: %w", err)
	}

	params := db.UserUpsertParams{
		Name:        claims.Name,
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
		Email:       pgtype.Text{String: claims.Email, Valid: claims.Email != ""},
	}

	row, err := qtx.UserUpsert(ctx, params)
	if err != nil {
		s.logger.Error("failed to upsert user", "error", err)
		return nil, "", fmt.Errorf("creating user: %w", err)
	}

	_, err = qtx.OrganizationUserCreate(ctx, db.OrganizationUserCreateParams{
		OrganizationID: organization.ID,
		UserID:         row.ID,
		Role:           "admin",
	})
	if err != nil {
		s.logger.Error("failed to create organization membership", "error", err)
		return nil, "", fmt.Errorf("creating organization membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	u := &user{
		ID:              row.ID,
		OrganizationIDs: []uuid.UUID{organization.ID},
		Name:            row.Name,
		ExternalRef:     row.ExternalRef.String,
	}

	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}

	s.logger.Info("new user registered",
		"login_method", loginMethod,
		"user_id", u.ID,
		"organization_id", organization.ID,
		"name", u.Name,
	)

	return u, accessToken, nil
}
