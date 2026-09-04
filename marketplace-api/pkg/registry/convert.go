package registry

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	registryv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1"
)

func textOrEmpty(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func textOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// uuidOrEmpty renders an absent id as "" rather than an all-zeros UUID, which a
// client would otherwise have to recognise as meaning "none".
func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func timestamptzOrNil(value pgtype.Timestamptz) *timestamppb.Timestamp {
	if !value.Valid {
		return nil
	}
	return timestamppb.New(value.Time)
}

func visibilityFromDB(visibility dbconst.PluginVisibility) registryv1.PluginVisibility {
	switch visibility {
	case dbconst.PluginVisibility_Public:
		return registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC
	case dbconst.PluginVisibility_Restricted:
		return registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED
	default:
		panic("unhandled PluginVisibility: " + string(visibility))
	}
}

func visibilityToDB(visibility registryv1.PluginVisibility) string {
	switch visibility {
	case registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC:
		return string(dbconst.PluginVisibility_Public)
	case registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED:
		return string(dbconst.PluginVisibility_Restricted)
	case registryv1.PluginVisibility_PLUGIN_VISIBILITY_UNSPECIFIED:
		// protovalidate rejects UNSPECIFIED (not_in: [0]) before a handler runs.
		panic("PluginVisibility must be set")
	default:
		panic("unhandled PluginVisibility: " + visibility.String())
	}
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

// normalizeVersion strips the optional leading v so a request's "v1.2.3" and a
// manifest's "1.2.3" compare equal.
func normalizeVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}
