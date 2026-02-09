package organization

import (
	"log/slog"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

type Config struct {
	JWTSecret []byte
}

type Server struct {
	config        *Config
	db            *psqldb.DB
	queries       *db.Queries
	logger        *slog.Logger
	authValidator *auth.Validator
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB) (*Server, error) {
	return &Server{
		logger:        logger,
		config:        cfg,
		db:            database,
		queries:       db.New(database.Pool),
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
	}, nil
}
