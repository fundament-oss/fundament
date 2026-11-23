package pluginlisting

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/common/pluginlisting/db/gen"
)

// ErrInvalidDefinition marks a manifest whose listing fields cannot be synced.
var ErrInvalidDefinition = errors.New("invalid plugin definition")

// Sync updates the listing of a just-approved version, unless a newer version
// is already published: it copies the fields definition.yaml owns from the
// version's manifest, and applies the listing draft frozen on its open
// submission. tx is the approving transaction, before the round is closed.
func Sync(ctx context.Context, tx db.DBTX, versionID uuid.UUID) error {
	q := db.New(tx)

	version, err := q.VersionGetManifest(ctx, db.VersionGetManifestParams{ID: versionID})
	if err != nil {
		return fmt.Errorf("loading approved manifest: %w", err)
	}

	if _, err := q.PluginLock(ctx, db.PluginLockParams{ID: version.PluginID}); err != nil {
		return fmt.Errorf("locking plugin listing: %w", err)
	}

	others, err := q.VersionsPublishedExcept(ctx, db.VersionsPublishedExceptParams{PluginID: version.PluginID, ID: versionID})
	if err != nil {
		return fmt.Errorf("listing published versions: %w", err)
	}
	if !IsNewest(version.PluginVersion, others) {
		return nil
	}

	fields, err := FromManifest(version.Manifest)
	if err == nil {
		err = fields.Validate()
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if err := syncDefinitionFields(ctx, q, version.PluginID, &fields); err != nil {
		return err
	}

	raw, err := q.SubmissionOpenDraft(ctx, db.SubmissionOpenDraftParams{PluginDefinitionID: versionID})
	if err != nil {
		return fmt.Errorf("loading listing draft: %w", err)
	}
	draft, err := ParseDraft(raw)
	if err != nil || draft == nil {
		return err
	}
	if err := apply(ctx, q, version.PluginID, draft); err != nil {
		return err
	}
	if err := q.PluginDraftClear(ctx, db.PluginDraftClearParams{ID: version.PluginID, Applied: raw}); err != nil {
		return fmt.Errorf("clearing listing draft: %w", err)
	}
	return nil
}

func syncDefinitionFields(ctx context.Context, q *db.Queries, pluginID uuid.UUID, fields *Fields) error {
	if err := q.PluginSyncDefinitionFields(ctx, db.PluginSyncDefinitionFieldsParams{
		ID:               pluginID,
		DisplayName:      fields.DisplayName,
		DescriptionShort: fields.DescriptionShort,
		RepositoryUrl:    optionalText(fields.RepositoryURL),
		License:          fields.License,
	}); err != nil {
		return fmt.Errorf("syncing listing: %w", err)
	}

	if err := q.PluginTagsDelete(ctx, db.PluginTagsDeleteParams{PluginID: pluginID}); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}
	for _, name := range fields.Tags {
		// Insert-then-select: either this call created the tag or a committed
		// one already exists, so the select finds it in both cases.
		if err := q.TagInsertIfMissing(ctx, db.TagInsertIfMissingParams{Name: name}); err != nil {
			return fmt.Errorf("creating tag: %w", err)
		}
		tagID, err := q.TagGetByName(ctx, db.TagGetByNameParams{Name: name})
		if err != nil {
			return fmt.Errorf("resolving tag: %w", err)
		}
		if err := q.PluginTagInsert(ctx, db.PluginTagInsertParams{PluginID: pluginID, TagID: tagID}); err != nil {
			return fmt.Errorf("attaching tag: %w", err)
		}
	}
	return nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
