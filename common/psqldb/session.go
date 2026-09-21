package psqldb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionSetting is one session variable an RLS pool pushes into a connection
// when a request acquires it and clears when the connection goes back.
type SessionSetting struct {
	// Name labels the setting in errors and logs, e.g. "organization".
	Name string
	// Set applies the request's value from ctx. It reports false, and sets
	// nothing, when ctx carries no value: the policies then compare against
	// NULL and fail closed.
	Set func(ctx context.Context, conn *pgx.Conn) (bool, error)
	// Reset clears the setting on release.
	Reset func(ctx context.Context, conn *pgx.Conn) error
}

// WithSessionSettings returns the Option every RLS pool uses: each setting is
// applied on acquire, and each is reset on release. A failed reset destroys
// the connection rather than handing a poisoned one back to the pool, which
// would leak one caller's identity into the next request.
func WithSessionSettings(logger *slog.Logger, settings ...SessionSetting) Option {
	return func(_ context.Context, config *pgxpool.Config) {
		config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
			for _, s := range settings {
				applied, err := s.Set(ctx, conn)
				if err != nil {
					return false, fmt.Errorf("failed to set %s context: %w", s.Name, err)
				}
				if applied {
					logger.Debug("set session context for RLS", "setting", s.Name)
				}
			}
			return true, nil
		}

		config.AfterRelease = func(conn *pgx.Conn) bool {
			for _, s := range settings {
				if err := s.Reset(context.Background(), conn); err != nil {
					logger.Warn("failed to reset session context on release, destroying connection",
						"setting", s.Name, "error", err)
					return false
				}
			}
			return true
		}
	}
}
