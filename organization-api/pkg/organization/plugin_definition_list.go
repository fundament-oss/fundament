package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListPluginDefinitions returns the published definitions (version + hash) for a
// plugin, latest first — the set the console offers as install-time version
// choices. An unknown plugin simply yields an empty list.
func (s *Server) ListPluginDefinitions(
	ctx context.Context,
	req *connect.Request[organizationv1.ListPluginDefinitionsRequest],
) (*connect.Response[organizationv1.ListPluginDefinitionsResponse], error) {
	pluginID, err := uuid.Parse(req.Msg.GetPluginId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("plugin_id must be a valid uuid: %w", err))
	}

	rows, err := s.queries.PluginDefinitionListByPlugin(ctx, db.PluginDefinitionListByPluginParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list plugin definitions: %w", err))
	}

	defs := make([]*organizationv1.PluginDefinitionVersion, 0, len(rows))
	for _, row := range rows {
		defs = append(defs, organizationv1.PluginDefinitionVersion_builder{
			Version: row.PluginVersion,
			Hash:    row.Hash,
		}.Build())
	}

	return connect.NewResponse(organizationv1.ListPluginDefinitionsResponse_builder{
		Definitions: defs,
	}.Build()), nil
}
