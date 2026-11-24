package authn

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/auth"
)

// userInfoLookupTimeout bounds the account lookup in GetUserInfo, so a
// database that hangs rather than failing fast still falls back to the token
// in time for the console's auth check.
const userInfoLookupTimeout = 2 * time.Second

// GetUserInfo is the RPC handler for getting user information from a valid JWT.
//
// The email address is read from the account, not the token: it stays out of
// the JWT (which is readable by anyone holding it), and a changed address
// shows at once instead of after the next refresh.
func (s *AuthnServer) GetUserInfo(
	ctx context.Context,
	_ *authnv1.GetUserInfoRequest,
) (*authnv1.GetUserInfoResponse, error) {
	callInfo, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("missing call info in context"))
	}

	claims, err := s.validator.Validate(callInfo.RequestHeader())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	u := protoUserFromClaims(claims)

	// A soft-deleted account no longer resolves: its token is refused, as on
	// refresh. Any other database failure answers from the token alone, without
	// the email address. The console's auth guard calls this on every page load
	// and treats any error as a sign-out, so an outage must not fail it, and a
	// hang must not hold it up.
	lookupCtx, cancel := context.WithTimeout(ctx, userInfoLookupTimeout)
	defer cancel()
	row, err := s.queries.UserGetByID(lookupCtx, db.UserGetByIDParams{ID: claims.UserID()})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
	case err != nil:
		s.logger.Warn("failed to read user email for user info, answering without it", "error", err)
	default:
		u.SetEmail(row.Email.String)
	}

	return authnv1.GetUserInfoResponse_builder{
		User: u,
	}.Build(), nil
}

// protoUserFromClaims converts JWT claims to a proto User.
func protoUserFromClaims(claims *auth.Claims) *authnv1.User {
	organizationIds := make([]string, 0, len(claims.OrganizationIDs))

	for _, organizationID := range claims.OrganizationIDs {
		organizationIds = append(organizationIds, organizationID.String())
	}

	return authnv1.User_builder{
		Id:              claims.Subject,
		OrganizationIds: organizationIds,
		Name:            claims.Name,
		Groups:          claims.Groups,
	}.Build()
}
