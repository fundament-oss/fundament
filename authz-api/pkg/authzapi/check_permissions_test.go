package authzapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authz-api/pkg/authzapi"
	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1/authzv1connect"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/actions"
	"github.com/fundament-oss/fundament/common/authz/authztest"
)

var jwtSecret = []byte("test-secret")

// fixture holds one object of every type, linked as in production, and users
// holding the organization admin, organization viewer and project viewer roles.
type fixture struct {
	org, cluster, nodePool, project, projectMember, namespace, apiKey, plugin uuid.UUID

	otherOrg, otherCluster uuid.UUID

	alice, bart, cindy uuid.UUID
}

func newFixture() *fixture {
	return &fixture{
		org: uuid.New(), cluster: uuid.New(), nodePool: uuid.New(), project: uuid.New(),
		projectMember: uuid.New(), namespace: uuid.New(), apiKey: uuid.New(), plugin: uuid.New(),
		otherOrg: uuid.New(), otherCluster: uuid.New(),
		alice: uuid.New(), bart: uuid.New(), cindy: uuid.New(),
	}
}

func (f *fixture) tuples() []authztest.Tuple {
	org := authz.Organization(f.org).String()
	cluster := authz.Cluster(f.cluster).String()
	project := authz.Project(f.project).String()

	return []authztest.Tuple{
		{User: authz.User(f.alice).String(), Relation: "admin", Object: org},
		{User: authz.User(f.bart).String(), Relation: "viewer", Object: org},
		{User: org, Relation: "owner", Object: cluster},
		{User: cluster, Relation: "parent", Object: "node_pool:" + f.nodePool.String()},
		{User: cluster, Relation: "parent", Object: project},
		{User: project, Relation: "parent", Object: "project_member:" + f.projectMember.String()},
		{User: project, Relation: "parent", Object: "namespace:" + f.namespace.String()},
		{User: authz.User(f.cindy).String(), Relation: "project_viewer", Object: project},
		{User: org, Relation: "owner", Object: "api_key:" + f.apiKey.String()},
		{User: org, Relation: "owner", Object: "plugin:" + f.plugin.String()},
		{User: authz.Organization(f.otherOrg).String(), Relation: "owner", Object: authz.Cluster(f.otherCluster).String()},
	}
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

type env struct {
	*fixture
	fga    *authztest.Server
	client authzv1connect.AuthzServiceClient
}

func newEnv(t *testing.T) *env {
	t.Helper()

	f := newFixture()
	fga := authztest.Serve(t, f.tuples()...)

	store, err := authz.NewStore(authz.Config{APIURL: fga.URL, StoreName: authztest.StoreName})
	require.NoError(t, err)

	srv := authzapi.New(slog.New(slog.NewTextHandler(io.Discard, nil)),
		&authzapi.Config{JWTSecret: jwtSecret}, authz.NewClient(store))
	api := httptest.NewServer(srv.Handler())
	t.Cleanup(api.Close)

	return &env{fixture: f, fga: fga, client: authzv1connect.NewAuthzServiceClient(&http.Client{Transport: bearerTransport{}}, api.URL)}
}

func token(t *testing.T, userID uuid.UUID, audience string) string {
	t.Helper()

	claims := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    auth.ConsoleIssuer,
		Subject:   userID.String(),
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	require.NoError(t, err)

	return signed
}

type tokenKey struct{}

// bearerTransport sends the token carried by the request context, if any.
type bearerTransport struct{}

func (bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if tok, ok := r.Context().Value(tokenKey{}).(string); ok {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+tok)
	}

	return http.DefaultTransport.RoundTrip(r) //nolint:wrapcheck // test double
}

func (e *env) call(t *testing.T, tok string, checks ...*authzv1.PermissionCheck) (*authzv1.CheckPermissionsResponse, error) {
	t.Helper()

	ctx := t.Context()
	if tok != "" {
		ctx = context.WithValue(ctx, tokenKey{}, tok)
	}

	return e.client.CheckPermissions(ctx, authzv1.CheckPermissionsRequest_builder{Checks: checks}.Build()) //nolint:wrapcheck // tests assert on the raw connect error
}

func (e *env) check(t *testing.T, userID uuid.UUID, checks ...*authzv1.PermissionCheck) (*authzv1.CheckPermissionsResponse, error) {
	t.Helper()

	return e.call(t, token(t, userID, auth.TokenTypeUser), checks...)
}

func pc(a authzv1.Action, id uuid.UUID) *authzv1.PermissionCheck {
	return authzv1.PermissionCheck_builder{Action: a, ObjectId: id.String()}.Build()
}

func (e *env) allChecks(t *testing.T) ([]*authzv1.PermissionCheck, []authzv1.Action) {
	t.Helper()

	all := actions.All()
	checks := make([]*authzv1.PermissionCheck, 0, len(all))
	for _, a := range all {
		checks = append(checks, pc(a, e.objectID(t, a)))
	}

	return checks, all
}

