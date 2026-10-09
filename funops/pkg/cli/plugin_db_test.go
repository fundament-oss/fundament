package cli

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// Plugin review decisions mirror marketplace-admin-api's ReviewService: only a
// PENDING version with an open submission round can be decided, the decision
// closes the round, and approval is what stamps published.

const (
	testOrgAcmeID   = "019b4000-0000-7000-8000-000000000001"
	testOrgGlobexID = "019b4000-0000-7000-8000-000000000002"

	// A tenant user from the test data, recorded as the round's submitter.
	testSubmitterID = "019b4000-1000-7000-8000-000000000004"

	// DCIM staff from the test data: reviewers live in dcim.users.
	testReviewerAliceID    = "019dce30-0000-7000-8000-000000000001"
	testReviewerAliceEmail = "alice@acme-corp.com"
)

// seedPluginVersion inserts a plugin with one version in the given status.
func seedPluginVersion(t *testing.T, ctx *Context, orgID, name, version string, status dbconst.PluginDefinitionStatus) (pluginID, definitionID uuid.UUID) {
	t.Helper()

	err := ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO appstore.plugins (organization_id, name, description)
		VALUES ($1, $2, '')
		RETURNING id`, orgID, name).Scan(&pluginID)
	require.NoError(t, err)

	err = ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, status)
		VALUES ($1, $2, '\x', 'testhash', $3)
		RETURNING id`, pluginID, version, string(status)).Scan(&definitionID)
	require.NoError(t, err)

	return pluginID, definitionID
}

