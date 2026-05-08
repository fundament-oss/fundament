package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func assetStatusToProto(status string) dcimv1.AssetStatus {
	switch status {
	case "in_stock":
		return dcimv1.AssetStatus_ASSET_STATUS_AVAILABLE
	case "deployed":
		return dcimv1.AssetStatus_ASSET_STATUS_DEPLOYED
	case "rma":
		return dcimv1.AssetStatus_ASSET_STATUS_NEEDS_REPAIR
	case "in_transit":
		return dcimv1.AssetStatus_ASSET_STATUS_ON_ORDER
	case "reserved":
		return dcimv1.AssetStatus_ASSET_STATUS_REQUESTED
	case "decommissioned":
		return dcimv1.AssetStatus_ASSET_STATUS_DECOMMISSIONED
	default:
		panic("unhandled asset status: " + status)
	}
}

func assetStatusToDB(status dcimv1.AssetStatus) string {
	switch status {
	case dcimv1.AssetStatus_ASSET_STATUS_AVAILABLE:
		return "in_stock"
	case dcimv1.AssetStatus_ASSET_STATUS_DEPLOYED:
		return "deployed"
	case dcimv1.AssetStatus_ASSET_STATUS_NEEDS_REPAIR:
		return "rma"
	case dcimv1.AssetStatus_ASSET_STATUS_ON_ORDER:
		return "in_transit"
	case dcimv1.AssetStatus_ASSET_STATUS_REQUESTED:
		return "reserved"
	case dcimv1.AssetStatus_ASSET_STATUS_DECOMMISSIONED:
		return "decommissioned"
	default:
		panic("unhandled asset status enum: " + status.String())
	}
}

func assetEventTypeToProto(eventType string) dcimv1.AssetEventType {
	switch eventType {
	case "received":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_RECEIVED
	case "deployed":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_DEPLOYED
	case "moved":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_MOVED
	case "rma_sent":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_REPAIR_SENT
	case "rma_received":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_REPAIR_RECEIVED
	case "decommissioned":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_DECOMMISSIONED
	case "reserved":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_REQUESTED
	case "note":
		return dcimv1.AssetEventType_ASSET_EVENT_TYPE_NOTE
	default:
		panic("unhandled asset event type: " + eventType)
	}
}

func assetFromGetRow(row *db.AssetGetByIDRow) *dcimv1.Asset {
	asset := dcimv1.Asset_builder{
		Id:              row.ID.String(),
		DeviceCatalogId: row.DeviceCatalogID.String(),
		Status:          assetStatusToProto(row.Status),
		Notes:           row.Notes.String,
		Created:         timestamppb.New(row.Created.Time),
	}.Build()

	if row.SerialNumber.Valid {
		asset.SetSerialNumber(row.SerialNumber.String)
	}

	if row.AssetTag.Valid {
		asset.SetAssetTag(row.AssetTag.String)
	}

	if row.PurchaseDate.Valid {
		asset.SetPurchaseDate(timestamppb.New(row.PurchaseDate.Time))
	}

	if row.PurchaseOrder.Valid {
		asset.SetPurchaseOrder(row.PurchaseOrder.String)
	}

	if row.WarrantyExpiry.Valid {
		asset.SetWarrantyExpiry(timestamppb.New(row.WarrantyExpiry.Time))
	}

	return asset
}

func assetFromListRow(row *db.AssetListRow) *dcimv1.Asset {
	asset := dcimv1.Asset_builder{
		Id:              row.ID.String(),
		DeviceCatalogId: row.DeviceCatalogID.String(),
		Status:          assetStatusToProto(row.Status),
		Notes:           row.Notes.String,
		Created:         timestamppb.New(row.Created.Time),
	}.Build()

	if row.SerialNumber.Valid {
		asset.SetSerialNumber(row.SerialNumber.String)
	}

	if row.AssetTag.Valid {
		asset.SetAssetTag(row.AssetTag.String)
	}

	if row.PurchaseDate.Valid {
		asset.SetPurchaseDate(timestamppb.New(row.PurchaseDate.Time))
	}

	if row.PurchaseOrder.Valid {
		asset.SetPurchaseOrder(row.PurchaseOrder.String)
	}

	if row.WarrantyExpiry.Valid {
		asset.SetWarrantyExpiry(timestamppb.New(row.WarrantyExpiry.Time))
	}

	return asset
}

func assetFromListByCatalogRow(row *db.AssetListByCatalogIDRow) *dcimv1.Asset {
	asset := dcimv1.Asset_builder{
		Id:              row.ID.String(),
		DeviceCatalogId: row.DeviceCatalogID.String(),
		Status:          assetStatusToProto(row.Status),
		Notes:           row.Notes.String,
		Created:         timestamppb.New(row.Created.Time),
	}.Build()

	if row.SerialNumber.Valid {
		asset.SetSerialNumber(row.SerialNumber.String)
	}

	if row.AssetTag.Valid {
		asset.SetAssetTag(row.AssetTag.String)
	}

	if row.PurchaseDate.Valid {
		asset.SetPurchaseDate(timestamppb.New(row.PurchaseDate.Time))
	}

	if row.PurchaseOrder.Valid {
		asset.SetPurchaseOrder(row.PurchaseOrder.String)
	}

	if row.WarrantyExpiry.Valid {
		asset.SetWarrantyExpiry(timestamppb.New(row.WarrantyExpiry.Time))
	}

	return asset
}

func assetEventFromRow(row *db.DcimAssetEvent) *dcimv1.AssetEvent {
	event := dcimv1.AssetEvent_builder{
		Id:        row.ID.String(),
		AssetId:   row.AssetID.String(),
		EventType: assetEventTypeToProto(row.EventType),
		Created:   timestamppb.New(row.Created.Time),
	}.Build()

	if row.Details.Valid {
		event.SetDetails(row.Details.String)
	}

	if row.PerformedBy.Valid {
		event.SetPerformedBy(row.PerformedBy.String)
	}

	return event
}
