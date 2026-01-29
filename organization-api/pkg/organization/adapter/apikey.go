package adapter

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func ToAPIKeyCreate(req *organizationv1.CreateAPIKeyRequest) models.APIKeyCreate {
	var expiresInDays *int64
	if req.ExpiresInDays != nil {
		v := req.GetExpiresInDays()
		expiresInDays = &v
	}
	return models.APIKeyCreate{
		Name:          req.Name,
		ExpiresInDays: expiresInDays,
	}
}

func FromAPIKey(record *db.APIKeyGetByIDRow) *organizationv1.APIKey {
	apiKey := &organizationv1.APIKey{
		Id:          record.ID.String(),
		Name:        record.Name,
		TokenPrefix: record.TokenPrefix,
		CreatedAt: &organizationv1.Timestamp{
			Value: record.Created.Time.Format(time.RFC3339),
		},
	}
	if record.Expires.Valid {
		apiKey.ExpiresAt = &organizationv1.Timestamp{
			Value: record.Expires.Time.Format(time.RFC3339),
		}
	}
	if record.LastUsed.Valid {
		apiKey.LastUsedAt = &organizationv1.Timestamp{
			Value: record.LastUsed.Time.Format(time.RFC3339),
		}
	}
	if record.Revoked.Valid {
		apiKey.RevokedAt = &organizationv1.Timestamp{
			Value: record.Revoked.Time.Format(time.RFC3339),
		}
	}
	return apiKey
}

func FromAPIKeys(keys []db.APIKeyListByOrganizationIDRow) []*organizationv1.APIKey {
	result := make([]*organizationv1.APIKey, 0, len(keys))
	for idx := range keys {
		result = append(result, FromAPIKey((*db.APIKeyGetByIDRow)(&keys[idx])))
	}
	return result
}

func ToExpires(expiresInDays *int64) pgtype.Timestamptz {
	if expiresInDays == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{
		Time:  time.Now().AddDate(0, 0, int(*expiresInDays)),
		Valid: true,
	}
}
