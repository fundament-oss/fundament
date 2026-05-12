package dcim

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func noteFromListRow(row *db.NoteListRow) *dcimv1.Note {
	entityType, entityID := noteEntityFromRow(
		row.DeviceCatalogID,
		row.PortDefinitionID,
		row.AssetID,
		row.SiteID,
		row.RoomID,
		row.RackRowID,
		row.RackID,
		row.PlacementID,
		row.PhysicalConnectionID,
		row.LogicalDesignID,
		row.LogicalDeviceID,
		row.LogicalConnectionID,
		row.TaskID,
	)

	note := dcimv1.Note_builder{
		Id:         row.ID.String(),
		EntityType: entityType,
		EntityId:   entityID,
		Body:       row.Body,
		Created:    timestamppb.New(row.Created.Time),
	}.Build()

	if row.CreatedBy.Valid {
		note.SetCreatedBy(row.CreatedBy.String)
	}

	return note
}

func noteEntityFromRow(
	deviceCatalogID, portDefinitionID, assetID, siteID, roomID, rackRowID, rackID,
	placementID, physicalConnectionID, logicalDesignID, logicalDeviceID, logicalConnectionID, taskID pgtype.UUID,
) (dcimv1.NoteEntityType, string) {
	switch {
	case deviceCatalogID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_DEVICE_CATALOG, uuid.UUID(deviceCatalogID.Bytes).String()
	case portDefinitionID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PORT_DEFINITION, uuid.UUID(portDefinitionID.Bytes).String()
	case assetID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ASSET, uuid.UUID(assetID.Bytes).String()
	case siteID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_SITE, uuid.UUID(siteID.Bytes).String()
	case roomID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ROOM, uuid.UUID(roomID.Bytes).String()
	case rackRowID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK_ROW, uuid.UUID(rackRowID.Bytes).String()
	case rackID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK, uuid.UUID(rackID.Bytes).String()
	case placementID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PLACEMENT, uuid.UUID(placementID.Bytes).String()
	case physicalConnectionID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PHYSICAL_CONNECTION, uuid.UUID(physicalConnectionID.Bytes).String()
	case logicalDesignID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DESIGN, uuid.UUID(logicalDesignID.Bytes).String()
	case logicalDeviceID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DEVICE, uuid.UUID(logicalDeviceID.Bytes).String()
	case logicalConnectionID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_CONNECTION, uuid.UUID(logicalConnectionID.Bytes).String()
	case taskID.Valid:
		return dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK, uuid.UUID(taskID.Bytes).String()
	default:
		panic("note has no entity FK set")
	}
}

func assignNoteEntityFK(
	entityType dcimv1.NoteEntityType,
	entityID uuid.UUID,
	deviceCatalogID, portDefinitionID, assetID, siteID, roomID,
	rackRowID, rackID, placementID, physicalConnectionID,
	logicalDesignID, logicalDeviceID, logicalConnectionID, taskID *pgtype.UUID,
) error {
	fk := pgtype.UUID{Bytes: entityID, Valid: true}

	switch entityType {
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_DEVICE_CATALOG:
		*deviceCatalogID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PORT_DEFINITION:
		*portDefinitionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ASSET:
		*assetID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_SITE:
		*siteID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ROOM:
		*roomID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK_ROW:
		*rackRowID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK:
		*rackID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PLACEMENT:
		*placementID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PHYSICAL_CONNECTION:
		*physicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DESIGN:
		*logicalDesignID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DEVICE:
		*logicalDeviceID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_CONNECTION:
		*logicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK:
		*taskID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_UNSPECIFIED:
		return fmt.Errorf("entity_type is required")
	default:
		panic(fmt.Sprintf("unknown note entity type: %d", entityType))
	}

	return nil
}

func noteEntityToCreateParams(entityType dcimv1.NoteEntityType, entityID uuid.UUID) (db.NoteCreateParams, error) {
	var params db.NoteCreateParams
	err := assignNoteEntityFK(entityType, entityID,
		&params.DeviceCatalogID, &params.PortDefinitionID, &params.AssetID,
		&params.SiteID, &params.RoomID, &params.RackRowID, &params.RackID,
		&params.PlacementID, &params.PhysicalConnectionID,
		&params.LogicalDesignID, &params.LogicalDeviceID,
		&params.LogicalConnectionID, &params.TaskID,
	)
	return params, err
}

func noteEntityToListParams(entityType dcimv1.NoteEntityType, entityID uuid.UUID) (db.NoteListParams, error) {
	var params db.NoteListParams
	err := assignNoteEntityFK(entityType, entityID,
		&params.DeviceCatalogID, &params.PortDefinitionID, &params.AssetID,
		&params.SiteID, &params.RoomID, &params.RackRowID, &params.RackID,
		&params.PlacementID, &params.PhysicalConnectionID,
		&params.LogicalDesignID, &params.LogicalDeviceID,
		&params.LogicalConnectionID, &params.TaskID,
	)
	return params, err
}
