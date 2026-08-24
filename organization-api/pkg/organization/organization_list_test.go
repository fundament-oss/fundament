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

func Test_Organization_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListOrganizations(context.Background(), &organizationv1.ListOrganizationsRequest{})

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Organization_List(t *testing.T) {
	t.Parallel()

	org1ID := uuid.New()
	org2ID := uuid.New()
	org3ID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(org1ID, "org-one"),
		WithOrganization(org2ID, "org-two"),
		WithOrganization(org3ID, "org-three"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{org1ID, org2ID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	listReq := &organizationv1.ListOrganizationsRequest{}
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)

	listRes, err := client.ListOrganizations(listCtx, listReq)
	require.NoError(t, err)

	require.Len(t, listRes.GetOrganizations(), 2)
}
