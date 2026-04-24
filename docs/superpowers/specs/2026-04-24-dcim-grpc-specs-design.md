# DCIM gRPC Specs Design

## Overview

Add gRPC/protobuf service definitions for the DCIM (DataCenter Infrastructure Management) system to the fundament monorepo. This covers device catalog, asset inventory, physical infrastructure (sites/rooms/rows/racks), placement tracking, physical connections, polymorphic notes, and logical design (topology modeling).

Based on two POCs:
- `fundament-dcim` — Angular + protobuf POC with 10 proto files
- `dcim-spike` — Go + SQLite spike that adds the logical design layer

## Scope

**In scope:** gRPC proto files only (no Go implementation, no DB schema, no frontend).

**Entities included:**
- Device Catalog (DeviceCatalog, PortDefinition, PortCompatibility)
- Asset Inventory (Asset, AssetEvent, AssetStats)
- Physical Hierarchy (Site, Room, RackRow, Rack)
- Placement (recursive asset-to-location mapping)
- Physical Connections (cable management)
- Notes (polymorphic comments on any entity)
- Logical Design (LogicalDesign, LogicalDevice, LogicalConnection, LogicalDeviceLayout)

**Excluded:** Tasks, Technicians (deferred to later iteration).

## File Structure

```
dcim-api/
  pkg/
    proto/
      buf.yaml
      buf.gen.yaml
      v1/
        common.proto
        catalog.proto
        asset.proto
        site.proto
        rack.proto
        placement.proto
        connection.proto
        note.proto
        design.proto
```

## Conventions

Following existing fundament monorepo patterns:

- **Edition:** `edition = "2023"`
- **Package:** `dcim.v1`
- **Go package:** `github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1`
- **Features:** `API_OPAQUE`, `field_presence = IMPLICIT` (default), `EXPLICIT` for optional fields
- **Field numbering:** Multiples of 10 (10, 20, 30...) for future insertions
- **Enum values:** `RESOURCE_FIELD_UNSPECIFIED = 0`, then values at multiples of 10
- **Validation:** `buf/validate/validate.proto` for input constraints
- **Timestamps:** `google.protobuf.Timestamp` throughout
- **Soft deletes:** `deleted` timestamp with `EXPLICIT` field presence on all persistent entities
- **Dependencies:** `buf.build/bufbuild/protovalidate`
- **Code gen:** `protoc-gen-go` + `protoc-gen-connect-go`

## Proto Definitions

### common.proto

Shared enums used across multiple files. Taken directly from fundament-dcim POC.

**Enums:**
- `AssetCategory` — server, switch, pdu, patch_panel, sfp, nic, cpu, dimm, disk, cable, adapter, power_supply, cable_manager, console_server
- `AssetStatus` — in_stock, deployed, rma, decommissioned, in_transit, reserved
- `AssetEventType` — received, deployed, moved, rma_sent, rma_received, decommissioned, reserved, note
- `RackSlotType` — unit, power, zero_u
- `PortType` — network, power_in, power_out, slot, bay, console
- `PortDirection` — in, out, bidir
- `NoteEntityType` — device_catalog, port_definition, asset, site, room, rack_row, rack, placement, physical_connection, logical_design, logical_device, logical_connection

**Messages:**
- `AssetEvent` — id, asset_id, event_type, details, performed_by, created (Timestamp, not string — fix from POC)

### catalog.proto

Device type definitions and port/compatibility specifications.

**Messages:**
- `DeviceCatalog` — id, manufacturer, model, part_number, category, form_factor, rack_units (optional), weight_kg, power_draw_w, specs (map<string,string>), created, deleted
- `PortDefinition` — id, device_catalog_id, name, port_type, media_type, speed (optional), max_power_w (optional), direction, ordinal
- `PortCompatibility` — port_definition_id, compatible_catalog_id, notes

**CatalogService RPCs:**
- `ListCatalog(category_filter, search)` — returns CatalogSummary (entry + asset count stats)
- `GetCatalogEntry(id)` — returns DeviceCatalog
- `CreateCatalogEntry(manufacturer, model, part_number, category, form_factor, rack_units?, weight_kg, power_draw_w, specs)` — returns DeviceCatalog
- `UpdateCatalogEntry(id, manufacturer?, model?, part_number?, form_factor?, rack_units?, weight_kg?, power_draw_w?, specs?)` — returns DeviceCatalog
- `DeleteCatalogEntry(id)` — soft delete
- `ListAssetsByCatalogEntry(device_catalog_id)` — returns repeated Asset
- `ListPortDefinitions(device_catalog_id)` — returns repeated PortDefinition
- `GetPortDefinition(id)` — returns PortDefinition
- `CreatePortDefinition(device_catalog_id, name, port_type, media_type, speed?, max_power_w?, direction, ordinal)` — returns PortDefinition
- `UpdatePortDefinition(id, name?, port_type?, media_type?, speed?, max_power_w?, direction?, ordinal?)` — returns PortDefinition
- `DeletePortDefinition(id)` — hard delete (no soft delete on port definitions)
- `ListPortCompatibilities(port_definition_id)` — returns repeated PortCompatibility
- `CreatePortCompatibility(port_definition_id, compatible_catalog_id, notes)` — returns PortCompatibility
- `DeletePortCompatibility(port_definition_id, compatible_catalog_id)` — hard delete

