package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openfga "github.com/openfga/go-sdk"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/authz"
)

// ApiKey syncs an API key's organization and user relationships to OpenFGA.
func (h *Handler) ApiKey(ctx context.Context, qtx *db.Queries, apiKeyID uuid.UUID) error {
	apiKey, err := qtx.GetApiKeyByID(ctx, db.GetApiKeyByIDParams{ID: apiKeyID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("api key not found: %s", apiKeyID)
		}

		return fmt.Errorf("get api key: %w", err)
	}

	h.logger.DebugContext(ctx, "handle api_key", "api_key", apiKey)

	orgObj := authz.Organization(apiKey.OrganizationID)
	userObj := authz.User(apiKey.UserID)
	apiKeyObj := authz.ApiKey(apiKey.ID)

	if apiKey.Deleted.Valid || apiKey.Revoked.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(orgObj, authz.ActionOwner, apiKeyObj),
			tupleDelete(userObj, authz.ActionCreator, apiKeyObj),
		)
	}

	tupleCreator := tuple(userObj, authz.ActionCreator, apiKeyObj)

	// Add is_not_expired self-relation for can_use check
	if apiKey.Expires.Valid {
		tupleCreator.Condition = &openfga.RelationshipCondition{
			Name: "is_not_expired",
			Context: &map[string]any{
				"expiration": apiKey.Expires.Time.Format(time.RFC3339),
			},
		}
	}

	return h.writeTuples(ctx,
		tuple(orgObj, authz.ActionOwner, apiKeyObj),
		tupleCreator,
	)
}
