package admin

import (
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

func timestamptzOrNil(value pgtype.Timestamptz) *timestamppb.Timestamp {
	if !value.Valid {
		return nil
	}
	return timestamppb.New(value.Time)
}

// pgUUIDOrEmpty renders an absent id as "" rather than an all-zeros UUID,
// which a client would otherwise have to recognise as meaning "none".
func pgUUIDOrEmpty(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func statusFromDB(status dbconst.PluginDefinitionStatus) marketplacev1.SubmissionStatus {
	switch status {
	case dbconst.PluginDefinitionStatus_Draft:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_DRAFT
	case dbconst.PluginDefinitionStatus_Pending:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING
	case dbconst.PluginDefinitionStatus_ChangesRequested:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_CHANGES_REQUESTED
	case dbconst.PluginDefinitionStatus_Approved:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_APPROVED
	case dbconst.PluginDefinitionStatus_Rejected:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_REJECTED
	case dbconst.PluginDefinitionStatus_Withdrawn:
		return marketplacev1.SubmissionStatus_SUBMISSION_STATUS_WITHDRAWN
	default:
		panic("unhandled PluginDefinitionStatus: " + string(status))
	}
}

func rejectionReasonToDB(reason adminv1.RejectionReason) string {
	switch reason {
	case adminv1.RejectionReason_REJECTION_REASON_INCOMPLETE_METADATA:
		return string(dbconst.SubmissionRejectionReason_IncompleteMetadata)
	case adminv1.RejectionReason_REJECTION_REASON_DUPLICATE:
		return string(dbconst.SubmissionRejectionReason_Duplicate)
	case adminv1.RejectionReason_REJECTION_REASON_SECURITY_CONCERNS:
		return string(dbconst.SubmissionRejectionReason_SecurityConcerns)
	case adminv1.RejectionReason_REJECTION_REASON_NAMING_GUIDELINES:
		return string(dbconst.SubmissionRejectionReason_NamingGuidelines)
	case adminv1.RejectionReason_REJECTION_REASON_OUT_OF_SCOPE:
		return string(dbconst.SubmissionRejectionReason_OutOfScope)
	case adminv1.RejectionReason_REJECTION_REASON_OTHER:
		return string(dbconst.SubmissionRejectionReason_Other)
	case adminv1.RejectionReason_REJECTION_REASON_UNSPECIFIED:
		// protovalidate rejects UNSPECIFIED (not_in: [0]) before a handler runs.
		panic("RejectionReason must be set")
	default:
		panic("unhandled RejectionReason: " + reason.String())
	}
}

// rejectionReasonFromDB maps the stored value back; NULL (rendered as "")
// means no decision or a non-reject decision, which is UNSPECIFIED on the wire.
func rejectionReasonFromDB(reason pgtype.Text) adminv1.RejectionReason {
	if !reason.Valid {
		return adminv1.RejectionReason_REJECTION_REASON_UNSPECIFIED
	}
	switch dbconst.SubmissionRejectionReason(reason.String) {
	case dbconst.SubmissionRejectionReason_IncompleteMetadata:
		return adminv1.RejectionReason_REJECTION_REASON_INCOMPLETE_METADATA
	case dbconst.SubmissionRejectionReason_Duplicate:
		return adminv1.RejectionReason_REJECTION_REASON_DUPLICATE
	case dbconst.SubmissionRejectionReason_SecurityConcerns:
		return adminv1.RejectionReason_REJECTION_REASON_SECURITY_CONCERNS
	case dbconst.SubmissionRejectionReason_NamingGuidelines:
		return adminv1.RejectionReason_REJECTION_REASON_NAMING_GUIDELINES
	case dbconst.SubmissionRejectionReason_OutOfScope:
		return adminv1.RejectionReason_REJECTION_REASON_OUT_OF_SCOPE
	case dbconst.SubmissionRejectionReason_Other:
		return adminv1.RejectionReason_REJECTION_REASON_OTHER
	default:
		panic("unhandled SubmissionRejectionReason: " + reason.String)
	}
}
