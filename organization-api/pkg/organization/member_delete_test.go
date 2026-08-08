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
