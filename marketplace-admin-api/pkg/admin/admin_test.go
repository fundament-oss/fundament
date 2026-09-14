package admin_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

// A published definition must carry a digest-pinned image or ParseDefinition
// rejects it; permissions are declared so GetPluginVersion has capabilities to
// derive.
const testManifest = `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: postgres-operator
  version: 1.2.3
spec:
  image: ghcr.io/example/postgres-operator@sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
  permissions:
    capabilities:
      - internet_access
    rbac:
      - apiGroups: [""]
        resources: ["secrets"]
        verbs: ["get", "list"]
      - apiGroups: ["apps"]
        resources: ["deployments"]
        verbs: ["create", "update"]
`

func seedOrganization(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	orgID := uuid.New()
	_, err := env.superPool.Exec(context.Background(),
		`INSERT INTO tenant.organizations (id, name, alias) VALUES ($1, $2, $2)`, orgID, name)
	require.NoError(t, err)

	return orgID
}

func seedUser(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	userID := uuid.New()
	_, err := env.superPool.Exec(context.Background(),
		`INSERT INTO tenant.users (id, name) VALUES ($1, $2)`, userID, name)
	require.NoError(t, err)

	return userID
}

func seedPlugin(t *testing.T, env *testEnv, organizationID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	pluginID := uuid.New()
	_, err := env.superPool.Exec(context.Background(), `
		INSERT INTO appstore.plugins (id, organization_id, name, display_name, description, visibility)
		VALUES ($1, $2, $3, $3, '', 'public')`, pluginID, organizationID, name)
	require.NoError(t, err)

	return pluginID
}

func seedVersion(t *testing.T, env *testEnv, pluginID uuid.UUID, version, status string) uuid.UUID {
	t.Helper()

	versionID := uuid.New()
	_, err := env.superPool.Exec(context.Background(), `
		INSERT INTO appstore.plugin_definitions (id, plugin_id, plugin_version, manifest, hash, image, status)
		VALUES ($1, $2, $3, $4, 'sha256:test', 'ghcr.io/example/img@sha256:abc', $5)`,
		versionID, pluginID, version, []byte(testManifest), status)
	require.NoError(t, err)

	return versionID
}

func seedSubmission(t *testing.T, env *testEnv, versionID, submitterID uuid.UUID) uuid.UUID {
	t.Helper()

	submissionID := uuid.New()
	_, err := env.superPool.Exec(context.Background(), `
		INSERT INTO appstore.submissions (id, plugin_definition_id, submitter_user_id)
		VALUES ($1, $2, $3)`, submissionID, versionID, submitterID)
	require.NoError(t, err)

	return submissionID
}

// seedPendingSubmission is the whole fixture chain: an organization, a
// submitter, a listing, a PENDING version and its open round.
type pendingSubmission struct {
	organizationID uuid.UUID
	submitterID    uuid.UUID
	pluginID       uuid.UUID
	versionID      uuid.UUID
	submissionID   uuid.UUID
}

func seedPendingSubmission(t *testing.T, env *testEnv, pluginName string) pendingSubmission {
	t.Helper()

	orgID := seedOrganization(t, env, "org-"+uuid.NewString()[:8])
	submitterID := seedUser(t, env, "dev-"+uuid.NewString()[:8])
	pluginID := seedPlugin(t, env, orgID, pluginName)
	versionID := seedVersion(t, env, pluginID, "1.2.3", "pending")
	submissionID := seedSubmission(t, env, versionID, submitterID)

	return pendingSubmission{
		organizationID: orgID,
		submitterID:    submitterID,
		pluginID:       pluginID,
		versionID:      versionID,
		submissionID:   submissionID,
	}
}

func newReviewer(t *testing.T, env *testEnv) reviewClient {
	t.Helper()
	return newClient(t, env, authFor(t, uuid.New()))
}

