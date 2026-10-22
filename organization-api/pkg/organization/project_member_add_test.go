package organization_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ProjectMember_Add_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.AddProjectMember(context.Background(), organizationv1.AddProjectMemberRequest_builder{
		ProjectId: uuid.New().String(),
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_ProjectMember_Add(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	newMemberUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     newMemberUserID,
			Name:   "new-member",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createClusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)

	projectID := createProjectRes.GetProjectId()

	addReq := organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    newMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build()
	addCtx, addCallInfo := connect.NewClientContext(context.Background())
	addCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	addCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.AddProjectMember(addCtx, addReq)
	require.NoError(t, err)
	assert.NotEmpty(t, res.GetMemberId())

	// Adding the same user a second time must fail.
	_, err = client.AddProjectMember(addCtx, addReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())

	invalidRoleReq := organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED,
	}.Build()
	invalidRoleCtx, invalidRoleCallInfo := connect.NewClientContext(context.Background())
	invalidRoleCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	invalidRoleCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.AddProjectMember(invalidRoleCtx, invalidRoleReq)
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// Only members of the project's organization can become project members: a
// user of another organization and a user whose membership was declined or
// revoked are refused, while an invited (pending) user is accepted, as in
// the console.
func Test_ProjectMember_Add_RefusesUsersOutsideOrganization(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	otherOrgID := uuid.New()
	callerUserID := uuid.New()
	outsiderUserID := uuid.New()
	pendingUserID := uuid.New()
	declinedUserID := uuid.New()
	revokedUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithOrganization(otherOrgID, "other-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     outsiderUserID,
			Name:   "outsider",
			OrgIDs: []uuid.UUID{otherOrgID},
		}),
		WithUser(&UserArgs{ID: pendingUserID, Name: "pending-invitee"}),
		WithUser(&UserArgs{ID: declinedUserID, Name: "declined-invitee"}),
		WithUser(&UserArgs{ID: revokedUserID, Name: "revoked-member"}),
	)

	for userID, status := range map[uuid.UUID]string{pendingUserID: "pending", declinedUserID: "declined", revokedUserID: "revoked"} {
		_, err := env.adminPool.Exec(t.Context(),
			"INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status) VALUES ($1, $2, 'viewer', $3)",
			orgID, userID, status,
		)
		require.NoError(t, err)
	}

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createClusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	createProjectRes, err := client.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build())
	require.NoError(t, err)
	projectID := createProjectRes.GetProjectId()

	for name, userID := range map[string]uuid.UUID{
		"member of another organization": outsiderUserID,
		"declined invitee":               declinedUserID,
		"revoked member":                 revokedUserID,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
				ProjectId: projectID,
				UserId:    userID.String(),
				Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
			}.Build())

			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
			assert.Contains(t, connectErr.Message(), "must first be added to the organization")

			var count int
			err = env.adminPool.QueryRow(t.Context(),
				"SELECT count(*) FROM tenant.project_members WHERE project_id = $1 AND user_id = $2",
				projectID, userID,
			).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count, "no membership row written")
		})
	}

	// An invited member may be added before accepting; cluster-worker only
	// provisions them once they accept.
	res, err := client.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    pendingUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	require.NoError(t, err)
	assert.NotEmpty(t, res.GetMemberId())
}

// The membership check locks the organization membership row: an add that
// runs while that member is being removed waits for the removal and then
// sees it, instead of inserting from a snapshot taken before it.
func Test_ProjectMember_Add_WaitsForConcurrentMemberRemoval(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	memberUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     memberUserID,
			Name:   "leaving-member",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createClusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	createProjectRes, err := client.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build())
	require.NoError(t, err)

	// Removal in flight: the membership row is updated but not yet committed.
	removal, err := env.adminPool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = removal.Rollback(context.Background()) }()
	_, err = removal.Exec(t.Context(),
		"UPDATE tenant.organizations_users SET deleted = now(), status = 'revoked' WHERE organization_id = $1 AND user_id = $2",
		orgID, memberUserID,
	)
	require.NoError(t, err)

	addErr := make(chan error, 1)
	go func() {
		_, err := client.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
			ProjectId: createProjectRes.GetProjectId(),
			UserId:    memberUserID.String(),
			Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
		}.Build())
		addErr <- err
	}()

	// The add must block on the locked row rather than answer from its snapshot.
	select {
	case err := <-addErr:
		t.Fatalf("AddProjectMember returned before the removal committed: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	require.NoError(t, removal.Commit(t.Context()))

	var connectErr *connect.Error
	require.ErrorAs(t, <-addErr, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// A missing or deleted project is reported as such (FUN-12), not as the user
// lacking organization membership; a soft-deleted user is refused even when
// an accepted membership row lingers.
func Test_ProjectMember_Add_DeletedProjectAndDeletedUser(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	memberUserID := uuid.New()
	deletedUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: callerUserID, Name: "caller-user", OrgIDs: []uuid.UUID{orgID}}),
		WithUser(&UserArgs{ID: memberUserID, Name: "member", OrgIDs: []uuid.UUID{orgID}}),
		WithUser(&UserArgs{ID: deletedUserID, Name: "deleted-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createClusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	createProjectRes, err := client.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build())
	require.NoError(t, err)
	projectID := createProjectRes.GetProjectId()

	_, err = env.adminPool.Exec(t.Context(), "UPDATE tenant.users SET deleted = now() WHERE id = $1", deletedUserID)
	require.NoError(t, err)

	var connectErr *connect.Error
	_, err = client.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    deletedUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code(), "soft-deleted user")

	// Soft-delete the project underneath the authorization check.
	_, err = env.adminPool.Exec(t.Context(), "UPDATE tenant.projects SET deleted = now() WHERE id = $1", projectID)
	require.NoError(t, err)

	_, err = client.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    memberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(), "deleted project")
}
