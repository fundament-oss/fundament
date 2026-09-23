package authn

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
)

// errEmailInUse is returned when the address an identity provider reports is
// already the address of another live account (users_uq_email). Linking the
// two by email alone is not something a sign-in may do on its own: it would
// hand one person's account to whoever controls a second identity with the
// same address at the provider.
var errEmailInUse = errors.New("email address already belongs to another account")

// upsertUser writes the identity provider's view of a user, turning a clash
// on users_uq_email into errEmailInUse with the address logged for the operator.
func (s *AuthnServer) upsertUser(ctx context.Context, queries *db.Queries, claims *oidcClaims) (db.UserUpsertRow, error) {
	row, err := queries.UserUpsert(ctx, db.UserUpsertParams{
		Name:        claims.Name,
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
		Email:       pgtype.Text{String: claims.Email, Valid: claims.Email != ""},
	})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.ConstraintName == dbconst.ConstraintUsersUqEmail {
			s.logger.Warn("sign-in refused: email address already belongs to another account",
				"email", claims.Email, "external_ref", claims.Sub)
			return db.UserUpsertRow{}, errEmailInUse
		}
		s.logger.Error("failed to upsert user", "error", err)
		return db.UserUpsertRow{}, fmt.Errorf("upserting user: %w", err)
	}
	return row, nil
}

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
			Email: claims.Email,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("failed to check for invited user", "error", err)
			return nil, "", fmt.Errorf("looking up invited user: %w", err)
		}
		if err == nil {
			return s.handleInvitedUser(ctx, claims, &invitedUser, loginMethod)
		}
	}

	// First login of an unknown user: register them without an organization
	return s.handleNewUser(ctx, claims, loginMethod)
}

// handleExistingUser handles login for users with a matching external_ref.
func (s *AuthnServer) handleExistingUser(ctx context.Context, claims *oidcClaims, loginMethod string) (*user, string, error) {
	row, err := s.upsertUser(ctx, s.queries, claims)
	if err != nil {
		return nil, "", err
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
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction"))
	}

	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	params := db.UserSetExternalRefParams{
		ID:          invitedUser.ID,
		ExternalRef: pgtype.Text{String: claims.Sub, Valid: true},
		Name:        claims.Name,
	}

	err = qtx.UserSetExternalRef(ctx, params)
	if err != nil {
		s.logger.Error("failed to set external_ref for invited user", "error", err)
		return nil, "", fmt.Errorf("claiming invited user: %w", err)
	}

	organizationIDs, err := s.getUserOrganizationIDs(ctx, invitedUser.ID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		return nil, "", fmt.Errorf("getting user organizations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
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

// handleNewUser registers a first-time user. The user starts without any
// organization membership: organizations are not self-service, an operator
// assigns users to them through funops (before or after this first login).
// Memberships assigned later reach the session on the next token refresh.
func (s *AuthnServer) handleNewUser(ctx context.Context, claims *oidcClaims, loginMethod string) (*user, string, error) {
	row, err := s.upsertUser(ctx, s.queries, claims)
	if err != nil {
		return nil, "", err
	}

	u := &user{
		ID:              row.ID,
		OrganizationIDs: []uuid.UUID{},
		Name:            row.Name,
		ExternalRef:     row.ExternalRef.String,
	}

	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		return nil, "", fmt.Errorf("generating JWT: %w", err)
	}

	s.logger.Info("new user registered without organization",
		"login_method", loginMethod,
		"user_id", u.ID,
		"name", u.Name,
		"email", claims.Email,
	)

	return u, accessToken, nil
}
