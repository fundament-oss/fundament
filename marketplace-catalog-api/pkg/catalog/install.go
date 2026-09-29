package catalog

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
)

// InstallServer is the install.v1 handler set: the storefront's reads, plus a
// ListPublishers that covers every listing install.v1 ListPlugins returns.
// The storefront's publisher query only knows publishers with a published
// version, which leaves an organization's own drafts without a name.
type InstallServer struct {
	*Server
}

// NewInstallService returns the install.v1 handlers over the install pool.
func NewInstallService(s *Server) *InstallServer {
	return &InstallServer{Server: s}
}

// ListPublishers implements install.v1.
func (s *InstallServer) ListPublishers(
	ctx context.Context,
	_ *catalogv1.ListPublishersRequest,
) (*catalogv1.ListPublishersResponse, error) {
	rows, err := s.queries.PublisherListInstallable(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing publishers: %w", err))
	}

	publishers := make([]*marketplacev1.Publisher, 0, len(rows))
	for _, row := range rows {
		publishers = append(publishers, marketplacev1.Publisher_builder{
			Id:          row.ID.String(),
			Name:        row.Name,
			DisplayName: row.Alias,
		}.Build())
	}

	return catalogv1.ListPublishersResponse_builder{
		Publishers: publishers,
	}.Build(), nil
}