// seedOpenRound opens a submission round for a version.
func seedOpenRound(t *testing.T, ctx *Context, definitionID uuid.UUID) {
	t.Helper()

	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO appstore.submissions (plugin_definition_id, submitter_user_id)
		VALUES ($1, $2)`, definitionID, testSubmitterID)
	require.NoError(t, err)
}

// versionState reads back what a decision is supposed to change on the version.
func versionState(t *testing.T, ctx *Context, definitionID uuid.UUID) (status string, published *time.Time) {
	t.Helper()

	err := ctx.DB.Pool.QueryRow(t.Context(), `
		SELECT status, published FROM appstore.plugin_definitions WHERE id = $1`,
		definitionID).Scan(&status, &published)
	require.NoError(t, err)
	return status, published
}

// roundState reads back what a decision records on the version's single round.
func roundState(t *testing.T, ctx *Context, definitionID uuid.UUID) (reviewerID, rejectionReason *string, reviewed, closed *time.Time, feedback string) {
	t.Helper()

	err := ctx.DB.Pool.QueryRow(t.Context(), `
		SELECT reviewer_user_id::text, rejection_reason, reviewed, closed, feedback
		FROM appstore.submissions WHERE plugin_definition_id = $1`,
		definitionID).Scan(&reviewerID, &rejectionReason, &reviewed, &closed, &feedback)
	require.NoError(t, err)
	return reviewerID, rejectionReason, reviewed, closed, feedback
}

func TestPluginApprove_PublishesPendingVersion(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceEmail, Feedback: "well packaged",
	}).Run(ctx))

	status, published := versionState(t, ctx, defID)
	assert.Equal(t, "approved", status)
	assert.NotNil(t, published, "approval stamps published")

	reviewerID, rejectionReason, reviewed, closed, feedback := roundState(t, ctx, defID)
	require.NotNil(t, reviewerID)
	assert.Equal(t, testReviewerAliceID, *reviewerID)
	assert.Nil(t, rejectionReason)
	assert.NotNil(t, reviewed)
	assert.NotNil(t, closed)
	assert.Equal(t, "well packaged", feedback)
}

func TestPluginApprove_ByPluginID(t *testing.T) {
	ctx := newTestContext(t)
	pluginID, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginApproveCmd{
		Plugin: pluginID.String(), Version: "1.0.0", Reviewer: testReviewerAliceID,
	}).Run(ctx))

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "approved", status)
}

func TestPluginRequestChanges_RecordsFeedback(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginRequestChangesCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceID, Feedback: "add release notes",
	}).Run(ctx))

	status, published := versionState(t, ctx, defID)
	assert.Equal(t, "changes_requested", status)
	assert.Nil(t, published, "only approval publishes")

	reviewerID, rejectionReason, _, closed, feedback := roundState(t, ctx, defID)
	require.NotNil(t, reviewerID)
	assert.Equal(t, testReviewerAliceID, *reviewerID)
	assert.Nil(t, rejectionReason)
	assert.NotNil(t, closed)
	assert.Equal(t, "add release notes", feedback)
}

func TestPluginReject_RecordsReason(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginRejectCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceEmail, Reason: "security_concerns",
	}).Run(ctx))

	status, published := versionState(t, ctx, defID)
	assert.Equal(t, "rejected", status)
	assert.Nil(t, published)

	_, rejectionReason, _, closed, _ := roundState(t, ctx, defID)
	require.NotNil(t, rejectionReason)
	assert.Equal(t, "security_concerns", *rejectionReason)
	assert.NotNil(t, closed)
}

func TestPluginDecide_RefusesNonPendingVersion(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Draft)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: testReviewerAliceEmail,
	}).Run(ctx)
	require.EqualError(t, err, `version "1.0.0" of plugin "logging-operator" is draft; only a pending version can be decided`)

	status, published := versionState(t, ctx, defID)
	assert.Equal(t, "draft", status)
	assert.Nil(t, published)
}

func TestPluginDecide_RefusesVersionWithoutOpenRound(t *testing.T) {
	ctx := newTestContext(t)
	seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: testReviewerAliceEmail,
	}).Run(ctx)
	require.EqualError(t, err, `version "1.0.0" of plugin "logging-operator" has no open submission round`)
}

func TestPluginDecide_RefusesUnknownReviewer(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: "nobody@example.com",
	}).Run(ctx)
	require.EqualError(t, err, `reviewer "nobody@example.com" not found`)

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "pending", status, "nothing lands when the reviewer is unknown")
}

func TestPluginDecide_RefusesUnknownPlugin(t *testing.T) {
	ctx := newTestContext(t)

	err := (&PluginApproveCmd{
		Plugin: "no-such-plugin", Version: "1.0.0", Reviewer: testReviewerAliceEmail,
	}).Run(ctx)
	require.EqualError(t, err, `plugin "no-such-plugin" not found`)
}

func TestPluginDecide_RefusesUnknownVersion(t *testing.T) {
	ctx := newTestContext(t)
	seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "9.9.9", Reviewer: testReviewerAliceEmail,
	}).Run(ctx)
	require.EqualError(t, err, `plugin "logging-operator" has no version "9.9.9"`)
}

func TestPluginDecide_AmbiguousNameNeedsOrganization(t *testing.T) {
	ctx := newTestContext(t)
	_, acmeDefID := seedPluginVersion(t, ctx, testOrgAcmeID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, acmeDefID)
	_, globexDefID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, globexDefID)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: testReviewerAliceEmail,
	}).Run(ctx)
	require.EqualError(t, err, `plugin "logging-operator" is published by 2 organizations (acme-corp, globex): pass --organization or the plugin ID`)

	require.NoError(t, (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceEmail, Organization: "globex",
	}).Run(ctx))

	status, _ := versionState(t, ctx, globexDefID)
	assert.Equal(t, "approved", status)
	status, _ = versionState(t, ctx, acmeDefID)
	assert.Equal(t, "pending", status, "the other organization's version is untouched")
}

func TestPluginDecide_RefusesIDWithMismatchedOrganization(t *testing.T) {
	ctx := newTestContext(t)
	pluginID, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	err := (&PluginApproveCmd{
		Plugin: pluginID.String(), Version: "1.0.0",
		Reviewer: testReviewerAliceEmail, Organization: "acme-corp",
	}).Run(ctx)
	require.EqualError(t, err, `plugin "`+pluginID.String()+`" is published by organization "globex", not "acme-corp"`)

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "pending", status)
}

func TestPluginDecide_UnknownOrganizationIsNotFound(t *testing.T) {
	ctx := newTestContext(t)
	seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceEmail, Organization: "initech",
	}).Run(ctx)
	require.EqualError(t, err, `plugin "logging-operator" not found in organization "initech"`)
}

func TestPluginList_FiltersByStatus(t *testing.T) {
	ctx := newTestContext(t)
	_, pendingDefID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, pendingDefID)
	seedPluginVersion(t, ctx, testOrgGlobexID, "metrics-operator", "2.0.0", dbconst.PluginDefinitionStatus_Draft)

	rows, err := ctx.Queries.PluginVersionList(t.Context(), db.PluginVersionListParams{Status: "pending"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "globex", rows[0].OrganizationName)
	assert.Equal(t, "logging-operator", rows[0].PluginName)
	assert.Equal(t, "1.0.0", rows[0].PluginVersion)
	assert.Equal(t, dbconst.PluginDefinitionStatus_Pending, rows[0].Status)
	assert.True(t, rows[0].Submitted.Valid, "the open round's submitted timestamp is listed")
	assert.False(t, rows[0].Published.Valid)

	rows, err = ctx.Queries.PluginVersionList(t.Context(), db.PluginVersionListParams{Status: ""})
	require.NoError(t, err)
	assert.Len(t, rows, 2, "an empty status lists every version")

	// The command accepts both a status and 'all'.
	require.NoError(t, (&PluginListCmd{Status: "pending"}).Run(ctx))
	require.NoError(t, (&PluginListCmd{Status: "all"}).Run(ctx))
}
