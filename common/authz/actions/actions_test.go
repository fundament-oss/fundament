package actions_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/actions"
)

// fixture holds one object of every type, linked as in production, and one
// user per role.
type fixture struct {
	org, cluster, nodePool, project, projectMember, namespace, apiKey, plugin uuid.UUID

	orgAdmin, orgMember, projectAdmin, projectMemberUser, keyOwner, stranger string
}

func (f *fixture) objectID(t *testing.T, a authzv1.Action) uuid.UUID {
	t.Helper()
	switch actions.Object(a, uuid.Nil).Type {
	case authz.ObjectTypeOrganization:
		return f.org
	case authz.ObjectTypeCluster:
		return f.cluster
	case authz.ObjectTypeNodePool:
		return f.nodePool
	case authz.ObjectTypeProject:
		return f.project
	case authz.ObjectTypeProjectMember:
		return f.projectMember
	case authz.ObjectTypeNamespace:
		return f.namespace
	case authz.ObjectTypeApiKey:
		return f.apiKey
	case authz.ObjectTypePlugin:
		return f.plugin
	case authz.ObjectTypeUser:
		t.Fatalf("action %s maps to a user object", a)
	}
	t.Fatalf("action %s maps to an unknown object type", a)
	return uuid.Nil
}

func (f *fixture) userFor(role authzv1.RequiredRole) string {
	switch role {
	case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN:
		return f.orgAdmin
	case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER:
		return f.orgMember
	case authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN:
		return f.projectAdmin
	case authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER:
		return f.projectMemberUser
	case authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER:
		return f.keyOwner
	case authzv1.RequiredRole_REQUIRED_ROLE_UNSPECIFIED:
		panic("unspecified role")
	default:
		panic("unknown role")
	}
}

func newFixture() *fixture {
	user := func() string { return "user:" + uuid.NewString() }
	return &fixture{
		org: uuid.New(), cluster: uuid.New(), nodePool: uuid.New(), project: uuid.New(),
		projectMember: uuid.New(), namespace: uuid.New(), apiKey: uuid.New(), plugin: uuid.New(),
		orgAdmin: user(), orgMember: user(), projectAdmin: user(),
		projectMemberUser: user(), keyOwner: user(), stranger: user(),
	}
}

// checker returns a function that asks an in-process OpenFGA, loaded with the
// fixture tuples, whether user holds relation on object.
func checker(t *testing.T, f *fixture) func(t *testing.T, user, relation, object string) bool {
	t.Helper()
	ctx := context.Background()

	fga := server.MustNewServerWithOpts(server.WithDatastore(memory.New()))
	t.Cleanup(fga.Close)

	store, err := fga.CreateStore(ctx, &openfgav1.CreateStoreRequest{Name: "actions-test"})
	require.NoError(t, err)

	model, err := transformer.TransformDSLToProto(authz.ModelDSL)
	require.NoError(t, err)

	written, err := fga.WriteAuthorizationModel(ctx, &openfgav1.WriteAuthorizationModelRequest{
		StoreId:         store.GetId(),
		TypeDefinitions: model.GetTypeDefinitions(),
		SchemaVersion:   model.GetSchemaVersion(),
		Conditions:      model.GetConditions(),
	})
	require.NoError(t, err)

	tuple := func(user, relation, object string) *openfgav1.TupleKey {
		return &openfgav1.TupleKey{User: user, Relation: relation, Object: object}
	}
	org := "organization:" + f.org.String()
	cluster := "cluster:" + f.cluster.String()
	project := "project:" + f.project.String()
	_, err = fga.Write(ctx, &openfgav1.WriteRequest{
		StoreId: store.GetId(),
		Writes: &openfgav1.WriteRequestWrites{TupleKeys: []*openfgav1.TupleKey{
			tuple(f.orgAdmin, "admin", org),
			tuple(f.orgMember, "viewer", org),
			tuple(org, "owner", cluster),
			tuple(cluster, "parent", "node_pool:"+f.nodePool.String()),
			tuple(cluster, "parent", project),
			tuple(project, "parent", "project_member:"+f.projectMember.String()),
			tuple(project, "parent", "namespace:"+f.namespace.String()),
			tuple(f.projectAdmin, "project_admin", project),
			tuple(f.projectMemberUser, "project_viewer", project),
			tuple(org, "owner", "api_key:"+f.apiKey.String()),
			tuple(f.keyOwner, "creator", "api_key:"+f.apiKey.String()),
			tuple(org, "owner", "plugin:"+f.plugin.String()),
		}},
	})
	require.NoError(t, err)

	return func(t *testing.T, user, relation, object string) bool {
		t.Helper()
		resp, err := fga.Check(t.Context(), &openfgav1.CheckRequest{
			StoreId:              store.GetId(),
			AuthorizationModelId: written.GetAuthorizationModelId(),
			TupleKey:             &openfgav1.CheckRequestTupleKey{User: user, Relation: relation, Object: object},
		})
		require.NoError(t, err, "%s %s %s", user, relation, object)
		return resp.GetAllowed()
	}
}

