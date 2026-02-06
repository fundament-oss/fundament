package authn

import (
	"context"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/auth"
)

// GetUserInfo is the RPC handler for getting user information from a valid JWT.
func (s *AuthnServer) GetUserInfo(
	ctx context.Context,
	req *connect.Request[authnv1.GetUserInfoRequest],
) (*connect.Response[authnv1.GetUserInfoResponse], error) {
	claims, err := s.validator.Validate(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return connect.NewResponse(&authnv1.GetUserInfoResponse{
		User: protoUserFromClaims(claims),
	}), nil
}

// protoUserFromClaims converts JWT claims to a proto User.
func protoUserFromClaims(claims *auth.Claims) *authnv1.User {
	return &authnv1.User{
		Id:             claims.UserID.String(),
		OrganizationId: claims.OrganizationID.String(),
		Name:           claims.Name,
		Groups:         claims.Groups,
	}
}
