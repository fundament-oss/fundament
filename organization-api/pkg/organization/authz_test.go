package organization_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/actions"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// These tests measure elapsed time, so none of them runs in parallel.
const (
	atOnce     = 500 * time.Millisecond
	shortRetry = 300 * time.Millisecond
)

func alwaysDeny(authz.EvaluationRequest) (authz.Decision, error) {
	return authz.Decision{Decision: false}, nil
}

// denyTimes denies the first n evaluations and allows the rest.
func denyTimes(n int32) func(authz.EvaluationRequest) (authz.Decision, error) {
	var calls atomic.Int32
	return func(authz.EvaluationRequest) (authz.Decision, error) {
		return authz.Decision{Decision: calls.Add(1) > n}, nil
	}
}

type gateEnv struct {
	*testEnv
	orgID          uuid.UUID
	userID         uuid.UUID
	otherUserID    uuid.UUID
	clusterID      uuid.UUID
	otherClusterID uuid.UUID
	projectID      uuid.UUID
	otherProjectID uuid.UUID
	ctx            context.Context
}

func newGateEnv(t *testing.T, decide func(authz.EvaluationRequest) (authz.Decision, error)) *gateEnv {
	t.Helper()

	orgID, userID, otherUserID := uuid.New(), uuid.New(), uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "gate-org"),
		WithUser(&UserArgs{ID: userID, Name: "caller", Email: "caller@example.com", OrgIDs: []uuid.UUID{orgID}}),
		WithUser(&UserArgs{ID: otherUserID, Name: "other", Email: "other@example.com", OrgIDs: []uuid.UUID{orgID}}),
		WithServerOptions(organization.WithDecisions(decide)),
	)

	g := &gateEnv{
		testEnv:        env,
		orgID:          orgID,
		userID:         userID,
		otherUserID:    otherUserID,
		clusterID:      uuid.New(),
		otherClusterID: uuid.New(),
		projectID:      uuid.New(),
		otherProjectID: uuid.New(),
		ctx:            organization.WithUserID(organization.WithOrganizationID(context.Background(), orgID), userID),
	}

	for _, clusterID := range []uuid.UUID{g.clusterID, g.otherClusterID} {
		_, err := env.adminPool.Exec(t.Context(),
			"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version) VALUES ($1, $2, $3, 'eu-west-1', '1.28')",
			clusterID, orgID, "cluster-"+clusterID.String()[:8])
		require.NoError(t, err)
	}
	g.seedProject(t, g.projectID, g.clusterID, userID, otherUserID)
	g.seedProject(t, g.otherProjectID, g.otherClusterID, otherUserID)

	g.completeOutbox(t)
	return g
}

// seedProject inserts the project and its admins together: a project must have one at commit.
func (g *gateEnv) seedProject(t *testing.T, projectID, clusterID uuid.UUID, admins ...uuid.UUID) {
	t.Helper()
	err := pgx.BeginFunc(t.Context(), g.adminPool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(),
			"INSERT INTO tenant.projects (id, cluster_id, name, alias) VALUES ($1, $2, $3, $3)",
			projectID, clusterID, "project-"+projectID.String()[:8])
		if err != nil {
			return err //nolint:wrapcheck // test helper
		}
		for _, admin := range admins {
			_, err := tx.Exec(t.Context(),
				"INSERT INTO tenant.project_members (project_id, user_id, role) VALUES ($1, $2, 'admin')",
				projectID, admin)
			if err != nil {
				return err //nolint:wrapcheck // test helper
			}
		}
		return nil
	})
	require.NoError(t, err)
}

// completeOutbox marks every row synced, including those the seed's triggers wrote.
func (g *gateEnv) completeOutbox(t *testing.T) {
	t.Helper()
	_, err := g.adminPool.Exec(t.Context(), "UPDATE authz.outbox SET status = 'completed', processed = now()")
	require.NoError(t, err)
}

func (g *gateEnv) addOutboxRow(t *testing.T, column string, id uuid.UUID, status string) {
	t.Helper()
	_, err := g.adminPool.Exec(t.Context(),
		"INSERT INTO authz.outbox ("+column+", status) VALUES ($1, $2)", id, status)
	require.NoError(t, err)
}

