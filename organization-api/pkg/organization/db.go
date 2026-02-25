package organization

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/psqldb"
	dbgen "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

func NewDB(ctx context.Context, logger *slog.Logger, cfg psqldb.Config) (*psqldb.DB, error) {
	db, err := psqldb.New(ctx, logger, cfg, rlsOptions(logger)...)
	if err != nil {
		return nil, fmt.Errorf("creating organization database: %w", err)
	}
	return db, nil
}

func rlsOptions(logger *slog.Logger) []psqldb.Option {
	return []psqldb.Option{
		func(ctx context.Context, config *pgxpool.Config) {
			config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
				queries := dbgen.New(conn)

				if _, err := conn.Exec(ctx, "SET ROLE fun_fundament_api"); err != nil {
					return false, fmt.Errorf("failed to set application role: %w", err)
				}

				if organizationID, ok := OrganizationIDFromContext(ctx); ok {
					logger.Debug("setting organization context for RLS", "organization_id", organizationID.String())
					if err := queries.SetOrganizationContext(ctx, dbgen.SetOrganizationContextParams{
						SetConfig: organizationID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set organization context: %w", err)
					}
				} else {
					logger.Debug("no organization_id in context for PrepareConn")
				}

				if userID, ok := UserIDFromContext(ctx); ok {
					logger.Debug("setting user context for RLS", "user_id", userID.String())
					if err := queries.SetUserContext(ctx, dbgen.SetUserContextParams{
						SetConfig: userID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				} else {
					logger.Debug("no user_id in context for PrepareConn")
				}

				if claims, ok := ClaimsFromContext(ctx); ok {
					if err := queries.SetUserContext(ctx, dbgen.SetUserContextParams{
						SetConfig: claims.UserID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				}

				return true, nil
			}

			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := dbgen.New(c)

				if err := queries.ResetOrganizationContext(context.Background()); err != nil {
					logger.Warn("failed to reset organization context on connection release, destroying connection", "error", err)
					return false
				}

				if err := queries.ResetUserContext(context.Background()); err != nil {
					logger.Warn("failed to reset user context on connection release, destroying connection", "error", err)
					return false
				}

				if _, err := c.Exec(context.Background(), "RESET ROLE"); err != nil {
					logger.Warn("failed to reset role on connection release, destroying connection", "error", err)
					return false
				}

				return true
			}
		},
	}
}