### asset.proto

Physical inventory item tracking with event log and statistics.

**Messages:**
- `Asset` — id, device_catalog_id, status, serial_number?, asset_tag?, purchase_date?, purchase_order?, warranty_expiry?, notes, created, deleted
- `AssetStats` — total, in_stock, deployed, available, rma, decommissioned

**Enums:**
- `AssetSortField` — status, serial_number, asset_tag, purchase_date, warranty_expiry
- `SortDirection` — asc, desc

**AssetService RPCs:**
- `ListAssets(status_filter?, category_filter?, device_catalog_id?, search, sort_by, sort_direction, include_deleted)` — returns repeated Asset
- `GetAsset(id)` — returns Asset
- `CreateAsset(device_catalog_id, status, serial_number?, asset_tag?, purchase_date?, purchase_order?, warranty_expiry?, notes)` — returns Asset
- `UpdateAsset(id, status?, serial_number?, asset_tag?, warranty_expiry?, notes?)` — returns Asset
- `DeleteAsset(id)` — soft delete
- `GetAssetEvents(asset_id)` — returns repeated AssetEvent
- `GetAssetStats(site_id?)` — returns AssetStats

### site.proto

Physical datacenter hierarchy: Site > Room > RackRow. Full CRUD on all three.

**Messages:**
- `Site` — id, name, address
- `Room` — id, site_id, name, floor
- `RackRow` — id, room_id, name, position_x?, position_y?

**SiteService RPCs:**
- `ListSites()` — returns repeated Site
- `GetSite(id)` — returns Site
- `CreateSite(name, address)` — returns Site
- `UpdateSite(id, name?, address?)` — returns Site
- `DeleteSite(id)` — soft delete

**RoomService RPCs:**
- `ListRooms(site_id?)` — returns repeated Room
- `GetRoom(id)` — returns Room
- `CreateRoom(site_id, name, floor)` — returns Room
- `UpdateRoom(id, name?, floor?)` — returns Room
- `DeleteRoom(id)` — soft delete

**RackRowService RPCs:**
- `ListRackRows(room_id?)` — returns repeated RackRow
- `GetRackRow(id)` — returns RackRow
- `CreateRackRow(room_id, name, position_x?, position_y?)` — returns RackRow
- `UpdateRackRow(id, name?, position_x?, position_y?)` — returns RackRow
- `DeleteRackRow(id)` — soft delete

### rack.proto

Equipment racks within rows. Full CRUD.

**Messages:**
- `Rack` — id, row_id, name, total_units, position_in_row

**RackService RPCs:**
- `ListRacks(row_id?)` — returns repeated RackSummary (rack + used_units, free_units, power_draw_w, device_count, utilization_pct)
- `GetRack(id)` — returns Rack
- `CreateRack(row_id, name, total_units, position_in_row)` — returns Rack
- `UpdateRack(id, name?, total_units?, position_in_row?)` — returns Rack
- `DeleteRack(id)` — soft delete

### placement.proto

Maps assets to physical locations. Recursive model: either in a rack (top-level) or inside another placement (sub-component). Unchanged from POC.

**Messages:**
- `Placement` — id, asset_id, rack_id?, rack_unit_start?, rack_slot_type?, parent_placement_id?, parent_port_name?, logical_device_id?, notes, created, deleted

**PlacementService RPCs:**
- `CreatePlacement(asset_id, rack_id?, rack_unit_start?, rack_slot_type?, parent_placement_id?, parent_port_name?, logical_device_id?, notes)` — returns Placement
- `GetPlacement(id)` — returns Placement
- `UpdatePlacement(id, rack_id?, rack_unit_start?, rack_slot_type?, parent_placement_id?, parent_port_name?, logical_device_id?, notes?)` — returns Placement
- `DeletePlacement(id)` — soft delete
- `ListPlacementsByRack(rack_id)` — returns repeated Placement
- `ListChildPlacements(parent_placement_id)` — returns repeated Placement

