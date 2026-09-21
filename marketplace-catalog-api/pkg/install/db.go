package install

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/psqldb"
	dbgen "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/db/gen"
)

// NewDB opens the pool install.v1 serves from, connected as
// fun_marketplace_install_api. The role is fixed for the process; the
// organization varies per request through the session GUC every install
// policy reads via authn.current_organization_id(). Same pattern as the
// registry's pool, but lazy: the storefront must boot and stay ready even
// while this role or its grants are still being provisioned, so an
// unreachable install database fails install.v1 calls, not the process.
func NewDB(ctx context.Context, logger *slog.Logger, cfg psqldb.Config) (*psqldb.DB, error) {
	db, err := psqldb.NewLazy(ctx, logger, cfg, rlsOptions(logger)...)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

// rlsOptions pushes the caller's organization into the session on acquire.
// An absent organization is not an error: the GUC stays unset, the policies
// compare against NULL, and only PUBLIC listings match. That fails closed.
func rlsOptions(logger *slog.Logger) []psqldb.Option {
	return []psqldb.Option{psqldb.WithSessionSettings(logger, psqldb.SessionSetting{
		Name: "organization",
		Set: func(ctx context.Context, conn *pgx.Conn) (bool, error) {
			organizationID, ok := OrganizationIDFromContext(ctx)
			if !ok {
				return false, nil
			}
			return true, dbgen.New(conn).SetOrganizationContext(ctx, dbgen.SetOrganizationContextParams{ //nolint:wrapcheck // wrapped by psqldb
				SetConfig: organizationID.String(),
			})
		},
		Reset: func(ctx context.Context, conn *pgx.Conn) error {
			return dbgen.New(conn).ResetOrganizationContext(ctx) //nolint:wrapcheck // wrapped by psqldb
		},
	})}
}