func (g *gateEnv) orgMembership(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, g.adminPool.QueryRow(t.Context(),
		"SELECT id FROM tenant.organizations_users WHERE organization_id = $1 AND user_id = $2",
		g.orgID, userID).Scan(&id))
	return id
}

func (g *gateEnv) projectMembership(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, g.adminPool.QueryRow(t.Context(),
		"SELECT id FROM tenant.project_members WHERE project_id = $1 AND user_id = $2",
		g.projectID, userID).Scan(&id))
	return id
}

func requireDenied(t *testing.T, err error, action authzv1.Action) {
	t.Helper()
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodePermissionDenied, cerr.Code())
	assert.Equal(t, "permission denied", cerr.Message())
	require.Len(t, cerr.Details(), 1)
	value, verr := cerr.Details()[0].Value()
	require.NoError(t, verr)
	detail, ok := value.(*authzv1.PermissionDeniedDetail)
	require.True(t, ok)
	assert.Equal(t, action, detail.GetAction())
	assert.Equal(t, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN, detail.GetRequiredRole())
}

func (g *gateEnv) clientContext(t *testing.T) context.Context {
	t.Helper()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+g.createAuthnToken(t, g.userID))
	callInfo.RequestHeader().Set("Fun-Organization", g.orgID.String())
	return ctx
}

// timedBudgetCheck runs a check with a short budget and reports how long it took.
func (g *gateEnv) timedBudgetCheck(action authzv1.Action, id uuid.UUID) (time.Duration, error) {
	start := time.Now()
	err := g.org.CheckPermissionWithBudget(g.ctx, action, id, shortRetry)
	return time.Since(start), err
}

func TestDenialHasDetail(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	client := organizationv1connect.NewClusterServiceClient(g.server.Client(), g.server.URL)
	_, err := client.DeleteCluster(g.clientContext(t), organizationv1.DeleteClusterRequest_builder{
		ClusterId: g.clusterID.String(),
	}.Build())

	requireDenied(t, err, authzv1.Action_ACTION_CLUSTER_CAN_DELETE)
}

func TestDeniedNothingPendingReturnsAtOnce(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	start := time.Now()
	err := g.org.CheckPermissionWithRetry(g.ctx, actions.ClusterCanCreateProject, g.clusterID)

	assert.Less(t, time.Since(start), atOnce)
	requireDenied(t, err, actions.ClusterCanCreateProject)
}

func TestDeniedCreateProjectRPCReturnsAtOnce(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	client := organizationv1connect.NewProjectServiceClient(g.server.Client(), g.server.URL)
	ctx := g.clientContext(t)

	start := time.Now()
	_, err := client.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: g.clusterID.String(),
		Name:      "denied",
	}.Build())

	assert.Less(t, time.Since(start), atOnce)
	requireDenied(t, err, actions.ClusterCanCreateProject)
}

func TestDeniedPendingRowRetriesUntilAllowed(t *testing.T) {
	g := newGateEnv(t, denyTimes(2))
	g.addOutboxRow(t, "cluster_id", g.clusterID, "pending")

	require.NoError(t, g.org.CheckPermissionWithRetry(g.ctx, actions.ClusterCanCreateProject, g.clusterID))
}

// With nothing pending the check is evaluated once more, since a row may have
// completed between the denial and the outbox read.
func TestDeniedNothingPendingEvaluatesOnceMore(t *testing.T) {
	g := newGateEnv(t, denyTimes(1))

	require.NoError(t, g.org.CheckPermissionWithRetry(g.ctx, actions.ClusterCanCreateProject, g.clusterID))
}

func TestDeniedFailedRowDoesNotWait(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)
	g.addOutboxRow(t, "cluster_id", g.clusterID, "failed")

	start := time.Now()
	err := g.org.CheckPermissionWithRetry(g.ctx, actions.ClusterCanCreateProject, g.clusterID)

	assert.Less(t, time.Since(start), atOnce)
	requireDenied(t, err, actions.ClusterCanCreateProject)
}

func TestRetryingRowWaitsOnlyForBudget(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)
	g.addOutboxRow(t, "cluster_id", g.clusterID, "retrying")

	elapsed, err := g.timedBudgetCheck(actions.ClusterCanCreateProject, g.clusterID)

	assert.GreaterOrEqual(t, elapsed, shortRetry)
	assert.Less(t, elapsed, shortRetry+atOnce)
	requireDenied(t, err, actions.ClusterCanCreateProject)
}

