package organization

import (
	"log/slog"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

type Config struct {
	JWTSecret []byte
}

type OrganizationServer struct {
	config        *Config
	db            *psqldb.DB
	queries       *db.Queries
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB, authzClient *authz.Client) (*OrganizationServer, error) {
	return &OrganizationServer{
		logger:        logger,
		config:        cfg,
		db:            database,
		queries:       db.New(database.Pool),
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
		authz:         authzClient,
	}, nil
}
