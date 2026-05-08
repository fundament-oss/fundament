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

func noteEntityToCreateParams(entityType dcimv1.NoteEntityType, entityID uuid.UUID) db.NoteCreateParams {
	fk := pgtype.UUID{Bytes: entityID, Valid: true}
	var params db.NoteCreateParams

	switch entityType {
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_DEVICE_CATALOG:
		params.DeviceCatalogID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PORT_DEFINITION:
		params.PortDefinitionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ASSET:
		params.AssetID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_SITE:
		params.SiteID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ROOM:
		params.RoomID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK_ROW:
		params.RackRowID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK:
		params.RackID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PLACEMENT:
		params.PlacementID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PHYSICAL_CONNECTION:
		params.PhysicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DESIGN:
		params.LogicalDesignID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DEVICE:
		params.LogicalDeviceID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_CONNECTION:
		params.LogicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK:
		params.TaskID = fk
	default:
		panic(fmt.Sprintf("unknown note entity type: %d", entityType))
	}

	return params
}

func noteEntityToListParams(entityType dcimv1.NoteEntityType, entityID uuid.UUID) db.NoteListParams {
	fk := pgtype.UUID{Bytes: entityID, Valid: true}
	var params db.NoteListParams

	switch entityType {
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_DEVICE_CATALOG:
		params.DeviceCatalogID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PORT_DEFINITION:
		params.PortDefinitionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ASSET:
		params.AssetID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DESIGN:
		params.LogicalDesignID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_SITE:
		params.SiteID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_ROOM:
		params.RoomID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK_ROW:
		params.RackRowID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_RACK:
		params.RackID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PLACEMENT:
		params.PlacementID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_PHYSICAL_CONNECTION:
		params.PhysicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_DEVICE:
		params.LogicalDeviceID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_LOGICAL_CONNECTION:
		params.LogicalConnectionID = fk
	case dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK:
		params.TaskID = fk
	default:
		panic(fmt.Sprintf("unknown note entity type: %d", entityType))
	}

	return params
}