func TestOutboxQueryFailureReturnsDenial(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)
	_, err := g.adminPool.Exec(t.Context(), "REVOKE SELECT ON authz.outbox FROM fun_fundament_api")
	require.NoError(t, err)

	start := time.Now()
	err = g.org.CheckPermissionWithRetry(g.ctx, actions.ClusterCanCreateProject, g.clusterID)

	assert.Less(t, time.Since(start), atOnce)
	requireDenied(t, err, actions.ClusterCanCreateProject)
}

func TestClusterGateRows(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	tests := []struct {
		name   string
		column string
		id     uuid.UUID
		waits  bool
	}{
		{"the cluster", "cluster_id", g.clusterID, true},
		{"another cluster", "cluster_id", g.otherClusterID, false},
		{"the caller's organization membership", "organization_user_id", g.orgMembership(t, g.userID), true},
		{"another user's organization membership", "organization_user_id", g.orgMembership(t, g.otherUserID), false},
		{"a project on the cluster", "project_id", g.projectID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.completeOutbox(t)
			g.addOutboxRow(t, tt.column, tt.id, "pending")

			elapsed, err := g.timedBudgetCheck(actions.ClusterCanCreateProject, g.clusterID)

			require.Error(t, err)
			if tt.waits {
				assert.GreaterOrEqual(t, elapsed, shortRetry)
			} else {
				assert.Less(t, elapsed, atOnce)
			}
		})
	}
}

func TestProjectGatePendingMembership(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	tests := []struct {
		name   string
		column string
		id     uuid.UUID
		waits  bool
	}{
		{"the project", "project_id", g.projectID, true},
		{"another project", "project_id", g.otherProjectID, false},
		{"its cluster", "cluster_id", g.clusterID, true},
		{"another cluster", "cluster_id", g.otherClusterID, false},
		{"the caller's project membership", "project_member_id", g.projectMembership(t, g.userID), true},
		{"another user's project membership", "project_member_id", g.projectMembership(t, g.otherUserID), false},
		{"the caller's organization membership", "organization_user_id", g.orgMembership(t, g.userID), true},
		{"another user's organization membership", "organization_user_id", g.orgMembership(t, g.otherUserID), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.completeOutbox(t)
			g.addOutboxRow(t, tt.column, tt.id, "pending")

			elapsed, err := g.timedBudgetCheck(actions.ProjectCanCreateNamespace, g.projectID)

			require.Error(t, err)
			if tt.waits {
				assert.GreaterOrEqual(t, elapsed, shortRetry)
			} else {
				assert.Less(t, elapsed, atOnce)
			}
		})
	}
}

func TestDeniedRemovedMembershipDoesNotWait(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	projectMember := g.projectMembership(t, g.userID)
	_, err := g.adminPool.Exec(t.Context(), "UPDATE tenant.project_members SET deleted = now() WHERE id = $1", projectMember)
	require.NoError(t, err)

	orgMember := g.orgMembership(t, g.userID)
	_, err = g.adminPool.Exec(t.Context(), "UPDATE tenant.organizations_users SET deleted = now() WHERE id = $1", orgMember)
	require.NoError(t, err)

	tests := []struct {
		name   string
		column string
		rowID  uuid.UUID
		action authzv1.Action
		id     uuid.UUID
	}{
		{"project membership on project", "project_member_id", projectMember, actions.ProjectCanCreateNamespace, g.projectID},
		{"organization membership on project", "organization_user_id", orgMember, actions.ProjectCanCreateNamespace, g.projectID},
		{"organization membership on cluster", "organization_user_id", orgMember, actions.ClusterCanCreateProject, g.clusterID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.completeOutbox(t)
			g.addOutboxRow(t, tt.column, tt.rowID, "pending")

			elapsed, err := g.timedBudgetCheck(tt.action, tt.id)

			require.Error(t, err)
			assert.Less(t, elapsed, atOnce)
		})
	}
}

func TestRetryOnUngatedObjectPanics(t *testing.T) {
	g := newGateEnv(t, alwaysDeny)

	assert.Panics(t, func() {
		_ = g.org.CheckPermissionWithRetry(g.ctx, actions.OrganizationCanView, g.orgID)
	})
}
