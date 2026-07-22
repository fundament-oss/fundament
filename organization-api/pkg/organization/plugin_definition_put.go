package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func (s *Server) PutPluginDefinition(
	ctx context.Context,
	req *connect.Request[organizationv1.PutPluginDefinitionRequest],
) (*connect.Response[organizationv1.PutPluginDefinitionResponse], error) {
	pluginIDStr := req.Msg.GetPluginId()
	version := req.Msg.GetPluginVersion()
	manifest := req.Msg.GetManifest()
	if pluginIDStr == "" || version == "" || len(manifest) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("plugin_id, plugin_version and manifest are required"))
	}
	pluginID, err := uuid.Parse(pluginIDStr)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("plugin_id must be a valid uuid: %w", err))
	}
	// Strict parse rejects an image-free template (and any malformed manifest):
	// a stored PluginDefinition must be complete.
	def, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid manifest: %w", err))
	}
	// The version this definition is stored (and later fetched) under must match
	// the version the manifest declares — otherwise GetPluginDefinition, keyed on
	// (name, plugin_version), could never resolve a manifest whose internal
	// metadata.version disagrees with its storage key.
	if def.Metadata.Version != version {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("plugin_version %q does not match manifest metadata.version %q", version, def.Metadata.Version))
	}

	// Verify the catalog plugin exists and is not soft-deleted.
	catalogRow, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("plugin %s not found in catalog; create the plugin in the appstore before publishing a definition", pluginID))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("lookup catalog plugin: %w", err))
	}

	hash := pluginruntime.HashManifest(manifest)

	// Idempotent: exact (plugin_id, version, hash) already stored → return it.
	if existing, err := s.queries.PluginDefinitionGetByPluginVersionHash(ctx, db.PluginDefinitionGetByPluginVersionHashParams{
		PluginID: pluginID, PluginVersion: version, Hash: hash,
	}); err == nil {
		return connect.NewResponse(organizationv1.PutPluginDefinitionResponse_builder{
			Id: existing.ID.String(), PluginId: pluginID.String(), PluginVersion: version, Hash: hash,
		}.Build()), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("lookup plugin definition: %w", err))
	}

	// Same (plugin_id, version) with a different hash → republish requires soft-delete.
	if _, err := s.queries.PluginDefinitionGetActive(ctx, db.PluginDefinitionGetActiveParams{
		Name: catalogRow.Name, PluginVersion: version,
	}); err == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("plugin definition %s@%s already exists with a different hash; republish requires soft-delete", catalogRow.Name, version))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check existing plugin definition: %w", err))
	}

	inserted, err := s.queries.PluginDefinitionInsert(ctx, db.PluginDefinitionInsertParams{
		PluginID: pluginID, PluginVersion: version, Manifest: manifest, Hash: hash,
	})
	if err != nil {
		// A concurrent publish of a different manifest for the same
		// (plugin_id, version) races past the checks above and loses the unique
		// index. Surface it as the same FailedPrecondition the serial path
		// returns, not an opaque Internal error.
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok &&
			pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == dbconst.ConstraintPluginDefinitionsUqPluginVersion {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("plugin definition %s@%s already exists with a different hash; republish requires soft-delete", catalogRow.Name, version))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("insert plugin definition: %w", err))
	}
	return connect.NewResponse(organizationv1.PutPluginDefinitionResponse_builder{
		Id: inserted.ID.String(), PluginId: pluginID.String(), PluginVersion: version, Hash: hash,
	}.Build()), nil
}