func TestRPCsRequireAuthentication(t *testing.T) {
	env := newTestEnv(t)
	client := newClient(t, env) // no token

	_, err := client.ListSubmissions(context.Background(),
		adminv1.ListSubmissionsRequest_builder{}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// The reviewer credential is the DCIM (staff) token; a customer's console
// token must be refused even though both are signed with the shared secret.
func TestConsoleTokenIsRefused(t *testing.T) {
	env := newTestEnv(t)
	client := newClient(t, env, authWithConsoleToken(t, uuid.New()))

	_, err := client.ListSubmissions(context.Background(),
		adminv1.ListSubmissionsRequest_builder{}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// The reviewer is cross-tenant by design: submissions from two different
// organizations land in one queue.
func TestListSubmissionsSpansOrganizations(t *testing.T) {
	env := newTestEnv(t)
	first := seedPendingSubmission(t, env, "cert-manager")
	second := seedPendingSubmission(t, env, "postgres-operator")

	client := newReviewer(t, env)
	resp, err := client.ListSubmissions(context.Background(),
		adminv1.ListSubmissionsRequest_builder{}.Build())
	require.NoError(t, err)

	ids := make([]string, 0, len(resp.GetSubmissions()))
	for _, submission := range resp.GetSubmissions() {
		ids = append(ids, submission.GetId())
	}
	assert.Contains(t, ids, first.submissionID.String())
	assert.Contains(t, ids, second.submissionID.String())
}

func TestListSubmissionsFiltersByStatus(t *testing.T) {
	env := newTestEnv(t)
	pending := seedPendingSubmission(t, env, "cert-manager")

	// A rejected version with its closed round.
	rejected := seedPendingSubmission(t, env, "postgres-operator")
	_, err := env.superPool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET status = 'rejected' WHERE id = $1`, rejected.versionID)
	require.NoError(t, err)
	_, err = env.superPool.Exec(context.Background(), `
		UPDATE appstore.submissions SET reviewer_user_id = $1, reviewed = now(), closed = now()
		WHERE id = $2`, uuid.New(), rejected.submissionID)
	require.NoError(t, err)

	client := newReviewer(t, env)
	resp, err := client.ListSubmissions(context.Background(),
		adminv1.ListSubmissionsRequest_builder{
			Status: marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING,
		}.Build())
	require.NoError(t, err)

	require.Len(t, resp.GetSubmissions(), 1)
	assert.Equal(t, pending.submissionID.String(), resp.GetSubmissions()[0].GetId())
}

func TestGetSubmissionResolvesIdentifiers(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	resp, err := client.GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
		}.Build())
	require.NoError(t, err)

	submission := resp.GetSubmission()
	assert.Equal(t, seeded.pluginID.String(), submission.GetPluginId())
	assert.Equal(t, seeded.versionID.String(), submission.GetPluginVersionId())
	assert.Equal(t, seeded.organizationID.String(), submission.GetOrganizationId())
	assert.Equal(t, seeded.submitterID.String(), submission.GetSubmitterUserId())
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING, submission.GetStatus())
	assert.NotNil(t, submission.GetSubmitted())
	assert.Nil(t, submission.GetReviewed())
	assert.Empty(t, submission.GetReviewerUserId())
}

func TestGetSubmissionUnknownIsNotFound(t *testing.T) {
	env := newTestEnv(t)

	client := newReviewer(t, env)
	_, err := client.GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{
			SubmissionId: uuid.NewString(),
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestApproveSubmissionPublishes(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	reviewerID := uuid.New()

	client := newClient(t, env, authFor(t, reviewerID))
	resp, err := client.ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
			Feedback:     "Looks good.",
		}.Build())
	require.NoError(t, err)

	submission := resp.GetSubmission()
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_APPROVED, submission.GetStatus())
	assert.Equal(t, reviewerID.String(), submission.GetReviewerUserId())
	assert.NotNil(t, submission.GetReviewed())
	assert.Equal(t, "Looks good.", submission.GetFeedback())

	// Approval is publication: the version now carries the published timestamp
	// that makes the listing catalog-visible.
	var status string
	var published *string
	err = env.superPool.QueryRow(context.Background(),
		`SELECT status, published::text FROM appstore.plugin_definitions WHERE id = $1`,
		seeded.versionID).Scan(&status, &published)
	require.NoError(t, err)
	assert.Equal(t, "approved", status)
	assert.NotNil(t, published)
}

func TestRequestChangesRecordsFeedback(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	resp, err := client.RequestChanges(context.Background(),
		adminv1.RequestChangesRequest_builder{
			SubmissionId: seeded.submissionID.String(),
			Feedback:     "Please scope down the RBAC rules.",
		}.Build())
	require.NoError(t, err)

	submission := resp.GetSubmission()
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_CHANGES_REQUESTED, submission.GetStatus())
	assert.Equal(t, "Please scope down the RBAC rules.", submission.GetFeedback())
	assert.Equal(t, adminv1.RejectionReason_REJECTION_REASON_UNSPECIFIED, submission.GetRejectionReason())

	// No published stamp: only approval publishes.
	var published *string
	err = env.superPool.QueryRow(context.Background(),
		`SELECT published::text FROM appstore.plugin_definitions WHERE id = $1`,
		seeded.versionID).Scan(&published)
	require.NoError(t, err)
	assert.Nil(t, published)
}

func TestRequestChangesRequiresFeedback(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	_, err := client.RequestChanges(context.Background(),
		adminv1.RequestChangesRequest_builder{
			SubmissionId: seeded.submissionID.String(),
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRejectSubmissionRecordsReason(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	resp, err := client.RejectSubmission(context.Background(),
		adminv1.RejectSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
			Reason:       adminv1.RejectionReason_REJECTION_REASON_SECURITY_CONCERNS,
			Feedback:     "The operator requests cluster-admin.",
		}.Build())
	require.NoError(t, err)

	submission := resp.GetSubmission()
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_REJECTED, submission.GetStatus())
	assert.Equal(t, adminv1.RejectionReason_REJECTION_REASON_SECURITY_CONCERNS, submission.GetRejectionReason())
}

func TestRejectSubmissionRequiresReason(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	_, err := client.RejectSubmission(context.Background(),
		adminv1.RejectSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// The resubmit flow: request changes, developer resubmits, a fresh round
// opens. The old round must keep showing the decision that closed it — as its
// own status, in the queue filter, and after the new round is approved. A
// version's history must never inherit its current state.
func TestResubmittedVersionKeepsRoundHistoryDistinct(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)

	// Round 1: request changes.
	_, err := client.RequestChanges(context.Background(),
		adminv1.RequestChangesRequest_builder{
			SubmissionId: seeded.submissionID.String(),
			Feedback:     "Scope down the RBAC rules.",
		}.Build())
	require.NoError(t, err)

	// Developer resubmits: version back to PENDING, a second round opens.
	_, err = env.superPool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET status = 'pending' WHERE id = $1`, seeded.versionID)
	require.NoError(t, err)
	secondRoundID := seedSubmission(t, env, seeded.versionID, seeded.submitterID)

	// The queue: one round pending, the old one still CHANGES_REQUESTED.
	resp, err := client.ListSubmissions(context.Background(),
		adminv1.ListSubmissionsRequest_builder{
			Status: marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING,
		}.Build())
	require.NoError(t, err)
	require.Len(t, resp.GetSubmissions(), 1)
	assert.Equal(t, secondRoundID.String(), resp.GetSubmissions()[0].GetId())

	oldRound, err := client.GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{SubmissionId: seeded.submissionID.String()}.Build())
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_CHANGES_REQUESTED,
		oldRound.GetSubmission().GetStatus())

	// The closed round cannot take the decision.
	_, err = client.ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{SubmissionId: seeded.submissionID.String()}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	// Approving the open round approves that round alone.
	approved, err := client.ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{SubmissionId: secondRoundID.String()}.Build())
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_APPROVED,
		approved.GetSubmission().GetStatus())

	oldRound, err = client.GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{SubmissionId: seeded.submissionID.String()}.Build())
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_CHANGES_REQUESTED,
		oldRound.GetSubmission().GetStatus())
}

