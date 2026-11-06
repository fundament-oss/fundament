package admin_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

func seedIcons(t *testing.T, env *testEnv, versionID uuid.UUID, logo, logoDark, menuIcons string) {
	t.Helper()

	_, err := env.superPool.Exec(context.Background(), `
		UPDATE appstore.plugin_definitions
		SET logo = $2, logo_dark = $3, menu_icons = $4::jsonb
		WHERE id = $1`, versionID, logo, logoDark, menuIcons)
	require.NoError(t, err)
}

func TestGetPluginVersion_ReturnsIcons(t *testing.T) {
	env := newTestEnv(t)
	orgID := seedOrganization(t, env, "org-"+uuid.NewString()[:8])
	pluginID := seedPlugin(t, env, orgID, "cert-manager")
	versionID := seedVersion(t, env, pluginID, "1.0.0", "draft")
	seedIcons(t, env, versionID, "data:image/svg+xml;base64,TElHSFQ=", "data:image/svg+xml;base64,REFSSw==",
		`{"sha256:bbb":"data:image/svg+xml;base64,Qg==","sha256:aaa":"data:image/svg+xml;base64,QQ=="}`)

	client := newReviewer(t, env)
	resp, err := client.GetPluginVersion(context.Background(),
		adminv1.GetPluginVersionRequest_builder{PluginVersionId: versionID.String()}.Build())
	require.NoError(t, err)

	assert.Equal(t, []string{"data:image/svg+xml;base64,QQ==", "data:image/svg+xml;base64,Qg=="}, resp.GetVersion().GetMenuIcons())
}

func TestListSubmissions_ReturnsLogo(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	seedIcons(t, env, seeded.versionID, "data:image/svg+xml;base64,UEVORElORw==", "data:image/svg+xml;base64,UEVORElORy1EQVJL", `{}`)

	// A newer published version with its own logo must not leak into the round.
	published := seedVersion(t, env, seeded.pluginID, "2.0.0", "approved")
	seedIcons(t, env, published, "data:image/svg+xml;base64,UFVCTElTSEVE", "", `{}`)

	client := newReviewer(t, env)
	resp, err := client.ListSubmissions(context.Background(), adminv1.ListSubmissionsRequest_builder{}.Build())
	require.NoError(t, err)

	require.Len(t, resp.GetSubmissions(), 1)
	submission := resp.GetSubmissions()[0]
	assert.Equal(t, "data:image/svg+xml;base64,UEVORElORw==", submission.GetLogo())
	assert.Equal(t, "data:image/svg+xml;base64,UEVORElORy1EQVJL", submission.GetLogoDark())
}
