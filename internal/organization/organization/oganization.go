package organization

import (
	"log/slog"

	"github.com/fundament-oss/fundament/pkg/psqldb"
	db "github.com/fundament-oss/fundament/internal/organization/db/gen"
)

type Config struct {
	JWTSecret []byte
}

type OrganizationServer struct {
	config  *Config
	queries *db.Queries
	logger  *slog.Logger
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB) (*OrganizationServer, error) {
	return &OrganizationServer{
		logger:  logger,
		config:  cfg,
		queries: db.New(database.Pool),
	}, nil
}
