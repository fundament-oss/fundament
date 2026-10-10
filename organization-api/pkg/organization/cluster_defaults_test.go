package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"google.golang.org/protobuf/proto"
)

// defaultsEnv is the setup every defaults test needs: an organization, an
// admin, a cluster and a project in it.
type defaultsEnv struct {
	env       *testEnv
	token     string
	orgID     uuid.UUID
	clusterID string
	projectID string
	clusters  organizationv1connect.ClusterServiceClient
	projects  organizationv1connect.ProjectServiceClient
}

func newDefaultsEnv(t *testing.T) *defaultsEnv {
	t.Helper()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	d := &defaultsEnv{
		env:      env,
		token:    env.createAuthnToken(t, userID),
		orgID:    orgID,
		clusters: organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL),
		projects: organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL),
	}

	clusterRes, err := d.clusters.CreateCluster(d.ctx(), organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)
	d.clusterID = clusterRes.GetClusterId()

	projectRes, err := d.projects.CreateProject(d.ctx(), organizationv1.CreateProjectRequest_builder{
		ClusterId: d.clusterID, Name: "test-project",
	}.Build())
	require.NoError(t, err)
	d.projectID = projectRes.GetProjectId()

	return d
}

// ctx returns a client context carrying the token and the organization header.
func (d *defaultsEnv) ctx() context.Context {
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+d.token)
	callInfo.RequestHeader().Set("Fun-Organization", d.orgID.String())
	return ctx
}

func (d *defaultsEnv) setClusterDefaults(t *testing.T, defaults *organizationv1.ContainerDefaults) error {
	t.Helper()
	_, err := d.clusters.UpdateClusterDefaults(d.ctx(), organizationv1.UpdateClusterDefaultsRequest_builder{
		ClusterId: d.clusterID, Defaults: defaults,
	}.Build())
	return err
}

func (d *defaultsEnv) setProjectDefaults(t *testing.T, defaults *organizationv1.ContainerDefaults) error {
	t.Helper()
	_, err := d.projects.UpdateProjectDefaults(d.ctx(), organizationv1.UpdateProjectDefaultsRequest_builder{
		ProjectId: d.projectID, Defaults: defaults,
	}.Build())
	return err
}

func (d *defaultsEnv) clusterDefaults(t *testing.T) *organizationv1.GetClusterDefaultsResponse {
	t.Helper()
	res, err := d.clusters.GetClusterDefaults(d.ctx(), organizationv1.GetClusterDefaultsRequest_builder{
		ClusterId: d.clusterID,
	}.Build())
	require.NoError(t, err)
	return res
}

func (d *defaultsEnv) projectDefaults(t *testing.T) *organizationv1.GetProjectDefaultsResponse {
	t.Helper()
	res, err := d.projects.GetProjectDefaults(d.ctx(), organizationv1.GetProjectDefaultsRequest_builder{
		ProjectId: d.projectID,
	}.Build())
	require.NoError(t, err)
	return res
}

// containerDefaults builds the message from values a nil pointer leaves absent.
func containerDefaults(memoryRequestMi, memoryLimitMi, cpuRequestM, cpuLimitM *int32) *organizationv1.ContainerDefaults {
	return organizationv1.ContainerDefaults_builder{
		MemoryRequestMi: memoryRequestMi,
		MemoryLimitMi:   memoryLimitMi,
		CpuRequestM:     cpuRequestM,
		CpuLimitM:       cpuLimitM,
	}.Build()
}

func Test_ClusterDefaults_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetClusterDefaults(context.Background(),
		organizationv1.GetClusterDefaultsRequest_builder{ClusterId: uuid.New().String()}.Build(),
	)

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// A new cluster starts with everything unset, and the suggested values are
// returned regardless so the console can offer them.
func Test_ClusterDefaults_Get_NothingSet(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	res := d.clusterDefaults(t)

	defaults := res.GetDefaults()
	require.NotNil(t, defaults)
	assert.False(t, defaults.HasMemoryRequestMi())
	assert.False(t, defaults.HasMemoryLimitMi())
	assert.False(t, defaults.HasCpuRequestM())
	assert.False(t, defaults.HasCpuLimitM())

	suggested := res.GetSuggested()
	require.NotNil(t, suggested)
	assert.EqualValues(t, 256, suggested.GetMemoryRequestMi())
	assert.EqualValues(t, 512, suggested.GetMemoryLimitMi())
	assert.EqualValues(t, 100, suggested.GetCpuRequestM())
	assert.EqualValues(t, 500, suggested.GetCpuLimitM())
}

func Test_ClusterDefaults_UpdateThenGet(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(
		proto.Int32(256), proto.Int32(512), proto.Int32(100), proto.Int32(500))))

	defaults := d.clusterDefaults(t).GetDefaults()
	assert.EqualValues(t, 256, defaults.GetMemoryRequestMi())
	assert.EqualValues(t, 512, defaults.GetMemoryLimitMi())
	assert.EqualValues(t, 100, defaults.GetCpuRequestM())
	assert.EqualValues(t, 500, defaults.GetCpuLimitM())
}

// The update replaces the whole set, so an absent field clears that default
// rather than keeping the stored value.
func Test_ClusterDefaults_Update_AbsentFieldsClear(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(
		proto.Int32(256), proto.Int32(512), proto.Int32(100), proto.Int32(500))))

	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, proto.Int32(100), nil)))

	defaults := d.clusterDefaults(t).GetDefaults()
	assert.False(t, defaults.HasMemoryRequestMi())
	assert.False(t, defaults.HasMemoryLimitMi())
	assert.EqualValues(t, 100, defaults.GetCpuRequestM())
	assert.False(t, defaults.HasCpuLimitM())
}

// A request above the limit inside one message never reaches the check
// constraint: the message's own CEL rule rejects it first. Either way the
// caller sees InvalidArgument.
func Test_ClusterDefaults_Update_RequestAboveLimit(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)

	err := d.setClusterDefaults(t, containerDefaults(nil, nil, proto.Int32(500), proto.Int32(100)))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	err = d.setClusterDefaults(t, containerDefaults(proto.Int32(512), proto.Int32(256), nil, nil))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// Lowering a cluster default below an active project's is refused, and the
// message names the project to lower first.
func Test_ClusterDefaults_Update_BelowProjectRefused(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(500))))
	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(400))))

	err := d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(300)))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "test-project")
	assert.Contains(t, err.Error(), "400m")

	// The project is unchanged: a refused update writes nothing.
	assert.EqualValues(t, 500, d.clusterDefaults(t).GetDefaults().GetCpuLimitM())
	assert.EqualValues(t, 400, d.projectDefaults(t).GetDefaults().GetCpuLimitM())
}

// A soft-deleted project is not a reason to refuse: only active projects count.
func Test_ClusterDefaults_Update_DeletedProjectIgnored(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(500))))
	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(400))))

	_, err := d.projects.DeleteProject(d.ctx(), organizationv1.DeleteProjectRequest_builder{
		ProjectId: d.projectID,
	}.Build())
	require.NoError(t, err)

	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(300))))
	assert.EqualValues(t, 300, d.clusterDefaults(t).GetDefaults().GetCpuLimitM())
}

func Test_ClusterDefaults_Update_UnknownCluster(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	_, err := d.clusters.UpdateClusterDefaults(d.ctx(), organizationv1.UpdateClusterDefaultsRequest_builder{
		ClusterId: uuid.New().String(),
		Defaults:  containerDefaults(nil, nil, proto.Int32(100), nil),
	}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
