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

// seedLabel writes appstore.plugin_labels directly: labels are
// Fundament-assigned trust badges, and no RPC creates one yet.
func seedLabel(t *testing.T, env *testEnv, pluginID uuid.UUID, name string, revoked bool) {
	t.Helper()

	_, err := env.superPool.Exec(context.Background(), `
		INSERT INTO appstore.plugin_labels (plugin_id, name, deleted)
		VALUES ($1, $2, CASE WHEN $3::bool THEN now() ELSE NULL END)`,
		pluginID, name, revoked)
	require.NoError(t, err)
}

// A revoked badge stays invisible, and a badge must never bleed onto another
// listing in the batch read.
func TestPluginsCarryAssignedLabels(t *testing.T) {
	env := newTestEnv(t)
	labeled := seedPendingSubmission(t, env, "cert-manager")
	plain := seedPendingSubmission(t, env, "postgres-operator")

	seedLabel(t, env, labeled.pluginID, "core", false)
	seedLabel(t, env, labeled.pluginID, "rijksoverheid", false)
	seedLabel(t, env, labeled.pluginID, "support_9_to_17", true)

	client := newReviewer(t, env)

	resp, err := client.GetPlugin(context.Background(),
		adminv1.GetPluginRequest_builder{PluginId: labeled.pluginID.String()}.Build())
	require.NoError(t, err)
	assert.Equal(t, []marketplacev1.PluginLabel{
		marketplacev1.PluginLabel_PLUGIN_LABEL_CORE,
		marketplacev1.PluginLabel_PLUGIN_LABEL_RIJKSOVERHEID,
	}, resp.GetPlugin().GetLabels())

	list, err := client.ListPlugins(context.Background(), adminv1.ListPluginsRequest_builder{}.Build())
	require.NoError(t, err)

	byID := map[string]*adminv1.Plugin{}
	for _, plugin := range list.GetPlugins() {
		byID[plugin.GetId()] = plugin
	}
	require.Contains(t, byID, labeled.pluginID.String())
	require.Contains(t, byID, plain.pluginID.String())
	assert.Equal(t, []marketplacev1.PluginLabel{
		marketplacev1.PluginLabel_PLUGIN_LABEL_CORE,
		marketplacev1.PluginLabel_PLUGIN_LABEL_RIJKSOVERHEID,
	}, byID[labeled.pluginID.String()].GetLabels())
	assert.Empty(t, byID[plain.pluginID.String()].GetLabels())
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

	// The manifest travels verbatim, so the reviewer reads what was hashed.
	assert.Equal(t, []byte(testManifest), version.GetManifest())
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

// seedReviewer creates a DCIM staff user, the store reviewers come from.
func seedReviewer(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	userID := uuid.New()
	_, err := env.superPool.Exec(context.Background(),
		`INSERT INTO dcim.users (id, name, email) VALUES ($1, $2, $2 || '@fundament.test')`, userID, name)
	require.NoError(t, err)

	return userID
}

func TestListReviewersResolvesDecidingStaff(t *testing.T) {
	env := newTestEnv(t)
	first := seedPendingSubmission(t, env, "cert-manager")
	second := seedPendingSubmission(t, env, "postgres-operator")
	alice := seedReviewer(t, env, "Alice")
	bob := seedReviewer(t, env, "Bob")
	// Staff who never decided anything are not listed.
	seedReviewer(t, env, "Carol")

	_, err := newClient(t, env, authFor(t, alice)).ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{SubmissionId: first.submissionID.String()}.Build())
	require.NoError(t, err)
	_, err = newClient(t, env, authFor(t, bob)).ApproveSubmission(context.Background(),
		adminv1.ApproveSubmissionRequest_builder{SubmissionId: second.submissionID.String()}.Build())
	require.NoError(t, err)

	// Bob has left since: he still made the decision, so he still resolves.
	_, err = env.superPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, bob)
	require.NoError(t, err)

	resp, err := newReviewer(t, env).ListReviewers(context.Background(),
		adminv1.ListReviewersRequest_builder{}.Build())
	require.NoError(t, err)

	names := make(map[string]string, len(resp.GetReviewers()))
	for _, reviewer := range resp.GetReviewers() {
		names[reviewer.GetId()] = reviewer.GetName()
	}
	assert.Equal(t, map[string]string{alice.String(): "Alice", bob.String(): "Bob"}, names)
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

// completeAnalysis records a finished analysis for a version, as the worker
// would.
func completeAnalysis(t *testing.T, env *testEnv, versionID uuid.UUID, packages, files string) {
	t.Helper()

	_, err := env.superPool.Exec(context.Background(), `
		INSERT INTO appstore.plugin_image_analyses
			(plugin_definition_id, status, image_digest, platform, packages, files, sbom, analyzed)
		VALUES ($1, 'completed', 'sha256:img', 'linux/amd64', $2, $3, '{"bomFormat":"CycloneDX"}', now())
		ON CONFLICT ON CONSTRAINT plugin_image_analyses_uq_plugin_definition_id DO UPDATE SET
			status = 'completed', image_digest = 'sha256:img', platform = 'linux/amd64',
			packages = EXCLUDED.packages, files = EXCLUDED.files, sbom = EXCLUDED.sbom, analyzed = now()`,
		versionID, packages, files)
	require.NoError(t, err)
}

