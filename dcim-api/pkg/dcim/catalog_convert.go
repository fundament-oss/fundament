package dcim

import (
	"encoding/json"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func assetCategoryToProto(category string) dcimv1.AssetCategory {
	switch category {
	case "server":
		return dcimv1.AssetCategory_ASSET_CATEGORY_SERVER
	case "switch":
		return dcimv1.AssetCategory_ASSET_CATEGORY_SWITCH
	case "pdu":
		return dcimv1.AssetCategory_ASSET_CATEGORY_PDU
	case "patch_panel":
		return dcimv1.AssetCategory_ASSET_CATEGORY_PATCH_PANEL
	case "sfp":
		return dcimv1.AssetCategory_ASSET_CATEGORY_SFP
	case "nic":
		return dcimv1.AssetCategory_ASSET_CATEGORY_NIC
	case "cpu":
		return dcimv1.AssetCategory_ASSET_CATEGORY_CPU
	case "dimm":
		return dcimv1.AssetCategory_ASSET_CATEGORY_DIMM
	case "disk":
		return dcimv1.AssetCategory_ASSET_CATEGORY_DISK
	case "cable":
		return dcimv1.AssetCategory_ASSET_CATEGORY_CABLE
	case "adapter":
		return dcimv1.AssetCategory_ASSET_CATEGORY_ADAPTER
	case "power_supply":
		return dcimv1.AssetCategory_ASSET_CATEGORY_POWER_SUPPLY
	case "cable_manager":
		return dcimv1.AssetCategory_ASSET_CATEGORY_CABLE_MANAGER
	case "console_server":
		return dcimv1.AssetCategory_ASSET_CATEGORY_CONSOLE_SERVER
	case "storage":
		return dcimv1.AssetCategory_ASSET_CATEGORY_STORAGE
	case "cooling":
		return dcimv1.AssetCategory_ASSET_CATEGORY_COOLING
	case "firewall":
		return dcimv1.AssetCategory_ASSET_CATEGORY_FIREWALL
	case "kvm":
		return dcimv1.AssetCategory_ASSET_CATEGORY_KVM
	case "gpu":
		return dcimv1.AssetCategory_ASSET_CATEGORY_GPU
	case "transceiver":
		return dcimv1.AssetCategory_ASSET_CATEGORY_TRANSCEIVER
	case "other":
		return dcimv1.AssetCategory_ASSET_CATEGORY_OTHER
	default:
		panic("unhandled asset category: " + category)
	}
}

func assetCategoryToDB(category dcimv1.AssetCategory) string {
	switch category {
	case dcimv1.AssetCategory_ASSET_CATEGORY_SERVER:
		return "server"
	case dcimv1.AssetCategory_ASSET_CATEGORY_SWITCH:
		return "switch"
	case dcimv1.AssetCategory_ASSET_CATEGORY_PDU:
		return "pdu"
	case dcimv1.AssetCategory_ASSET_CATEGORY_PATCH_PANEL:
		return "patch_panel"
	case dcimv1.AssetCategory_ASSET_CATEGORY_SFP:
		return "sfp"
	case dcimv1.AssetCategory_ASSET_CATEGORY_NIC:
		return "nic"
	case dcimv1.AssetCategory_ASSET_CATEGORY_CPU:
		return "cpu"
	case dcimv1.AssetCategory_ASSET_CATEGORY_DIMM:
		return "dimm"
	case dcimv1.AssetCategory_ASSET_CATEGORY_DISK:
		return "disk"
	case dcimv1.AssetCategory_ASSET_CATEGORY_CABLE:
		return "cable"
	case dcimv1.AssetCategory_ASSET_CATEGORY_ADAPTER:
		return "adapter"
	case dcimv1.AssetCategory_ASSET_CATEGORY_POWER_SUPPLY:
		return "power_supply"
	case dcimv1.AssetCategory_ASSET_CATEGORY_CABLE_MANAGER:
		return "cable_manager"
	case dcimv1.AssetCategory_ASSET_CATEGORY_CONSOLE_SERVER:
		return "console_server"
	case dcimv1.AssetCategory_ASSET_CATEGORY_STORAGE:
		return "storage"
	case dcimv1.AssetCategory_ASSET_CATEGORY_COOLING:
		return "cooling"
	case dcimv1.AssetCategory_ASSET_CATEGORY_FIREWALL:
		return "firewall"
	case dcimv1.AssetCategory_ASSET_CATEGORY_KVM:
		return "kvm"
	case dcimv1.AssetCategory_ASSET_CATEGORY_GPU:
		return "gpu"
	case dcimv1.AssetCategory_ASSET_CATEGORY_TRANSCEIVER:
		return "transceiver"
	case dcimv1.AssetCategory_ASSET_CATEGORY_OTHER:
		return "other"
	default:
		panic("unhandled asset category enum: " + category.String())
	}
}

func specsToProto(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var specs map[string]string
	if err := json.Unmarshal(data, &specs); err != nil {
		return nil
	}
	return specs
}

func specsToDB(specs map[string]string) []byte {
	if len(specs) == 0 {
		return nil
	}
	data, err := json.Marshal(specs)
	if err != nil {
		return nil
	}
	return data
}

func catalogFromGetRow(row *db.DeviceCatalogGetByIDRow) *dcimv1.DeviceCatalog {
	entry := dcimv1.DeviceCatalog_builder{
		Id:           row.ID.String(),
		Manufacturer: row.Manufacturer,
		Model:        row.Model,
		Category:     assetCategoryToProto(row.Category),
		Specs:        specsToProto(row.Specs),
		Created:      timestamppb.New(row.Created.Time),
	}.Build()

	if row.PartNumber.Valid {
		entry.SetPartNumber(row.PartNumber.String)
	}

	if row.FormFactor.Valid {
		entry.SetFormFactor(row.FormFactor.String)
	}

	if row.RackUnits.Valid {
		entry.SetRackUnits(row.RackUnits.Int32)
	}

	if row.WeightKg.Valid {
		entry.SetWeightKg(numericToFloat64(row.WeightKg))
	}

	if row.PowerDrawW.Valid {
		entry.SetPowerDrawW(numericToFloat64(row.PowerDrawW))
	}

	return entry
}

func catalogFromListRow(row *db.DeviceCatalogListRow) *dcimv1.DeviceCatalog {
	entry := dcimv1.DeviceCatalog_builder{
		Id:           row.ID.String(),
		Manufacturer: row.Manufacturer,
		Model:        row.Model,
		Category:     assetCategoryToProto(row.Category),
		Specs:        specsToProto(row.Specs),
		Created:      timestamppb.New(row.Created.Time),
	}.Build()

	if row.PartNumber.Valid {
		entry.SetPartNumber(row.PartNumber.String)
	}

	if row.FormFactor.Valid {
		entry.SetFormFactor(row.FormFactor.String)
	}

	if row.RackUnits.Valid {
		entry.SetRackUnits(row.RackUnits.Int32)
	}

	if row.WeightKg.Valid {
		entry.SetWeightKg(numericToFloat64(row.WeightKg))
	}

	if row.PowerDrawW.Valid {
		entry.SetPowerDrawW(numericToFloat64(row.PowerDrawW))
	}

	return entry
}
