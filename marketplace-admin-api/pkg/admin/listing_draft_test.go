package admin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/pluginlisting"
	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

// seedDraft stores draft as the listing's pending edits and, when submitted,
// on the version's open round too.
func seedDraft(t *testing.T, env *testEnv, pluginID uuid.UUID, draft *pluginlisting.Draft) []byte {
	t.Helper()

	raw, err := json.Marshal(draft)
	require.NoError(t, err)
	_, err = env.superPool.Exec(context.Background(),
		`UPDATE appstore.plugins SET listing_draft = $2 WHERE id = $1`, pluginID, raw)
	require.NoError(t, err)
	return raw
}

func freezeDraft(t *testing.T, env *testEnv, submissionID uuid.UUID, raw []byte) {
	t.Helper()

	_, err := env.superPool.Exec(context.Background(),
		`UPDATE appstore.submissions SET listing_draft = $2 WHERE id = $1`, submissionID, raw)
	require.NoError(t, err)
}

func seedFeature(t *testing.T, env *testEnv, pluginID uuid.UUID, title string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := env.superPool.QueryRow(context.Background(),
		`INSERT INTO appstore.plugin_features (plugin_id, title, body, position) VALUES ($1, $2, '', 0) RETURNING id`,
		pluginID, title).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedCategory(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := env.superPool.QueryRow(context.Background(),
		`INSERT INTO appstore.categories (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	require.NoError(t, err)
	return id
}

type liveFeature struct {
	id    uuid.UUID
	title string
}

func liveFeatures(t *testing.T, env *testEnv, pluginID uuid.UUID) []liveFeature {
	t.Helper()

	rows, err := env.superPool.Query(context.Background(),
		`SELECT id, title FROM appstore.plugin_features WHERE plugin_id = $1 AND deleted IS NULL ORDER BY position`, pluginID)
	require.NoError(t, err)
	defer rows.Close()

	var features []liveFeature
	for rows.Next() {
		var feature liveFeature
		require.NoError(t, rows.Scan(&feature.id, &feature.title))
		features = append(features, feature)
	}
	require.NoError(t, rows.Err())
	return features
}

func pendingDraft(t *testing.T, env *testEnv, pluginID uuid.UUID) []byte {
	t.Helper()

	var raw []byte
	err := env.superPool.QueryRow(context.Background(),
		`SELECT listing_draft FROM appstore.plugins WHERE id = $1`, pluginID).Scan(&raw)
	require.NoError(t, err)
	return raw
}

// Approval applies the draft frozen on the round: rows the draft keeps are
// edited in place, the rest are soft-deleted, and new ones are added.
func TestApproveAppliesTheSubmittedListingDraft(t *testing.T) {
	env := newTestEnv(t)
	orgID := seedOrganization(t, env, "acme")
	submitterID := seedUser(t, env, "dev")
	pluginID := seedPlugin(t, env, orgID, "cert-manager")
	kept := seedFeature(t, env, pluginID, "Renewal")
	seedFeature(t, env, pluginID, "Dropped")
	categoryID := seedCategory(t, env, "Certificates "+uuid.NewString()[:8])

	raw := seedDraft(t, env, pluginID, &pluginlisting.Draft{
		Description: "Reviewed write-up.",
		CategoryIDs: []uuid.UUID{categoryID},
		AuthorName:  "Jetstack",
		Logo:        "data:image/svg+xml;base64,PHN2Zy8+",
		Features: []pluginlisting.Feature{
			{Title: "Issuers"},
			{ID: kept, Title: "Automatic renewal"},
		},
	})
	versionID := seedVersionWithManifest(t, env, pluginID, "1.0.0", listingManifest("1.0.0", "Cert Manager"))
	submissionID := seedSubmission(t, env, versionID, submitterID)
	freezeDraft(t, env, submissionID, raw)

	got, err := newReviewer(t, env).GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{SubmissionId: submissionID.String()}.Build())
	require.NoError(t, err)
	changes := got.GetSubmission().GetListingChanges()
	assert.Equal(t, "Reviewed write-up.", changes.GetDescription(), "the reviewer sees what approval applies")
	assert.Equal(t, []string{categoryID.String()}, changes.GetCategoryIds())

	approve(t, env, submissionID)

	assert.Equal(t, "Reviewed write-up.", readListing(t, env, pluginID).description)
	features := liveFeatures(t, env, pluginID)
	require.Len(t, features, 2)
	assert.Equal(t, "Issuers", features[0].title)
	assert.Equal(t, liveFeature{id: kept, title: "Automatic renewal"}, features[1])

	var authorName, logo string
	var categories int
	err = env.superPool.QueryRow(context.Background(), `
		SELECT author_name, logo,
		       (SELECT count(*) FROM appstore.categories_plugins WHERE plugin_id = $1 AND category_id = $2)
		FROM appstore.plugins WHERE id = $1`, pluginID, categoryID).Scan(&authorName, &logo, &categories)
	require.NoError(t, err)
	assert.Equal(t, "Jetstack", authorName)
	assert.Equal(t, "data:image/svg+xml;base64,PHN2Zy8+", logo)
	assert.Equal(t, 1, categories)

	assert.Nil(t, pendingDraft(t, env, pluginID), "the applied draft is cleared")
}

// Edits made after the submission are not what was reviewed: they stay
// pending for the next round.
func TestApproveKeepsEditsMadeDuringReview(t *testing.T) {
	env := newTestEnv(t)
	orgID := seedOrganization(t, env, "acme")
	submitterID := seedUser(t, env, "dev")
	pluginID := seedPlugin(t, env, orgID, "cert-manager")

	submitted := seedDraft(t, env, pluginID, &pluginlisting.Draft{Description: "Submitted."})
	versionID := seedVersionWithManifest(t, env, pluginID, "1.0.0", listingManifest("1.0.0", "Cert Manager"))
	submissionID := seedSubmission(t, env, versionID, submitterID)
	freezeDraft(t, env, submissionID, submitted)
	later := seedDraft(t, env, pluginID, &pluginlisting.Draft{Description: "Edited during review."})

	approve(t, env, submissionID)

	assert.Equal(t, "Submitted.", readListing(t, env, pluginID).description)
	assert.JSONEq(t, string(later), string(pendingDraft(t, env, pluginID)))
}

// Approving an older version line leaves the listing alone, draft included.
func TestApprovingAnOlderVersionLeavesTheDraft(t *testing.T) {
	env := newTestEnv(t)
	orgID := seedOrganization(t, env, "acme")
	submitterID := seedUser(t, env, "dev")
	pluginID := seedPlugin(t, env, orgID, "cert-manager")

	newer := seedVersionWithManifest(t, env, pluginID, "2.0.0", listingManifest("2.0.0", "Cert Manager"))
	approve(t, env, seedSubmission(t, env, newer, submitterID))

	raw := seedDraft(t, env, pluginID, &pluginlisting.Draft{Description: "Reviewed with 1.0.1."})
	older := seedVersionWithManifest(t, env, pluginID, "1.0.1", listingManifest("1.0.1", "Cert Manager"))
	submissionID := seedSubmission(t, env, older, submitterID)
	freezeDraft(t, env, submissionID, raw)
	approve(t, env, submissionID)

	assert.NotEqual(t, "Reviewed with 1.0.1.", readListing(t, env, pluginID).description)
	assert.NotNil(t, pendingDraft(t, env, pluginID))
}