func TestGetImageAnalysisDiffsAgainstBase(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	baseID := seedVersion(t, env, seeded.pluginID, "1.0.0", "approved")
	completeAnalysis(t, env, baseID,
		`[{"purl":"pkg:deb/debian/openssl@3.0.13","name":"openssl","version":"3.0.13","type":"deb","licenses":[]},
		  {"purl":"pkg:deb/debian/zlib@1.3","name":"zlib","version":"1.3","type":"deb","licenses":[]}]`,
		`[{"path":"/a","size":1,"mode":420,"sha256":"1"},{"path":"/b","size":1,"mode":420,"sha256":"2"},
		  {"path":"/c","size":1,"mode":420,"sha256":"3"}]`)
	completeAnalysis(t, env, seeded.versionID,
		`[{"purl":"pkg:deb/debian/openssl@3.0.14","name":"openssl","version":"3.0.14","type":"deb","licenses":["Apache-2.0"]},
		  {"purl":"pkg:deb/debian/zlib@1.3","name":"zlib","version":"1.3","type":"deb","licenses":[]}]`,
		`[{"path":"/a","size":1,"mode":420,"sha256":"1"},{"path":"/b","size":2,"mode":420,"sha256":"changed"},
		  {"path":"/d","size":1,"mode":420,"sha256":"4"},{"path":"/e","size":1,"mode":420,"sha256":"5"}]`)

	client := newReviewer(t, env)
	resp, err := client.GetImageAnalysis(context.Background(), adminv1.GetImageAnalysisRequest_builder{
		PluginVersionId:     seeded.versionID.String(),
		BasePluginVersionId: baseID.String(),
		FilesChangesOnly:    true,
		FilesPageSize:       2,
	}.Build())
	require.NoError(t, err)

	assert.True(t, resp.GetCompared())
	assert.Equal(t, adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_COMPLETED, resp.GetAnalysis().GetStatus())
	assert.Equal(t, adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_COMPLETED, resp.GetBase().GetStatus())

	// Packages are served whole, the files filtered.
	require.Len(t, resp.GetPackages(), 2)
	assert.Equal(t, "openssl", resp.GetPackages()[0].GetName())
	assert.Equal(t, "3.0.14", resp.GetPackages()[0].GetVersion())
	assert.Equal(t, "3.0.13", resp.GetPackages()[0].GetBaseVersion())
	assert.Equal(t, adminv1.Change_CHANGE_CHANGED, resp.GetPackages()[0].GetChange())
	assert.Equal(t, adminv1.Change_CHANGE_UNCHANGED, resp.GetPackages()[1].GetChange())
	assert.Equal(t, int32(1), resp.GetPackageSummary().GetUnchanged())
	assert.Equal(t, int32(1), resp.GetPackageSummary().GetChanged())

	// /b changed, /c removed, /d and /e added: four of them, two per page.
	assert.Equal(t, int32(4), resp.GetFilesMatching())
	assert.Equal(t, int32(2), resp.GetFileSummary().GetAdded())
	paths := []string{}
	for _, f := range resp.GetFiles() {
		paths = append(paths, f.GetPath())
	}
	assert.Equal(t, []string{"/b", "/c"}, paths)
	require.NotEmpty(t, resp.GetNextFilesPageToken())

	next, err := client.GetImageAnalysis(context.Background(), adminv1.GetImageAnalysisRequest_builder{
		PluginVersionId:     seeded.versionID.String(),
		BasePluginVersionId: baseID.String(),
		FilesChangesOnly:    true,
		FilesPageSize:       2,
		FilesPageToken:      resp.GetNextFilesPageToken(),
	}.Build())
	require.NoError(t, err)
	paths = paths[:0]
	for _, f := range next.GetFiles() {
		paths = append(paths, f.GetPath())
	}
	assert.Equal(t, []string{"/d", "/e"}, paths)
	assert.Empty(t, next.GetNextFilesPageToken())
}

// A submission queues its analysis (the trigger), so a fresh version reads as
// pending, and its lists are empty until the worker is done.
func TestGetImageAnalysisWhilePending(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	resp, err := newReviewer(t, env).GetImageAnalysis(context.Background(), adminv1.GetImageAnalysisRequest_builder{
		PluginVersionId: seeded.versionID.String(),
	}.Build())
	require.NoError(t, err)

	assert.Equal(t, adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_PENDING, resp.GetAnalysis().GetStatus())
	assert.False(t, resp.GetCompared())
	assert.Empty(t, resp.GetPackages())
	assert.Empty(t, resp.GetFiles())
}

func TestGetSbomServesCompletedAnalysis(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	client := newReviewer(t, env)

	_, err := client.GetSbom(context.Background(),
		adminv1.GetSbomRequest_builder{PluginVersionId: seeded.versionID.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	completeAnalysis(t, env, seeded.versionID, `[]`, `[]`)
	resp, err := client.GetSbom(context.Background(),
		adminv1.GetSbomRequest_builder{PluginVersionId: seeded.versionID.String()}.Build())
	require.NoError(t, err)
	assert.JSONEq(t, `{"bomFormat":"CycloneDX"}`, string(resp.GetSbom()))
}
