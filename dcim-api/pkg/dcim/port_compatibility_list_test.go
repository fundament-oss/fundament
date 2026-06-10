package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createPortDefinition adds a network port to a catalog entry and returns its id.
func createPortDefinition(t *testing.T, env *testEnv, catalogID, name string) string {
	t.Helper()

	client := dcimv1connect.NewCatalogServiceClient(env.client(), env.server.URL)

	resp, err := client.CreatePortDefinition(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePortDefinitionRequest_builder{
			DeviceCatalogId: catalogID,
			Name:            name,
			PortType:        dcimv1.PortType_PORT_TYPE_NETWORK,
			Direction:       dcimv1.PortDirection_PORT_DIRECTION_BIDIR,
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetPortDefinitionId())

	return resp.Msg.GetPortDefinitionId()
}

// TestCatalogService_ListPortCompatibilities verifies that both kinds of
// compatibility row are returned with their category populated: an
// entry-specific one (created via the API, which derives the category from the
// catalog entry) and a category-wide one (compatible_catalog_id NULL, only
// reachable via direct insert) whose catalog id comes back empty.
func TestCatalogService_ListPortCompatibilities(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewCatalogServiceClient(env.client(), env.server.URL)

	hostID := createCatalogEntry(t, env, "Host Switch")
	portDefID := createPortDefinition(t, env, hostID, "eth0")

	// Entry-specific compatibility: the API derives the category (SERVER) from
	// the compatible catalog entry created by the helper.
	compatID := createCatalogEntry(t, env, "Compatible SFP")
	_, err := client.CreatePortCompatibility(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePortCompatibilityRequest_builder{
			PortDefinitionId:    portDefID,
			CompatibleCatalogId: compatID,
		}).Build(),
	))
	require.NoError(t, err)

	// Category-wide compatibility (no specific catalog entry). The create API
	// requires a UUID, so this row can only be seeded directly.
	_, err = env.adminPool.Exec(context.Background(),
		`INSERT INTO dcim.port_compatibilities (port_definition_id, compatible_category) VALUES ($1, 'switch')`,
		portDefID,
	)
	require.NoError(t, err)

	resp, err := client.ListPortCompatibilities(context.Background(), connect.NewRequest(
		(&dcimv1.ListPortCompatibilitiesRequest_builder{PortDefinitionId: portDefID}).Build(),
	))
	require.NoError(t, err)

	compatibilities := resp.Msg.GetCompatibilities()
	require.Len(t, compatibilities, 2)

	var entrySpecific, categoryWide *dcimv1.PortCompatibility
	for _, pc := range compatibilities {
		if pc.GetCompatibleCatalogId() == "" {
			categoryWide = pc
		} else {
			entrySpecific = pc
		}
	}

	require.NotNil(t, entrySpecific, "expected an entry-specific compatibility")
	assert.Equal(t, compatID, entrySpecific.GetCompatibleCatalogId())
	assert.Equal(t, dcimv1.AssetCategory_ASSET_CATEGORY_SERVER, entrySpecific.GetCompatibleCategory())

	require.NotNil(t, categoryWide, "expected a category-wide compatibility")
	assert.Empty(t, categoryWide.GetCompatibleCatalogId())
	assert.Equal(t, dcimv1.AssetCategory_ASSET_CATEGORY_SWITCH, categoryWide.GetCompatibleCategory())
}
