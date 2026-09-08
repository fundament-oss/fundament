package registry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/psqldb"
	dbgen "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
)

// NewDB opens the pool the registry serves from. Unlike the catalog's, every
// connection carries the caller's identity: the role is fixed for the process
// and only the session GUCs vary per request.
func NewDB(ctx context.Context, logger *slog.Logger, cfg psqldb.Config) (*psqldb.DB, error) {
	db, err := psqldb.New(ctx, logger, cfg, rlsOptions(logger)...)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

// rlsOptions pushes the caller into the session on connection acquire, which is
// what every appstore policy reads through authn.current_organization_id().
//
// An absent organization is not an error: the GUC simply stays unset, and the
// policies compare against NULL, which never matches. That fails closed.
func rlsOptions(logger *slog.Logger) []psqldb.Option {
	return []psqldb.Option{
		func(_ context.Context, config *pgxpool.Config) {
			config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
				queries := dbgen.New(conn)

				if organizationID, ok := OrganizationIDFromContext(ctx); ok {
					if err := queries.SetOrganizationContext(ctx, dbgen.SetOrganizationContextParams{
						SetConfig: organizationID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set organization context: %w", err)
					}
				}

				if userID, ok := UserIDFromContext(ctx); ok {
					if err := queries.SetUserContext(ctx, dbgen.SetUserContextParams{
						SetConfig: userID.String(),
					}); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				}

				return true, nil
			}

			// Returning false destroys the connection rather than handing a
			// poisoned one back to the pool; a failed reset would otherwise leak
			// one caller's organization into the next request.
			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := dbgen.New(c)

				if err := queries.ResetOrganizationContext(context.Background()); err != nil {
					logger.Warn("failed to reset organization context on release, destroying connection", "error", err)
					return false
				}

				if err := queries.ResetUserContext(context.Background()); err != nil {
					logger.Warn("failed to reset user context on release, destroying connection", "error", err)
					return false
				}

				return true
			}
		},
	}
}
