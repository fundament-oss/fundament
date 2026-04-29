package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func Test_OrganizationLimits_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetOrganizationLimits(context.Background(), connect.NewRequest(
		organizationv1.GetOrganizationLimitsRequest_builder{Id: uuid.New().String()}.Build(),
	))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_OrganizationLimits_Get_NoLimitsSet(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(organizationv1.GetOrganizationLimitsRequest_builder{Id: orgID.String()}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	res, err := client.GetOrganizationLimits(context.Background(), req)
	require.NoError(t, err)

	limits := res.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.False(t, limits.HasMaxNodesPerCluster())
	assert.False(t, limits.HasMaxNodePoolsPerCluster())
	assert.False(t, limits.HasMaxNodesPerNodePool())
	assert.False(t, limits.HasDefaultMemoryRequestMi())
	assert.False(t, limits.HasDefaultMemoryLimitMi())
	assert.False(t, limits.HasDefaultCpuRequestM())
	assert.False(t, limits.HasDefaultCpuLimitM())
}

func Test_OrganizationLimits_Get(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	updateReq := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                     orgID.String(),
		MaxNodesPerCluster:     proto.Int32(50),
		MaxNodePoolsPerCluster: proto.Int32(10),
		MaxNodesPerNodePool:    proto.Int32(20),
		DefaultMemoryRequestMi: proto.Int32(128),
		DefaultMemoryLimitMi:   proto.Int32(256),
		DefaultCpuRequestM:     proto.Int32(100),
		DefaultCpuLimitM:       proto.Int32(500),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token)
	updateReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateOrganizationLimits(context.Background(), updateReq)
	require.NoError(t, err)

	getReq := connect.NewRequest(organizationv1.GetOrganizationLimitsRequest_builder{Id: orgID.String()}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.GetOrganizationLimits(context.Background(), getReq)
	require.NoError(t, err)

	limits := res.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.EqualValues(t, 50, limits.GetMaxNodesPerCluster())
	assert.EqualValues(t, 10, limits.GetMaxNodePoolsPerCluster())
	assert.EqualValues(t, 20, limits.GetMaxNodesPerNodePool())
	assert.EqualValues(t, 128, limits.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 256, limits.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, limits.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, limits.GetDefaultCpuLimitM())
}