// A withdrawn round reads as WITHDRAWN, not as whatever the version does next.
func TestWithdrawnRoundKeepsItsStatus(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	// Withdraw: round closes with no reviewer, version leaves PENDING.
	_, err := env.superPool.Exec(context.Background(),
		`UPDATE appstore.submissions SET closed = now() WHERE id = $1`, seeded.submissionID)
	require.NoError(t, err)
	_, err = env.superPool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET status = 'withdrawn' WHERE id = $1`, seeded.versionID)
	require.NoError(t, err)

	client := newReviewer(t, env)
	resp, err := client.GetSubmission(context.Background(),
		adminv1.GetSubmissionRequest_builder{SubmissionId: seeded.submissionID.String()}.Build())
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_WITHDRAWN,
		resp.GetSubmission().GetStatus())
}

// A decided submission cannot be decided again: the version has left PENDING.
func TestDecidingTwiceIsRefused(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	_, err := client.ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
		}.Build())
	require.NoError(t, err)

	_, err = client.RejectSubmission(context.Background(),
		adminv1.RejectSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
			Reason:       adminv1.RejectionReason_REJECTION_REASON_OTHER,
			Feedback:     "changed my mind",
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// A closed round of a version that is PENDING again (the developer resubmitted)
// must not take a decision: the verdict belongs to the open round.
func TestDecidingAClosedRoundIsRefused(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	// Close the first round as withdrawn, then open a second one.
	_, err := env.superPool.Exec(context.Background(),
		`UPDATE appstore.submissions SET closed = now() WHERE id = $1`, seeded.submissionID)
	require.NoError(t, err)
	seedSubmission(t, env, seeded.versionID, seeded.submitterID)

	client := newReviewer(t, env)
	_, err = client.ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{
			SubmissionId: seeded.submissionID.String(),
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestListPluginsSpansOrganizations(t *testing.T) {
	env := newTestEnv(t)
	first := seedPendingSubmission(t, env, "cert-manager")
	second := seedPendingSubmission(t, env, "postgres-operator")

	client := newReviewer(t, env)
	resp, err := client.ListPlugins(context.Background(),
		adminv1.ListPluginsRequest_builder{}.Build())
	require.NoError(t, err)

	ids := make([]string, 0, len(resp.GetPlugins()))
	for _, plugin := range resp.GetPlugins() {
		ids = append(ids, plugin.GetId())
	}
	assert.Contains(t, ids, first.pluginID.String())
	assert.Contains(t, ids, second.pluginID.String())
}

func TestGetPluginVersionDerivesReviewPayload(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	resp, err := client.GetPluginVersion(context.Background(),
		adminv1.GetPluginVersionRequest_builder{
			PluginVersionId: seeded.versionID.String(),
		}.Build())
	require.NoError(t, err)

	version := resp.GetVersion()
	assert.Equal(t, seeded.pluginID.String(), version.GetPluginId())
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING, version.GetStatus())
	assert.NotNil(t, version.GetSubmitted())

	// The review payload, derived from the pinned manifest.
	assert.Equal(t, []string{"internet_access"}, version.GetCapabilities())
	require.Len(t, version.GetPermissions(), 2)
	assert.Equal(t, "secrets", version.GetPermissions()[0].GetResource())
	assert.Equal(t, "Read", version.GetPermissions()[0].GetAccess())
	assert.Equal(t, "deployments", version.GetPermissions()[1].GetResource())
	assert.Equal(t, "Read and write", version.GetPermissions()[1].GetAccess())
}

// A corrupt manifest must be distinguishable from one that declares nothing:
// empty capabilities with manifest_error set, never a silent empty payload.
func TestGetPluginVersionSurfacesManifestError(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	_, err := env.superPool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET manifest = '\x00ff' WHERE id = $1`, seeded.versionID)
	require.NoError(t, err)

	client := newReviewer(t, env)
	resp, err := client.GetPluginVersion(context.Background(),
		adminv1.GetPluginVersionRequest_builder{
			PluginVersionId: seeded.versionID.String(),
		}.Build())
	require.NoError(t, err)

	version := resp.GetVersion()
	assert.Empty(t, version.GetCapabilities())
	assert.Empty(t, version.GetPermissions())
	assert.NotEmpty(t, version.GetManifestError())
}

func TestListPublishersIncludesUnpublishedPublishers(t *testing.T) {
	env := newTestEnv(t)
	// A publisher whose only version is still under review: the catalog hides
	// it, the backoffice must not.
	seeded := seedPendingSubmission(t, env, "cert-manager")

	client := newReviewer(t, env)
	resp, err := client.ListPublishers(context.Background(),
		adminv1.ListPublishersRequest_builder{}.Build())
	require.NoError(t, err)

	ids := make([]string, 0, len(resp.GetPublishers()))
	for _, publisher := range resp.GetPublishers() {
		ids = append(ids, publisher.GetId())
	}
	assert.Contains(t, ids, seeded.organizationID.String())
}

func TestListCategoriesServesVocabulary(t *testing.T) {
	env := newTestEnv(t)

	client := newReviewer(t, env)
	resp, err := client.ListCategories(context.Background(),
		adminv1.ListCategoriesRequest_builder{}.Build())
	require.NoError(t, err)

	// db/seed/0101-appstore-catalog.sql ships the vocabulary.
	assert.NotEmpty(t, resp.GetCategories())
}
