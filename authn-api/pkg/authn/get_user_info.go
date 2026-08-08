package authn

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/auth"
)

// GetUserInfo is the RPC handler for getting user information from a valid JWT.
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

	return authnv1.GetUserInfoResponse_builder{
		User: protoUserFromClaims(claims),
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
