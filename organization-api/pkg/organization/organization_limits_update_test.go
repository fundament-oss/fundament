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

func Test_OrganizationLimits_Update_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	_, err := client.UpdateOrganizationLimits(context.Background(), connect.NewRequest(
		organizationv1.UpdateOrganizationLimitsRequest_builder{Id: uuid.New().String()}.Build(),
	))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_OrganizationLimits_Update(t *testing.T) {
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

	getRes, err := client.GetOrganizationLimits(context.Background(), getReq)
	require.NoError(t, err)

	limits := getRes.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.EqualValues(t, 50, limits.GetMaxNodesPerCluster())
	assert.EqualValues(t, 10, limits.GetMaxNodePoolsPerCluster())
	assert.EqualValues(t, 20, limits.GetMaxNodesPerNodePool())
	assert.EqualValues(t, 128, limits.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 256, limits.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, limits.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, limits.GetDefaultCpuLimitM())
}

func Test_OrganizationLimits_Update_Overwrites(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	firstUpdate := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                 orgID.String(),
		MaxNodesPerCluster: proto.Int32(50),
	}.Build())
	firstUpdate.Header().Set("Authorization", "Bearer "+token)
	firstUpdate.Header().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateOrganizationLimits(context.Background(), firstUpdate)
	require.NoError(t, err)

	secondUpdate := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                 orgID.String(),
		MaxNodesPerCluster: proto.Int32(100),
	}.Build())
	secondUpdate.Header().Set("Authorization", "Bearer "+token)
	secondUpdate.Header().Set("Fun-Organization", orgID.String())

	_, err = client.UpdateOrganizationLimits(context.Background(), secondUpdate)
	require.NoError(t, err)

	getReq := connect.NewRequest(organizationv1.GetOrganizationLimitsRequest_builder{Id: orgID.String()}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	getRes, err := client.GetOrganizationLimits(context.Background(), getReq)
	require.NoError(t, err)

	assert.EqualValues(t, 100, getRes.Msg.GetLimits().GetMaxNodesPerCluster())
}

func Test_OrganizationLimits_Update_IsolatedBetweenOrgs(t *testing.T) {
	t.Parallel()

	org1ID := uuid.New()
	org2ID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(org1ID, "org-one"),
		WithOrganization(org2ID, "org-two"),
		WithUser(&UserArgs{ID: user1ID, Name: "user-one", OrgIDs: []uuid.UUID{org1ID}}),
		WithUser(&UserArgs{ID: user2ID, Name: "user-two", OrgIDs: []uuid.UUID{org2ID}}),
	)

	token1 := env.createAuthnToken(t, user1ID)
	token2 := env.createAuthnToken(t, user2ID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	updateReq := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                 org1ID.String(),
		MaxNodesPerCluster: proto.Int32(42),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token1)
	updateReq.Header().Set("Fun-Organization", org1ID.String())

	_, err := client.UpdateOrganizationLimits(context.Background(), updateReq)
	require.NoError(t, err)

	// org2 should see no limits
	getReq := connect.NewRequest(organizationv1.GetOrganizationLimitsRequest_builder{Id: org2ID.String()}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token2)
	getReq.Header().Set("Fun-Organization", org2ID.String())

	getRes, err := client.GetOrganizationLimits(context.Background(), getReq)
	require.NoError(t, err)
	assert.False(t, getRes.Msg.GetLimits().HasMaxNodesPerCluster())
}

func Test_OrganizationLimits_Update_MemoryLimitLessThanRequest(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                     orgID.String(),
		DefaultMemoryRequestMi: proto.Int32(256),
		DefaultMemoryLimitMi:   proto.Int32(128),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateOrganizationLimits(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func Test_OrganizationLimits_Update_CpuLimitLessThanRequest(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(organizationv1.UpdateOrganizationLimitsRequest_builder{
		Id:                 orgID.String(),
		DefaultCpuRequestM: proto.Int32(500),
		DefaultCpuLimitM:   proto.Int32(100),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateOrganizationLimits(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
