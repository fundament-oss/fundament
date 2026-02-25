package organization

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

// NewRLSOptions returns psqldb.Option values that configure a connection pool
// to set and reset PostgreSQL session variables for Row-Level Security.
func NewRLSOptions(logger *slog.Logger) []psqldb.Option {
	return []psqldb.Option{
		func(ctx context.Context, config *pgxpool.Config) {
			config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
				queries := db.New(conn)

				if organizationID, ok := OrganizationIDFromContext(ctx); ok {
					logger.Debug("setting organization context for RLS", "organization_id", organizationID.String())
					if err := queries.SetOrganizationContext(ctx, db.SetOrganizationContextParams{
						SetConfig: organizationID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set organization context: %w", err)
					}
				} else {
					logger.Debug("no organization_id in context for PrepareConn")
				}

				if userID, ok := UserIDFromContext(ctx); ok {
					logger.Debug("setting user context for RLS", "user_id", userID.String())
					if err := queries.SetUserContext(ctx, db.SetUserContextParams{
						SetConfig: userID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				} else {
					logger.Debug("no user_id in context for PrepareConn")
				}

				if claims, ok := ClaimsFromContext(ctx); ok {
					if err := queries.SetUserContext(ctx, db.SetUserContextParams{
						SetConfig: claims.UserID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				}

				return true, nil
			}

			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := db.New(c)

				if err := queries.ResetOrganizationContext(context.Background()); err != nil {
					logger.Warn("failed to reset organization context on connection release, destroying connection", "error", err)
					return false
				}

				if err := queries.ResetUserContext(context.Background()); err != nil {
					logger.Warn("failed to reset user context on connection release, destroying connection", "error", err)
					return false
				}

				return true
			}
		},
	}
}
