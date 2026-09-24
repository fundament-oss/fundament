package organization_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// namespaceNamingEnv is an organization with one cluster and two projects whose
// names collided under a single-hyphen scheme: "a-b" and "a".
type namespaceNamingEnv struct {
	client   organizationv1connect.NamespaceServiceClient
	ctx      func() context.Context
	projectA string // "a"
	projectB string // "a-b"
	// createProject makes another project on the cluster.
	createProject func(name string) string
	// renameProject sets a name the API no longer accepts, as projects created
	// before "--" was reserved may have.
	renameProject func(projectID, name string)
}

func newNamespaceNamingEnv(t *testing.T) *namespaceNamingEnv {
	t.Helper()
	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)
	token := env.createAuthnToken(t, userID)
	authed := func() context.Context {
		ctx, callInfo := connect.NewClientContext(context.Background())
		callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
		return ctx
	}

	cluster, err := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL).
		CreateCluster(authed(), organizationv1.CreateClusterRequest_builder{
			Name: "naming", Region: "eu-west-1", KubernetesVersion: "1.28",
		}.Build())
	require.NoError(t, err)

	projects := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	create := func(name string) string {
		res, err := projects.CreateProject(authed(), organizationv1.CreateProjectRequest_builder{
			ClusterId: cluster.GetClusterId(), Name: name,
		}.Build())
		require.NoError(t, err)
		return res.GetProjectId()
	}

	return &namespaceNamingEnv{
		client:        organizationv1connect.NewNamespaceServiceClient(env.server.Client(), env.server.URL),
		ctx:           authed,
		projectA:      create("a"),
		projectB:      create("a-b"),
		createProject: create,
		renameProject: func(projectID, name string) {
			_, err := env.adminPool.Exec(t.Context(),
				"UPDATE tenant.projects SET name = $1 WHERE id = $2", name, projectID)
			require.NoError(t, err)
		},
	}
}

func (e *namespaceNamingEnv) create(projectID, name string) error {
	_, err := e.client.CreateNamespace(e.ctx(), organizationv1.CreateNamespaceRequest_builder{
		ProjectId: projectID, Name: name,
	}.Build())
	if err != nil {
		return fmt.Errorf("create namespace %q: %w", name, err)
	}
	return nil
}

func requireCode(t *testing.T, err error, code connect.Code, msg string) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, code, connectErr.Code())
	assert.Contains(t, connectErr.Message(), msg)
}

// The "--" separator keeps "a-b"+"c" (tnt-a-b--c) and "a"+"b-c" (tnt-a--b-c)
// apart, while an in-project duplicate is still refused.
func Test_Namespace_Create_SeparatorPreventsCollision(t *testing.T) {
	t.Parallel()
	e := newNamespaceNamingEnv(t)

	require.NoError(t, e.create(e.projectB, "c"))
	require.NoError(t, e.create(e.projectA, "b-c"))
	requireCode(t, e.create(e.projectB, "c"), connect.CodeAlreadyExists, "already exists in this project")
}

// A project named before "--" was reserved can still collide: "x--y"+"z" and
// "x"+"y--z" are both tnt-x--y--z. The second is refused.
func Test_Namespace_Create_LegacyProjectCollision(t *testing.T) {
	t.Parallel()
	e := newNamespaceNamingEnv(t)
	legacy := e.createProject("legacy")
	e.renameProject(legacy, "x--y")
	plain := e.createProject("x")

	require.NoError(t, e.create(legacy, "z"))
	requireCode(t, e.create(plain, "y--z"), connect.CodeAlreadyExists, `would be named "tnt-x--y--z" on the cluster`)
}

// The namespace name may use whatever is left of 63 chars after "tnt-<project>--".
func Test_Namespace_Create_LengthDependsOnProject(t *testing.T) {
	t.Parallel()
	e := newNamespaceNamingEnv(t)

	require.NoError(t, e.create(e.projectB, strings.Repeat("n", 54)), `"tnt-a-b--" leaves 54`)
	requireCode(t, e.create(e.projectB, strings.Repeat("m", 55)), connect.CodeInvalidArgument, "max 54 (project name + namespace name may be at most 57)")
	require.NoError(t, e.create(e.projectA, strings.Repeat("n", 56)), `"tnt-a--" leaves 56`)
}
