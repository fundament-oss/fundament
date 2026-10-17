package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
func seedOpenRound(t *testing.T, ctx *Context, definitionID uuid.UUID) (roundID uuid.UUID) {
	t.Helper()

	err := ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO appstore.submissions (plugin_definition_id, submitter_user_id)
		VALUES ($1, $2)
		RETURNING id`, definitionID, testSubmitterID).Scan(&roundID)
	require.NoError(t, err)
	return roundID
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

func TestSubmissionDecide_IsPinnedToTheRoundItLoaded(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	oldRound := seedOpenRound(t, ctx, defID)

	// A withdraw closes the round and a resubmit opens a fresh one; a decision
	// pinned to the old round must land on nothing.
	_, err := ctx.DB.Pool.Exec(t.Context(),
		`UPDATE appstore.submissions SET closed = now() WHERE id = $1`, oldRound)
	require.NoError(t, err)
	newRound := seedOpenRound(t, ctx, defID)

	closed, err := ctx.Queries.SubmissionDecide(t.Context(), db.SubmissionDecideParams{
		ID:             oldRound,
		ReviewerUserID: pgtype.UUID{Bytes: uuid.MustParse(testReviewerAliceID), Valid: true},
		Feedback:       "late verdict",
	})
	require.NoError(t, err)
	assert.Zero(t, closed, "a verdict must not land on a round the operator did not see")

	var newRoundClosed *time.Time
	err = ctx.DB.Pool.QueryRow(t.Context(),
		`SELECT closed FROM appstore.submissions WHERE id = $1`, newRound).Scan(&newRoundClosed)
	require.NoError(t, err)
	assert.Nil(t, newRoundClosed, "the fresh round is untouched")
}

func TestPluginRequestChanges_RefusesEmptyFeedback(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	err := (&PluginRequestChangesCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceID, Feedback: "   ",
	}).Run(ctx)
	require.EqualError(t, err, "feedback is required: say what should change")

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "pending", status)
}

func TestPluginDecide_RefusesOverlongFeedback(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0",
		Reviewer: testReviewerAliceID, Feedback: strings.Repeat("x", 4001),
	}).Run(ctx)
	require.EqualError(t, err, "feedback must be at most 4000 characters")

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "pending", status)
}

func TestPluginDecide_UUIDShapedNameStillResolves(t *testing.T) {
	ctx := newTestContext(t)
	// plugins_ck_name allows an all-hex name that uuid.Parse also accepts.
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "deadbeefdeadbeefdeadbeefdeadbeef", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginApproveCmd{
		Plugin: "deadbeefdeadbeefdeadbeefdeadbeef", Version: "1.0.0",
		Reviewer: testReviewerAliceID,
	}).Run(ctx))

	status, _ := versionState(t, ctx, defID)
	assert.Equal(t, "approved", status)
}

func TestPluginDecide_InvalidReviewerSaysReviewer(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	err := (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: "not an email",
	}).Run(ctx)
	require.EqualError(t, err, `invalid reviewer "not an email": expected a user ID or an email address`)
}

func TestPluginDecide_AmbiguousReviewerListsNames(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	var revOne, revTwo string
	err := ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO dcim.users (name, email) VALUES ('Rev One', 'dup@example.com')
		RETURNING id::text`).Scan(&revOne)
	require.NoError(t, err)
	err = ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO dcim.users (name, email) VALUES ('Rev Two', 'dup@example.com')
		RETURNING id::text`).Scan(&revTwo)
	require.NoError(t, err)

	err = (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: "dup@example.com",
	}).Run(ctx)
	require.EqualError(t, err,
		`2 reviewers match email "dup@example.com" (Rev One `+revOne+`, Rev Two `+revTwo+`): pass the user ID instead`)
}

func TestPluginList_ShowsSubmittedForDecidedVersions(t *testing.T) {
	ctx := newTestContext(t)
	_, defID := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.0.0", dbconst.PluginDefinitionStatus_Pending)
	seedOpenRound(t, ctx, defID)

	require.NoError(t, (&PluginApproveCmd{
		Plugin: "logging-operator", Version: "1.0.0", Reviewer: testReviewerAliceID,
	}).Run(ctx))

	rows, err := ctx.Queries.PluginVersionList(t.Context(), db.PluginVersionListParams{Status: "approved"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Submitted.Valid, "a decided version still shows when it was submitted")
	assert.True(t, rows[0].Published.Valid)
}

func TestPluginList_OrdersVersionsByCreation(t *testing.T) {
	ctx := newTestContext(t)
	pluginID, _ := seedPluginVersion(t, ctx, testOrgGlobexID, "logging-operator", "1.2.0", dbconst.PluginDefinitionStatus_Draft)

	// A lexical sort would put 1.10.0 before 1.2.0; creation order must win.
	_, err := ctx.DB.Pool.Exec(t.Context(), `
		UPDATE appstore.plugin_definitions SET created = now() - interval '1 hour' WHERE plugin_id = $1`, pluginID)
	require.NoError(t, err)
	_, err = ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, status)
		VALUES ($1, '1.10.0', '\x', 'testhash', 'draft')`, pluginID)
	require.NoError(t, err)

	rows, err := ctx.Queries.PluginVersionList(t.Context(), db.PluginVersionListParams{Status: "draft"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "1.2.0", rows[0].PluginVersion)
	assert.Equal(t, "1.10.0", rows[1].PluginVersion)
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
