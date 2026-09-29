package auth

import (
	"fmt"
	"net/http"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// OrganizationHeader selects the organization a UserToken caller acts for, on
// every organization-scoped surface (FUN-6).
const OrganizationHeader = "Fun-Organization"

// OrganizationFromHeader resolves the OrganizationHeader against the caller's
// memberships. The UserToken carries the authoritative membership list, so
// this is settled without a database round trip. Errors are connect errors,
// ready to return from an interceptor: a missing or malformed header is
// InvalidArgument, an organization the caller is not a member of is
// PermissionDenied.
func OrganizationFromHeader(claims *Claims, header http.Header) (uuid.UUID, error) {
	value := header.Get(OrganizationHeader)
	if value == "" {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing %s header", OrganizationHeader))
	}
	organizationID, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization ID: %w", err))
	}
	if !slices.Contains(claims.OrganizationIDs, organizationID) {
		return uuid.Nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a member of organization %s", organizationID))
	}
	return organizationID, nil
}
