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

	return connect.NewResponse(authnv1.GetUserInfoResponse_builder{
		User: protoUserFromClaims(claims),
	}.Build()), nil
}

// protoUserFromClaims converts JWT claims to a proto User.
func protoUserFromClaims(claims *auth.Claims) *authnv1.User {
	organizationIds := make([]string, 0, len(claims.OrganizationIDs))

	for _, organizationID := range claims.OrganizationIDs {
		organizationIds = append(organizationIds, organizationID.String())
	}

	return authnv1.User_builder{
		Id:              claims.UserID.String(),
		OrganizationIds: organizationIds,
		Name:            claims.Name,
		Groups:          claims.Groups,
	}.Build()
}
