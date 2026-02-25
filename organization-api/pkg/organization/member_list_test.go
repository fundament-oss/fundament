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

func Test_Member_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListMembers(context.Background(), connect.NewRequest(&organizationv1.ListMembersRequest{}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Member_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "my-organization"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user2",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user3",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user4",
			OrgIDs: []uuid.UUID{},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	listReq := connect.NewRequest(&organizationv1.ListMembersRequest{})
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(context.Background(), listReq)
	require.NoError(t, err)

	require.Len(t, listRes.Msg.Members, 3)
}
