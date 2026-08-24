package proto_test

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/admin/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

func TestRejectSubmissionRequestValidation(t *testing.T) {
	t.Run("accepts a concrete reason with feedback", func(t *testing.T) {
		req := adminv1.RejectSubmissionRequest_builder{
			SubmissionId: testSubmissionID,
			Reason:       adminv1.RejectionReason_REJECTION_REASON_SECURITY_CONCERNS,
			Feedback:     "The backup controller requests cluster-admin. Please scope it down.",
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects an unspecified reason", func(t *testing.T) {
		req := adminv1.RejectSubmissionRequest_builder{
			SubmissionId: testSubmissionID,
			Reason:       adminv1.RejectionReason_REJECTION_REASON_UNSPECIFIED,
			Feedback:     "No.",
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid submission id", func(t *testing.T) {
		req := adminv1.RejectSubmissionRequest_builder{
			SubmissionId: "quick-backup",
			Reason:       adminv1.RejectionReason_REJECTION_REASON_DUPLICATE,
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestRequestChangesRequestValidation(t *testing.T) {
	t.Run("accepts feedback explaining what to change", func(t *testing.T) {
		req := adminv1.RequestChangesRequest_builder{
			SubmissionId: testSubmissionID,
			Feedback:     "Requested RBAC scope is too broad. Narrow the ClusterRole and resubmit.",
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects empty feedback", func(t *testing.T) {
		// Requesting changes without saying which is a dead end for the developer.
		req := adminv1.RequestChangesRequest_builder{
			SubmissionId: testSubmissionID,
			Feedback:     "",
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestListSubmissionsRequestValidation(t *testing.T) {
	t.Run("accepts an empty request", func(t *testing.T) {
		require.NoError(t, protovalidate.Validate(adminv1.ListSubmissionsRequest_builder{}.Build()))
	})

	t.Run("accepts a status filter", func(t *testing.T) {
		// One vocabulary, one definition — the enum comes from marketplace.v1,
		// not from a copy in this package.
		req := adminv1.ListSubmissionsRequest_builder{
			Status: marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING,
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects an out-of-range status", func(t *testing.T) {
		req := adminv1.ListSubmissionsRequest_builder{
			Status: marketplacev1.SubmissionStatus(99),
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestSubmissionCarriesIdentityAndDecisionOnly(t *testing.T) {
	// No listing snapshot and no captured submitter identity: a submission
	// points at what is under review and records what was decided.
	submission := adminv1.Submission_builder{
		Id:              testSubmissionID,
		PluginId:        testPluginID,
		PluginVersionId: testPluginVersionID,
		OrganizationId:  testOrganizationID,
		SubmitterUserId: testUserID,
		Status:          marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING,
	}.Build()

	assert.Equal(t, testPluginID, submission.GetPluginId())
	assert.Equal(t, testPluginVersionID, submission.GetPluginVersionId())
	assert.Equal(t, testOrganizationID, submission.GetOrganizationId())
	assert.Equal(t, testUserID, submission.GetSubmitterUserId())
}

func TestAdminGetPluginRequestValidation(t *testing.T) {
	t.Run("accepts a uuid plugin id", func(t *testing.T) {
		req := adminv1.GetPluginRequest_builder{PluginId: testPluginID}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid plugin id", func(t *testing.T) {
		req := adminv1.GetPluginRequest_builder{PluginId: "postgres-operator"}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestGetPluginVersionRequestValidation(t *testing.T) {
	t.Run("accepts a uuid plugin version id", func(t *testing.T) {
		req := adminv1.GetPluginVersionRequest_builder{PluginVersionId: testPluginVersionID}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid plugin version id", func(t *testing.T) {
		req := adminv1.GetPluginVersionRequest_builder{PluginVersionId: "v1.2.3"}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestReviewPayloadLivesOnTheVersion(t *testing.T) {
	// definition_hash, capabilities and permissions are properties of the
	// version, not of the submission that points at it.
	version := adminv1.PluginVersion_builder{
		Id:             testPluginVersionID,
		PluginId:       testPluginID,
		Version:        "v1.17.2",
		DefinitionHash: "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
		Capabilities:   []string{"internet_access"},
		Permissions: []*marketplacev1.PluginPermission{
			marketplacev1.PluginPermission_builder{Resource: "Certificates", Access: "Read and write"}.Build(),
		},
		Status: marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING,
	}.Build()

	assert.Equal(t, []string{"internet_access"}, version.GetCapabilities())
	require.Len(t, version.GetPermissions(), 1)
	assert.Equal(t, "Certificates", version.GetPermissions()[0].GetResource())
}

func TestListPublishersRequestValidation(t *testing.T) {
	// The reviewer's only route to a publishing organization's name: the
	// submission carries organization_id and organization-api's
	// GetOrganization is scoped to members, which a reviewer is not.
	require.NoError(t, protovalidate.Validate(adminv1.ListPublishersRequest_builder{}.Build()))
}
