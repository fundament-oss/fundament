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

	_, err := client.DeleteMember(context.Background(), connect.NewRequest(&organizationv1.DeleteMemberRequest{
		Id: "arbitrary",
	}))

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
	theReq := connect.NewRequest(&organizationv1.ListMembersRequest{})
	theReq.Header().Set("Authorization", "Bearer "+token)
	theReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(context.Background(), theReq)
	require.NoError(t, err)

	var targetMemberID string
	for _, m := range listRes.Msg.Members {
		if m.UserId == targetUserID.String() {
			targetMemberID = m.Id
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
			Request: &organizationv1.DeleteMemberRequest{
				Id: uuid.New().String(),
			},
			WantCode: connect.CodeNotFound,
			WantErr:  true,
		},
		"happy_flow": {
			Request: &organizationv1.DeleteMemberRequest{
				Id: targetMemberID,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := connect.NewRequest(tc.Request)
			req.Header().Set("Authorization", "Bearer "+token)
			req.Header().Set("Fun-Organization", orgID.String())

			res, err := client.DeleteMember(context.Background(), req)

			if tc.WantErr {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.WantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "", res.Msg.String())

			getReq := connect.NewRequest(&organizationv1.GetMemberRequest{
				Lookup: &organizationv1.GetMemberRequest_UserId{UserId: targetUserID.String()},
			})
			getReq.Header().Set("Authorization", "Bearer "+token)
			getReq.Header().Set("Fun-Organization", orgID.String())

			_, err = client.GetMember(context.Background(), getReq)

			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeNotFound, connectErr.Code())
		})
	}
}
