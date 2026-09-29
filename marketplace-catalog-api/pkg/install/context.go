// Package install serves install.v1: the marketplace as an installer sees it,
// read as one organization through the fun_marketplace_install_api role
// (FUN-22, "The installer surface"). The handlers are the storefront's; the
// credential, the pool and the policies differ.
package install

import (
	"context"

	"github.com/google/uuid"
)

type contextKeyOrganizationID struct{}

// WithOrganizationID stores the organization the request reads as. The pool
// pushes it into the session on acquire (see NewDB).
func WithOrganizationID(ctx context.Context, organizationID uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyOrganizationID{}, organizationID)
}

// OrganizationIDFromContext extracts the organization set by the interceptor.
func OrganizationIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	organizationID, ok := ctx.Value(contextKeyOrganizationID{}).(uuid.UUID)
	return organizationID, ok
}
