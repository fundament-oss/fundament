package adapter

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		CreatedAt:   timestamppb.New(record.Created.Time),
	}
	if record.Expires.Valid {
		apiKey.ExpiresAt = timestamppb.New(record.Expires.Time)
	}
	if record.LastUsed.Valid {
		apiKey.LastUsedAt = timestamppb.New(record.LastUsed.Time)
	}
	if record.Revoked.Valid {
		apiKey.RevokedAt = timestamppb.New(record.Revoked.Time)
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
