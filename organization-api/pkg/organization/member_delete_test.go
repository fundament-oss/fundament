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
)

func Test_MemberDelete_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.DeleteMember(context.Background(), organizationv1.DeleteMemberRequest_builder{
		Id: "arbitrary",
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_MemberDelete(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	targetUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     targetUserID,
			Name:   "target-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	// List members to discover the target member's membership ID
	theReq := &organizationv1.ListMembersRequest{}
	theCtx, theCallInfo := connect.NewClientContext(context.Background())
	theCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	theCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(theCtx, theReq)
	require.NoError(t, err)

	var targetMemberID string
	for _, m := range listRes.GetMembers() {
		if m.GetUserId() == targetUserID.String() {
			targetMemberID = m.GetId()
			break
		}
	}
	require.NotEmpty(t, targetMemberID, "target member not found in ListMembers response")

	tests := map[string]struct {
		Request  *organizationv1.DeleteMemberRequest
		WantCode connect.Code
		WantErr  bool
	}{
		"not_found": {
			Request: organizationv1.DeleteMemberRequest_builder{
				Id: uuid.New().String(),
			}.Build(),
			WantCode: connect.CodeNotFound,
			WantErr:  true,
		},
		"happy_flow": {
			Request: organizationv1.DeleteMemberRequest_builder{
				Id: targetMemberID,
			}.Build(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := tc.Request
			ctx, callInfo := connect.NewClientContext(context.Background())
			callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
			callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

			res, err := client.DeleteMember(ctx, req)

			if tc.WantErr {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.WantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "", res.String())

			getReq := organizationv1.GetMemberRequest_builder{
				UserId: new(targetUserID.String()),
			}.Build()
			getCtx, getCallInfo := connect.NewClientContext(context.Background())
			getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
			getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

			_, err = client.GetMember(getCtx, getReq)

			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeNotFound, connectErr.Code())
		})
	}
}

// Removing a member from the organization ends their project memberships in
// the same transaction, unless they are the last admin of a project, in which
// case nothing is removed and the project is named.
func Test_MemberDelete_CascadesProjectMemberships(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	targetUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: callerUserID, Name: "caller-user", OrgIDs: []uuid.UUID{orgID}}),
		WithUser(&UserArgs{ID: targetUserID, Name: "target-user", OrgIDs: []uuid.UUID{orgID}}),
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

	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	createProjectRes, err := projectClient.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "shared-project",
	}.Build())
	require.NoError(t, err)
	_, err = projectClient.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: createProjectRes.GetProjectId(),
		UserId:    targetUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	require.NoError(t, err)

	// A project where the target is the only admin: created directly, since
	// CreateProject always makes the caller an admin.
	// The require-admin trigger checks at commit, so project and admin go in one transaction.
	soloTx, err := env.adminPool.Begin(t.Context())
	require.NoError(t, err)
	var soloProjectID uuid.UUID
	err = soloTx.QueryRow(t.Context(),
		"INSERT INTO tenant.projects (cluster_id, name, alias) VALUES ($1, 'solo-project', 'solo-project') RETURNING id",
		uuid.MustParse(createClusterRes.GetClusterId()),
	).Scan(&soloProjectID)
	require.NoError(t, err)
	_, err = soloTx.Exec(t.Context(),
		"INSERT INTO tenant.project_members (project_id, user_id, role) VALUES ($1, $2, 'admin')",
		soloProjectID, targetUserID,
	)
	require.NoError(t, err)
	require.NoError(t, soloTx.Commit(t.Context()))

	memberClient := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)
	member, err := memberClient.GetMember(ctx, organizationv1.GetMemberRequest_builder{UserId: new(targetUserID.String())}.Build())
	require.NoError(t, err)
	deleteReq := organizationv1.DeleteMemberRequest_builder{Id: member.GetMember().GetId()}.Build()

	activeMemberships := func() int {
		var n int
		err := env.adminPool.QueryRow(t.Context(),
			"SELECT count(*) FROM tenant.project_members WHERE user_id = $1 AND deleted IS NULL", targetUserID,
		).Scan(&n)
		require.NoError(t, err)
		return n
	}

	_, err = memberClient.DeleteMember(ctx, deleteReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "solo-project")
	assert.Equal(t, 2, activeMemberships(), "nothing removed while the target is a last admin")

	_, err = env.adminPool.Exec(t.Context(),
		"INSERT INTO tenant.project_members (project_id, user_id, role) VALUES ($1, $2, 'admin')",
		soloProjectID, callerUserID,
	)
	require.NoError(t, err)

	_, err = memberClient.DeleteMember(ctx, deleteReq)
	require.NoError(t, err)
	assert.Equal(t, 0, activeMemberships(), "project memberships end with the organization membership")
}
