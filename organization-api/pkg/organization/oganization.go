package organization

import (
	"fmt"
	"log/slog"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/common/validate"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

type Config struct {
	JWTSecret []byte
}

type OrganizationServer struct {
	config    *Config
	db        *psqldb.DB
	queries   *db.Queries
	logger    *slog.Logger
	validator *validate.Validator
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB) (*OrganizationServer, error) {
	validator, err := validate.New()
	if err != nil {
		return nil, fmt.Errorf("new validator: %w", err)
	}

	return &OrganizationServer{
		logger:    logger,
		config:    cfg,
		db:        database,
		queries:   db.New(database.Pool),
		validator: validator,
	}, nil
}
