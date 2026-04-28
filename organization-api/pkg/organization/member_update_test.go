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

func Test_Member_Update_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.UpdateMemberPermission(context.Background(), connect.NewRequest(organizationv1.UpdateMemberPermissionRequest_builder{
		Id:         uuid.New().String(),
		Permission: "viewer",
	}.Build()))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Member_Update(t *testing.T) {
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

	listReq := connect.NewRequest(organizationv1.ListMembersRequest_builder{}.Build())
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(context.Background(), listReq)
	require.NoError(t, err)

	var callerMemberID, targetMemberID string
	for _, m := range listRes.Msg.GetMembers() {
		switch m.GetUserId() {
		case callerUserID.String():
			callerMemberID = m.GetId()
		case targetUserID.String():
			targetMemberID = m.GetId()
		}
	}
	require.NotEmpty(t, callerMemberID, "caller member not found in ListMembers response")
	require.NotEmpty(t, targetMemberID, "target member not found in ListMembers response")

	tests := map[string]struct {
		id         string
		permission string
		wantCode   connect.Code
		wantErr    bool
	}{
		"not_found": {
			id:         uuid.New().String(),
			permission: "viewer",
			wantCode:   connect.CodeNotFound,
			wantErr:    true,
		},
		"self_update_blocked": {
			id:         callerMemberID,
			permission: "viewer",
			wantCode:   connect.CodeFailedPrecondition,
			wantErr:    true,
		},
		"happy_flow": {
			id:         targetMemberID,
			permission: "viewer",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := connect.NewRequest(organizationv1.UpdateMemberPermissionRequest_builder{
				Id:         tc.id,
				Permission: tc.permission,
			}.Build())
			req.Header().Set("Authorization", "Bearer "+token)
			req.Header().Set("Fun-Organization", orgID.String())

			_, err := client.UpdateMemberPermission(context.Background(), req)

			if tc.wantErr {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.wantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)

			getReq := connect.NewRequest(organizationv1.GetMemberRequest_builder{
				Id: proto.String(tc.id),
			}.Build())
			getReq.Header().Set("Authorization", "Bearer "+token)
			getReq.Header().Set("Fun-Organization", orgID.String())

			getRes, err := client.GetMember(context.Background(), getReq)
			require.NoError(t, err)
			assert.Equal(t, tc.permission, getRes.Msg.GetMember().GetPermission())
		})
	}
}
