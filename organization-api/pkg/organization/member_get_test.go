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

func Test_Member_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetMember(context.Background(), organizationv1.GetMemberRequest_builder{
		Id: new(uuid.New().String()),
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Member_Get(t *testing.T) {
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
	listReq := organizationv1.ListMembersRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(listCtx, listReq)
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
		Request  *organizationv1.GetMemberRequest
		WantCode connect.Code
		WantErr  bool
	}{
		"by_id_not_found": {
			Request: organizationv1.GetMemberRequest_builder{
				Id: new(uuid.New().String()),
			}.Build(),
			WantCode: connect.CodeNotFound,
			WantErr:  true,
		},
		"by_user_id_not_found": {
			Request: organizationv1.GetMemberRequest_builder{
				UserId: new(uuid.New().String()),
			}.Build(),
			WantCode: connect.CodeNotFound,
			WantErr:  true,
		},
		"by_id_happy_flow": {
			Request: organizationv1.GetMemberRequest_builder{
				Id: new(targetMemberID),
			}.Build(),
		},
		"by_user_id_happy_flow": {
			Request: organizationv1.GetMemberRequest_builder{
				UserId: new(targetUserID.String()),
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

			res, err := client.GetMember(ctx, req)

			if tc.WantErr {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.WantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)

			member := res.GetMember()
			assert.Equal(t, targetMemberID, member.GetId())
			assert.Equal(t, targetUserID.String(), member.GetUserId())
			assert.Equal(t, "target-user", member.GetName())
			assert.Equal(t, "admin", member.GetPermission())
			assert.Equal(t, "accepted", member.GetStatus())
		})
	}
}

// GetMember must resolve within the current organization only. A caller who
// is a member of two organizations can select both of their own membership
// rows, so without an organization filter a lookup by user id could return the
// other organization's row, and a lookup by that row's id would succeed.
func Test_Member_Get_UserInTwoOrganizations(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	callerUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-a"),
		WithOrganization(orgBID, "org-b"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	callerMembershipID := func(t *testing.T, orgID uuid.UUID) string {
		t.Helper()

		ctx, callInfo := connect.NewClientContext(context.Background())
		callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

		res, err := client.ListMembers(ctx, &organizationv1.ListMembersRequest{})
		require.NoError(t, err)
		require.Len(t, res.GetMembers(), 1)

		return res.GetMembers()[0].GetId()
	}

	membershipA := callerMembershipID(t, orgAID)
	membershipB := callerMembershipID(t, orgBID)
	require.NotEqual(t, membershipA, membershipB)

	ctxA, callInfoA := connect.NewClientContext(context.Background())
	callInfoA.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfoA.RequestHeader().Set("Fun-Organization", orgAID.String())

	ctxB, callInfoB := connect.NewClientContext(context.Background())
	callInfoB.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfoB.RequestHeader().Set("Fun-Organization", orgBID.String())

	byUserID := organizationv1.GetMemberRequest_builder{
		UserId: new(callerUserID.String()),
	}.Build()

	resA, err := client.GetMember(ctxA, byUserID)
	require.NoError(t, err)
	assert.Equal(t, membershipA, resA.GetMember().GetId())

	resB, err := client.GetMember(ctxB, byUserID)
	require.NoError(t, err)
	assert.Equal(t, membershipB, resB.GetMember().GetId())

	_, err = client.GetMember(ctxA, organizationv1.GetMemberRequest_builder{
		Id: new(membershipB),
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
