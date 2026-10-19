package organization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// Removing both admins of a project at the same time leaves one of them: the
// last-admin trigger locks the project, so the second removal counts the
// admins only after the first has committed, and is refused.
func Test_ProjectMember_ConcurrentAdminRemovalsKeepAnAdmin(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	otherUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     otherUserID,
			Name:   "other-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	clusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	projectRes, err := projectClient.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build())
	require.NoError(t, err)
	projectID := projectRes.GetProjectId()

	_, err = projectClient.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    otherUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN,
	}.Build())
	require.NoError(t, err)

	firstAdmin := projectMemberID(ctx, t, projectClient, projectID, callerUserID)
	secondAdmin := projectMemberID(ctx, t, projectClient, projectID, otherUserID)

	const removeAdmin = `UPDATE tenant.project_members SET deleted = now() WHERE id = $1`

	first, err := env.adminPool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = first.Rollback(context.Background()) }()

	_, err = first.Exec(t.Context(), removeAdmin, firstAdmin)
	require.NoError(t, err, "the first removal leaves the other admin")

	second := make(chan error, 1)
	go func() {
		_, err := env.adminPool.Exec(t.Context(), removeAdmin, secondAdmin)
		second <- err
	}()

	require.Eventually(t, func() bool {
		var waiting bool
		err := env.adminPool.QueryRow(t.Context(), `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE pg_stat_activity.datname = current_database()
					AND pg_stat_activity.wait_event_type = 'Lock'
			)`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "the second removal waits for the first")

	require.NoError(t, first.Commit(t.Context()))

	err = <-second
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok, "the second removal is refused, got %v", err)
	assert.Equal(t, dbconst.HintProjectContainsOneAdmin, pgErr.Hint)

	var admins int
	err = env.adminPool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM tenant.project_members
		WHERE tenant.project_members.project_id = $1
			AND tenant.project_members.role = 'admin'
			AND tenant.project_members.deleted IS NULL`, projectID).Scan(&admins)
	require.NoError(t, err)
	assert.Equal(t, 1, admins)
}
