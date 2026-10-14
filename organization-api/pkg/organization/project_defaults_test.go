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

func Test_ProjectDefaults_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetProjectDefaults(context.Background(),
		organizationv1.GetProjectDefaultsRequest_builder{ProjectId: uuid.New().String()}.Build(),
	)

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// The read carries the cluster's values too: the console needs them to show
// what an unset field inherits and how far a custom value may go.
func Test_ProjectDefaults_Get_ReturnsOwnAndClusterValues(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(
		proto.Int32(256), proto.Int32(512), proto.Int32(100), proto.Int32(500))))
	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(250))))

	res := d.projectDefaults(t)

	own := res.GetDefaults()
	assert.False(t, own.HasMemoryRequestMi())
	assert.False(t, own.HasMemoryLimitMi())
	assert.False(t, own.HasCpuRequestM())
	assert.EqualValues(t, 250, own.GetCpuLimitM())

	cluster := res.GetClusterDefaults()
	assert.EqualValues(t, 256, cluster.GetMemoryRequestMi())
	assert.EqualValues(t, 512, cluster.GetMemoryLimitMi())
	assert.EqualValues(t, 100, cluster.GetCpuRequestM())
	assert.EqualValues(t, 500, cluster.GetCpuLimitM())
}

// A project may match the cluster but not exceed it, per field.
func Test_ProjectDefaults_Update_AgainstClusterCeiling(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(500))))

	err := d.setProjectDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(600)))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "600m")
	assert.Contains(t, err.Error(), "500m")

	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(500))))
	assert.EqualValues(t, 500, d.projectDefaults(t).GetDefaults().GetCpuLimitM())
}

// An unset cluster field is no ceiling at all.
func Test_ProjectDefaults_Update_UnsetClusterFieldIsNoCeiling(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setProjectDefaults(t, containerDefaults(
		proto.Int32(1024), proto.Int32(2048), proto.Int32(2000), proto.Int32(4000))))

	own := d.projectDefaults(t).GetDefaults()
	assert.EqualValues(t, 1024, own.GetMemoryRequestMi())
	assert.EqualValues(t, 4000, own.GetCpuLimitM())
}

// Neither row breaks its own limit >= request constraint, but the effective
// pair does: the project's request against the cluster's limit.
func Test_ProjectDefaults_Update_EffectiveRequestAboveEffectiveLimit(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, nil, proto.Int32(200))))

	err := d.setProjectDefaults(t, containerDefaults(nil, nil, proto.Int32(300), nil))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "effective")
	assert.Contains(t, err.Error(), "300m")
	assert.Contains(t, err.Error(), "200m")
}

// The update replaces the whole set, so an absent field goes back to
// inheriting the cluster's value.
func Test_ProjectDefaults_Update_AbsentFieldsInherit(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	require.NoError(t, d.setClusterDefaults(t, containerDefaults(nil, nil, proto.Int32(100), proto.Int32(500))))
	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, proto.Int32(50), proto.Int32(250))))

	require.NoError(t, d.setProjectDefaults(t, containerDefaults(nil, nil, nil, nil)))

	own := d.projectDefaults(t).GetDefaults()
	assert.False(t, own.HasCpuRequestM())
	assert.False(t, own.HasCpuLimitM())
}

func Test_ProjectDefaults_Update_RequestAboveLimit(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	err := d.setProjectDefaults(t, containerDefaults(proto.Int32(512), proto.Int32(256), nil, nil))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func Test_ProjectDefaults_Update_UnknownProject(t *testing.T) {
	t.Parallel()

	d := newDefaultsEnv(t)
	_, err := d.projects.UpdateProjectDefaults(d.ctx(), organizationv1.UpdateProjectDefaultsRequest_builder{
		ProjectId: uuid.New().String(),
		Defaults:  containerDefaults(nil, nil, proto.Int32(100), nil),
	}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