func TestCheckPermissionsByRole(t *testing.T) {
	e := newEnv(t)
	checks, all := e.allChecks(t)

	t.Run("organization admin is allowed everything", func(t *testing.T) {
		resp, err := e.check(t, e.alice, checks...)
		require.NoError(t, err)
		require.Len(t, resp.GetAllowed(), len(all))
		for i, a := range all {
			assert.True(t, resp.GetAllowed()[i], a.String())
		}
	})

	t.Run("organization viewer is allowed exactly the organization member actions", func(t *testing.T) {
		resp, err := e.check(t, e.bart, checks...)
		require.NoError(t, err)
		require.Len(t, resp.GetAllowed(), len(all))
		for i, a := range all {
			assert.Equal(t, actions.Role(a) == authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER,
				resp.GetAllowed()[i], "%s (role %s)", a, actions.Role(a))
		}
	})

	t.Run("project viewer is allowed exactly the project member actions", func(t *testing.T) {
		resp, err := e.check(t, e.cindy, checks...)
		require.NoError(t, err)
		require.Len(t, resp.GetAllowed(), len(all))
		for i, a := range all {
			assert.Equal(t, actions.Role(a) == authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER,
				resp.GetAllowed()[i], "%s (role %s)", a, actions.Role(a))
		}
	})
}

func TestOrderPreserved(t *testing.T) {
	e := newEnv(t)

	resp, err := e.check(t, e.bart,
		pc(actions.OrganizationCanEdit, e.org),
		pc(actions.OrganizationCanView, e.org),
		pc(actions.OrganizationCanCreateCluster, e.org),
		pc(actions.OrganizationCanListClusters, e.org),
	)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false, true}, resp.GetAllowed())
}

func TestSubjectFromTokenOnly(t *testing.T) {
	e := newEnv(t)
	req := pc(actions.OrganizationCanEdit, e.org)

	alice, err := e.check(t, e.alice, req)
	require.NoError(t, err)
	bart, err := e.check(t, e.bart, req)
	require.NoError(t, err)

	assert.Equal(t, []bool{true}, alice.GetAllowed())
	assert.Equal(t, []bool{false}, bart.GetAllowed())
}

func TestOtherOrganizationObjectIsFalse(t *testing.T) {
	e := newEnv(t)

	resp, err := e.check(t, e.alice,
		pc(actions.ClusterCanView, e.otherCluster),
		pc(actions.OrganizationCanView, e.otherOrg),
		pc(actions.ClusterCanView, e.cluster),
	)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, true}, resp.GetAllowed())
}

func TestUnknownObjectIsFalse(t *testing.T) {
	e := newEnv(t)

	resp, err := e.check(t, e.alice, pc(actions.ClusterCanEdit, uuid.New()), pc(actions.OrganizationCanEdit, e.org))
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, resp.GetAllowed())
}

func TestEmptyRequestReturnsEmpty(t *testing.T) {
	e := newEnv(t)

	resp, err := e.check(t, e.alice)
	require.NoError(t, err)
	assert.Empty(t, resp.GetAllowed())
	assert.Zero(t, e.fga.BatchCheckCalls())
}

func TestOpenFGAFailureIsInternal(t *testing.T) {
	e := newEnv(t)

	// A first call resolves the store, so the failure below is in BatchCheck.
	resp, err := e.check(t, e.alice, pc(actions.OrganizationCanEdit, e.org))
	require.NoError(t, err)
	require.Equal(t, []bool{true}, resp.GetAllowed())

	e.fga.Close()

	_, err = e.check(t, e.alice, pc(actions.OrganizationCanEdit, e.org))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), e.fga.URL, "backend details must not leak")
}

func TestUnauthenticatedRejected(t *testing.T) {
	e := newEnv(t)
	check := pc(actions.OrganizationCanView, e.org)

	t.Run("no token", func(t *testing.T) {
		_, err := e.call(t, "", check)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("expired token", func(t *testing.T) {
		claims := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   e.alice.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		}}
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
		require.NoError(t, err)

		_, err = e.call(t, signed, check)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("wrong secret", func(t *testing.T) {
		claims := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   e.alice.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}}
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("other-secret"))
		require.NoError(t, err)

		_, err = e.call(t, signed, check)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("plugin token", func(t *testing.T) {
		_, err := e.call(t, token(t, e.alice, auth.TokenTypePlugin), check)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})
}

func TestOverLimitRejected(t *testing.T) {
	e := newEnv(t)
	checks := make([]*authzv1.PermissionCheck, 501)
	for i := range checks {
		checks[i] = pc(actions.OrganizationCanView, e.org)
	}

	_, err := e.check(t, e.alice, checks...)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestLargeBatchKeepsOrder(t *testing.T) {
	e := newEnv(t)
	checks := make([]*authzv1.PermissionCheck, 500)
	want := make([]bool, 500)
	for i := range checks {
		// bart may view the organization but not edit it.
		if i%2 == 0 {
			checks[i] = pc(actions.OrganizationCanView, e.org)
			want[i] = true
		} else {
			checks[i] = pc(actions.OrganizationCanEdit, e.org)
		}
	}

	resp, err := e.check(t, e.bart, checks...)
	require.NoError(t, err)
	assert.Equal(t, want, resp.GetAllowed())
	assert.Greater(t, e.fga.BatchCheckCalls(), 1)
}
