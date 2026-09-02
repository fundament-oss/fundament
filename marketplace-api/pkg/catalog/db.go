package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fundament-oss/fundament/common/psqldb"
)

// No connection hooks: the catalog is anonymous, so there is no organization or
// user to push into the session. The SELECT-only role is the boundary.
func NewDB(ctx context.Context, logger *slog.Logger, cfg psqldb.Config) (*psqldb.DB, error) {
	database, err := psqldb.New(ctx, logger, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return database, nil
}
