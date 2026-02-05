package organization

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func apiKeyFromGetRow(record *db.APIKeyGetByIDRow) *organizationv1.APIKey {
	apiKey := &organizationv1.APIKey{
		Id:          record.ID.String(),
		Name:        record.Name,
		TokenPrefix: record.TokenPrefix,
		Created:     timestamppb.New(record.Created.Time),
	}
	if record.Expires.Valid {
		apiKey.Expires = timestamppb.New(record.Expires.Time)
	}
	if record.LastUsed.Valid {
		apiKey.LastUsed = timestamppb.New(record.LastUsed.Time)
	}
	if record.Revoked.Valid {
		apiKey.Revoked = timestamppb.New(record.Revoked.Time)
	}
	return apiKey
}

func apiKeyFromListRow(record *db.APIKeyListByOrganizationIDRow) *organizationv1.APIKey {
	apiKey := &organizationv1.APIKey{
		Id:          record.ID.String(),
		Name:        record.Name,
		TokenPrefix: record.TokenPrefix,
		Created:     timestamppb.New(record.Created.Time),
	}
	if record.Expires.Valid {
		apiKey.Expires = timestamppb.New(record.Expires.Time)
	}
	if record.LastUsed.Valid {
		apiKey.LastUsed = timestamppb.New(record.LastUsed.Time)
	}
	if record.Revoked.Valid {
		apiKey.Revoked = timestamppb.New(record.Revoked.Time)
	}
	return apiKey
}
