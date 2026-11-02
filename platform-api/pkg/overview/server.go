// Package overview serves platform.v1.OverviewService, the anonymous public
// platform page (FUN-23).
package overview

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/sync/singleflight"

	"github.com/fundament-oss/fundament/common/connectrecovery"
	platformv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1/platformv1connect"
)

// DefaultCacheTTL is how long one database read serves every caller.
const DefaultCacheTTL = time.Minute

// readTimeout bounds the one database read that callers share.
const readTimeout = 10 * time.Second

type Config struct {
	// CORSAllowedOrigins is the console origin. The page is anonymous, so no
	// credentials are allowed.
	CORSAllowedOrigins []string
	// CacheTTL bounds how stale the counts may be; it is also the max-age
	// browsers and proxies are told.
	CacheTTL time.Duration
}

type Server struct {
	logger  *slog.Logger
	pool    *pgxpool.Pool
	ttl     time.Duration
	clock   func() time.Time
	handler http.Handler

	// The page is public and its numbers change slowly, so concurrent callers
	// share one read and every caller within the TTL gets the cached result.
	group    singleflight.Group
	mu       sync.Mutex
	cached   *platformv1.GetPlatformOverviewResponse
	cachedAt time.Time
}

// New serves the overview over a pool connected as fun_platform_public_api,
// a role that can run tenant.platform_overview() and read no table.
func New(logger *slog.Logger, pool *pgxpool.Pool, cfg Config) *Server {
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	s := &Server{
		logger: logger,
		pool:   pool,
		ttl:    ttl,
		clock:  time.Now,
	}

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	// No auth interceptor: the page is public by design (FUN-23).
	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		loggingInterceptor,
		validate.NewInterceptor(),
	)

	mux := http.NewServeMux()
	mux.Handle(platformv1connect.NewOverviewServiceHandler(s, interceptors))

	reflector := grpcreflect.NewStaticReflector(platformv1connect.OverviewServiceName)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// GET is how the console calls it (the method has no side effects), so a
	// browser or proxy can serve it from cache. No credentials, so no
	// Authorization or organization headers.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: false,
	})

	s.handler = corsHandler.Handler(mux)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