### connection.proto

Physical cable connections between placement ports. Unchanged from POC.

**Messages:**
- `PhysicalConnection` — id, source_placement_id, source_port_name, target_placement_id, target_port_name, cable_asset_id?, logical_connection_id?, notes, created, deleted

**PhysicalConnectionService RPCs:**
- `CreatePhysicalConnection(source_placement_id, source_port_name, target_placement_id, target_port_name, cable_asset_id?, logical_connection_id?, notes)` — returns PhysicalConnection
- `GetPhysicalConnection(id)` — returns PhysicalConnection
- `UpdatePhysicalConnection(id, cable_asset_id?, logical_connection_id?, notes?)` — returns PhysicalConnection
- `DeletePhysicalConnection(id)` — soft delete
- `ListConnectionsByPlacement(placement_id)` — returns repeated PhysicalConnection

### note.proto

Polymorphic comments on any DCIM entity. Unchanged from POC except NoteEntityType additions.

**Messages:**
- `Note` — id, entity_type, entity_id, body, created_by, created, deleted

**NoteService RPCs:**
- `ListNotes(entity_type, entity_id)` — returns repeated Note
- `CreateNote(entity_type, entity_id, body, created_by)` — returns Note
- `DeleteNote(id)` — soft delete

### design.proto (new)

Logical design layer from the spike POC. Models abstract datacenter topology: what devices should exist, how they connect, and where they sit in a visualization.

**Enums:**
- `LogicalDesignStatus` — draft, active, archived
- `LogicalDeviceRole` — compute, tor, spine, core, pdu, patch_panel, storage, firewall, load_balancer, console_server, cable_manager, adapter
- `LogicalConnectionType` — network, power, console

**Messages:**
- `LogicalDesign` — id, name, version, description, status, created, deleted
- `LogicalDevice` — id, design_id, label, role, device_catalog_id?, requirements?, notes, created, deleted
- `LogicalConnection` — id, design_id, source_device_id, source_port_role, target_device_id, target_port_role, connection_type, requirements?, label, created, deleted
- `LogicalDeviceLayout` — design_id, device_id, position_x, position_y, updated

**LogicalDesignService RPCs:**
- `ListDesigns()` — returns repeated LogicalDesign
- `GetDesign(id)` — returns LogicalDesign
- `CreateDesign(name, description)` — returns LogicalDesign (version=1, status=draft)
- `UpdateDesign(id, name?, description?, status?)` — returns LogicalDesign (auto-increments version)
- `DeleteDesign(id)` — soft delete

**LogicalDeviceService RPCs:**
- `ListDevices(design_id)` — returns repeated LogicalDevice
- `GetDevice(id)` — returns LogicalDevice
- `CreateDevice(design_id, label, role, device_catalog_id?, requirements?, notes)` — returns LogicalDevice
- `UpdateDevice(id, label?, role?, device_catalog_id?, requirements?, notes?)` — returns LogicalDevice
- `DeleteDevice(id)` — soft delete

**LogicalConnectionService RPCs:**
- `ListConnections(design_id)` — returns repeated LogicalConnection
- `CreateConnection(design_id, source_device_id, source_port_role, target_device_id, target_port_role, connection_type, requirements?, label)` — returns LogicalConnection
- `UpdateConnection(id, source_port_role?, target_port_role?, connection_type?, requirements?, label?)` — returns LogicalConnection
- `DeleteConnection(id)` — soft delete

**LogicalDeviceLayoutService RPCs:**
- `GetLayout(design_id)` — returns repeated LogicalDeviceLayout
- `SaveLayout(design_id, repeated LogicalDeviceLayout)` — batch upsert, returns repeated LogicalDeviceLayout
- `DeleteLayout(design_id)` — hard delete all positions (reset to auto-layout)

## Design Decisions

1. **Soft deletes everywhere** except LogicalDeviceLayout (ephemeral view data) and PortDefinition/PortCompatibility (catalog metadata, not lifecycle-tracked).
2. **No pagination** — consistent with existing fundament APIs. Add later across all services.
3. **No tenant scoping in proto** — RLS handles this at the DB layer.
4. **Adapters as explicit logical device nodes** — SFPs/transceivers visible in topology, not hidden in connection metadata.
5. **Recursive placement model** — single table handles server-in-rack and NIC-in-server with check constraint at DB layer.
6. **Cables as first-class assets** — tracked in asset inventory, referenced from physical connections.
7. **Optional logical-physical links** — placement.logical_device_id and connection.logical_connection_id are nullable; reality may diverge from design intent.
