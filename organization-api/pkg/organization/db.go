package organization

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

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
	return []psqldb.Option{psqldb.WithSessionSettings(logger,
		psqldb.SessionSetting{
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
		},
		psqldb.SessionSetting{
			Name: "user",
			Set: func(ctx context.Context, conn *pgx.Conn) (bool, error) {
				userID, ok := UserIDFromContext(ctx)
				if !ok {
					return false, nil
				}
				return true, dbgen.New(conn).SetUserContext(ctx, dbgen.SetUserContextParams{ //nolint:wrapcheck // wrapped by psqldb
					SetConfig: userID.String(),
				})
			},
			Reset: func(ctx context.Context, conn *pgx.Conn) error {
				return dbgen.New(conn).ResetUserContext(ctx) //nolint:wrapcheck // wrapped by psqldb
			},
		},
	)}
}
