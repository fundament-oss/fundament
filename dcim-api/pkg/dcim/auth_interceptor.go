package dcim

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
)

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate validates the DCIM JWT and injects the user ID into ctx. A
// valid signature is not enough: the subject must still resolve to a live
// directory entry, so revoking (soft-deleting) a user ends their access at
// the next RPC rather than at token expiry.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	userID := claims.UserID()
	if _, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: userID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated,
				errors.New("no directory entry for the authenticated user"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolving user: %w", err))
	}

	ctx = auth.WithUserID(ctx, userID)
	s.logger.DebugContext(ctx, "request authenticated", "user_id", userID)
	return ctx, nil
}
