package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AcceptInvitation_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.AcceptInvitation(context.Background(), connect.NewRequest(&organizationv1.AcceptInvitationRequest{
		Id: "arbitrary",
	}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}
