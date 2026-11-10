package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/notify"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/registry"
)

type recordingNotifier chan notify.ReviewRequest

func (n recordingNotifier) ReviewRequested(_ context.Context, req notify.ReviewRequest) { n <- req }

func (n recordingNotifier) next(t *testing.T) notify.ReviewRequest {
	t.Helper()
	select {
	case req := <-n:
		return req
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no review notification")
		return notify.ReviewRequest{}
	}
}

// Every opened round notifies, resubmissions included; an idempotent submit of
// an already pending version opens nothing and so notifies nothing.
func TestSubmitPluginVersionNotifiesReviewers(t *testing.T) {
	env := newTestEnv(t)
	notifier := make(recordingNotifier, 4)

	orgID := seedOrganization(t, env, "org-"+uuid.NewString()[:8])
	userID := seedUser(t, env, "user-"+uuid.NewString()[:8])
	client := newClientWithConfig(t, env, registry.Config{
		JWTSecret:      testJWTSecret,
		ReviewNotifier: notifier,
	}, authFor(t, userID, orgID))

	plugin, version := seedDraftVersion(t, client)
	submit := registryv1.SubmitPluginVersionRequest_builder{PluginVersionId: version.GetId()}.Build()

	_, err := client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)

	first := notifier.next(t)
	assert.Equal(t, plugin.GetName(), first.PluginName)
	assert.Equal(t, "1.2.3", first.Version)
	assert.NotEqual(t, uuid.Nil, first.SubmissionID)

	_, err = client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)

	_, err = client.WithdrawPluginVersion(context.Background(), registryv1.WithdrawPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.NoError(t, err)

	_, err = client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)

	second := notifier.next(t)
	assert.NotEqual(t, first.SubmissionID, second.SubmissionID, "a resubmission opens a new round")
	assert.Never(t, func() bool { return len(notifier) > 0 }, 200*time.Millisecond, 10*time.Millisecond,
		"the idempotent submit must not notify")
}