func TestEveryActionIsMapped(t *testing.T) {
	all := actions.All()
	values := authzv1.Action_ACTION_UNSPECIFIED.Descriptor().Values()
	assert.Len(t, all, values.Len()-1, "All must list every enum value except the zero value")
	assert.NotContains(t, all, authzv1.Action_ACTION_UNSPECIFIED)

	for _, a := range all {
		assert.NotPanics(t, func() {
			assert.NotEmpty(t, actions.Relation(a).Name, a.String())
			assert.NotEmpty(t, actions.Object(a, uuid.New()).Type, a.String())
			assert.NotEqual(t, authzv1.RequiredRole_REQUIRED_ROLE_UNSPECIFIED, actions.Role(a), a.String())
		}, a.String())
	}
}

func TestUnknownActionPanics(t *testing.T) {
	assert.Panics(t, func() { actions.Relation(authzv1.Action_ACTION_UNSPECIFIED) })
	assert.Panics(t, func() { actions.Role(authzv1.Action(9999)) })
}

func TestConstantsAliasTheEnum(t *testing.T) {
	assert.Equal(t, authzv1.Action_ACTION_CLUSTER_CAN_DELETE, actions.ClusterCanDelete)
	assert.Equal(t, authzv1.Action_ACTION_ORGANIZATION_CAN_CREATE_APIKEY, actions.OrganizationCanCreateApikey)
}

func TestRequiredRoleIsSufficient(t *testing.T) {
	f := newFixture()
	check := checker(t, f)

	for _, a := range actions.All() {
		t.Run(a.String(), func(t *testing.T) {
			user := f.userFor(actions.Role(a))
			obj := actions.Object(a, f.objectID(t, a))
			assert.True(t, check(t, user, string(actions.Relation(a).Name), obj.String()),
				"role %s must allow %s", actions.Role(a), a)
		})
	}
}

func TestRoleBelowIsDenied(t *testing.T) {
	f := newFixture()
	check := checker(t, f)

	for _, a := range actions.All() {
		t.Run(a.String(), func(t *testing.T) {
			relation := string(actions.Relation(a).Name)
			obj := actions.Object(a, f.objectID(t, a)).String()

			assert.False(t, check(t, f.stranger, relation, obj), "unrelated user must be denied")

			switch actions.Role(a) {
			case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN:
				assert.False(t, check(t, f.orgMember, relation, obj), "organization viewer must be denied")
			case authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN:
				assert.False(t, check(t, f.projectMemberUser, relation, obj), "project viewer must be denied")
			case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER,
				authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER,
				authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER:
			case authzv1.RequiredRole_REQUIRED_ROLE_UNSPECIFIED:
				t.Fatal("unspecified role")
			default:
				t.Fatal("unknown role")
			}
		})
	}
}
