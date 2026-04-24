# DCIM gRPC Specs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the protobuf/gRPC service definitions for the DCIM system in the fundament monorepo.

**Architecture:** 9 proto files under `dcim-api/pkg/proto/v1/` defining services for device catalog, asset inventory, physical infrastructure, placement, connections, notes, and logical design. All follow existing fundament conventions (edition 2023, API_OPAQUE, field numbering at multiples of 10, buf.validate).

**Tech Stack:** Protocol Buffers (edition 2023), buf, Connect (connectrpc.com), protovalidate

**Reference files:**
- Existing fundament proto conventions: `organization-api/pkg/proto/v1/organization.proto`
- Existing buf config: `organization-api/pkg/proto/buf.yaml`, `organization-api/pkg/proto/buf.gen.yaml`
- Design spec: `docs/superpowers/specs/2026-04-24-dcim-grpc-specs-design.md`

---

## File Structure

```
dcim-api/
  pkg/
    proto/
      buf.yaml              — buf module config with protovalidate dep
      buf.gen.yaml           — code gen config for Go + Connect
      v1/
        common.proto         — shared enums (AssetCategory, AssetStatus, etc.) + AssetEvent
        catalog.proto        — DeviceCatalog, PortDefinition, PortCompatibility, CatalogService
        asset.proto          — Asset, AssetStats, AssetService
        site.proto           — Site, Room, RackRow + SiteService, RoomService, RackRowService
        rack.proto           — Rack, RackService
        placement.proto      — Placement, PlacementService
        connection.proto     — PhysicalConnection, PhysicalConnectionService
        note.proto           — Note, NoteService
        design.proto         — LogicalDesign, LogicalDevice, LogicalConnection, LogicalDeviceLayout + services
```

---

### Task 1: buf configuration

**Files:**
- Create: `dcim-api/pkg/proto/buf.yaml`
- Create: `dcim-api/pkg/proto/buf.gen.yaml`

- [ ] **Step 1: Create buf.yaml**

```yaml
version: v2
modules:
  - path: .
deps:
  - buf.build/bufbuild/protovalidate
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

- [ ] **Step 2: Create buf.gen.yaml**

```yaml
version: v2
plugins:
  - local: protoc-gen-go
    out: gen
    opt: paths=source_relative
  - local: protoc-gen-connect-go
    out: gen
    opt: paths=source_relative
```

---

### Task 2: common.proto — shared enums and AssetEvent

**Files:**
- Create: `dcim-api/pkg/proto/v1/common.proto`

- [ ] **Step 1: Create common.proto**

Every other proto file imports this. Contains all shared enums and the AssetEvent message.

```protobuf
edition = "2023";

package dcim.v1;

import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// AssetCategory aligns with dcim.device_catalog category check constraint.
enum AssetCategory {
  ASSET_CATEGORY_UNSPECIFIED    = 0;
  ASSET_CATEGORY_SERVER         = 10;
  ASSET_CATEGORY_SWITCH         = 20;
  ASSET_CATEGORY_PDU            = 30;
  ASSET_CATEGORY_PATCH_PANEL    = 40;
  ASSET_CATEGORY_SFP            = 50;
  ASSET_CATEGORY_NIC            = 60;
  ASSET_CATEGORY_CPU            = 70;
  ASSET_CATEGORY_DIMM           = 80;
  ASSET_CATEGORY_DISK           = 90;
  ASSET_CATEGORY_CABLE          = 100;
  ASSET_CATEGORY_ADAPTER        = 110;
  ASSET_CATEGORY_POWER_SUPPLY   = 120;
  ASSET_CATEGORY_CABLE_MANAGER  = 130;
  ASSET_CATEGORY_CONSOLE_SERVER = 140;
}

// AssetStatus aligns with dcim.asset status check constraint.
enum AssetStatus {
  ASSET_STATUS_UNSPECIFIED    = 0;
  ASSET_STATUS_IN_STOCK       = 10;
  ASSET_STATUS_DEPLOYED       = 20;
  ASSET_STATUS_RMA            = 30;
  ASSET_STATUS_DECOMMISSIONED = 40;
  ASSET_STATUS_IN_TRANSIT     = 50;
  ASSET_STATUS_RESERVED       = 60;
}

// AssetEventType aligns with dcim.asset_event event_type check constraint.
enum AssetEventType {
  ASSET_EVENT_TYPE_UNSPECIFIED    = 0;
  ASSET_EVENT_TYPE_RECEIVED       = 10;
  ASSET_EVENT_TYPE_DEPLOYED       = 20;
  ASSET_EVENT_TYPE_MOVED          = 30;
  ASSET_EVENT_TYPE_RMA_SENT       = 40;
  ASSET_EVENT_TYPE_RMA_RECEIVED   = 50;
  ASSET_EVENT_TYPE_DECOMMISSIONED = 60;
  ASSET_EVENT_TYPE_RESERVED       = 70;
  ASSET_EVENT_TYPE_NOTE           = 80;
}

// RackSlotType aligns with dcim.placement rack_slot_type check constraint.
enum RackSlotType {
  RACK_SLOT_TYPE_UNSPECIFIED = 0;
  RACK_SLOT_TYPE_UNIT        = 10;
  RACK_SLOT_TYPE_POWER       = 20;
  RACK_SLOT_TYPE_ZERO_U      = 30;
}

// PortType aligns with dcim.port_definition port_type check constraint.
enum PortType {
  PORT_TYPE_UNSPECIFIED = 0;
  PORT_TYPE_NETWORK     = 10;
  PORT_TYPE_POWER_IN    = 20;
  PORT_TYPE_POWER_OUT   = 30;
  PORT_TYPE_SLOT        = 40;
  PORT_TYPE_BAY         = 50;
  PORT_TYPE_CONSOLE     = 60;
}

// PortDirection aligns with dcim.port_definition direction check constraint.
enum PortDirection {
  PORT_DIRECTION_UNSPECIFIED = 0;
  PORT_DIRECTION_IN          = 10;
  PORT_DIRECTION_OUT         = 20;
  PORT_DIRECTION_BIDIR       = 30;
}

// NoteEntityType covers all entity types that support notes.
enum NoteEntityType {
  NOTE_ENTITY_TYPE_UNSPECIFIED         = 0;
  NOTE_ENTITY_TYPE_DEVICE_CATALOG      = 10;
  NOTE_ENTITY_TYPE_PORT_DEFINITION     = 20;
  NOTE_ENTITY_TYPE_ASSET               = 30;
  NOTE_ENTITY_TYPE_SITE                = 40;
  NOTE_ENTITY_TYPE_ROOM                = 50;
  NOTE_ENTITY_TYPE_RACK_ROW            = 60;
  NOTE_ENTITY_TYPE_RACK                = 70;
  NOTE_ENTITY_TYPE_PLACEMENT           = 80;
  NOTE_ENTITY_TYPE_PHYSICAL_CONNECTION = 90;
  NOTE_ENTITY_TYPE_LOGICAL_DESIGN      = 100;
  NOTE_ENTITY_TYPE_LOGICAL_DEVICE      = 110;
  NOTE_ENTITY_TYPE_LOGICAL_CONNECTION  = 120;
}

// AssetEvent is an append-only audit record for asset lifecycle changes.
message AssetEvent {
  string                    id           = 10;
  string                    asset_id     = 20;
  AssetEventType            event_type   = 30;
  string                    details      = 40;
  string                    performed_by = 50;
  google.protobuf.Timestamp created      = 60;
}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors

---

### Task 3: catalog.proto — device catalog, ports, compatibility

**Files:**
- Create: `dcim-api/pkg/proto/v1/catalog.proto`

- [ ] **Step 1: Create catalog.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";
import "v1/asset.proto";
import "v1/common.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// DeviceCatalog is a device type definition (dcim.device_catalog).
message DeviceCatalog {
  string              id           = 10;
  string              manufacturer = 20;
  string              model        = 30;
  string              part_number  = 40;
  AssetCategory       category     = 50;
  string              form_factor  = 60;
  // rack_units is absent for sub-components that do not occupy rack units (e.g. cables, DIMMs).
  int32               rack_units   = 70 [features.field_presence = EXPLICIT];
  double              weight_kg    = 80;
  double              power_draw_w = 90;
  map<string, string> specs        = 100;
  google.protobuf.Timestamp created = 110;
  google.protobuf.Timestamp deleted = 120 [features.field_presence = EXPLICIT];
}

// PortDefinition describes a port or slot on a device catalog entry (dcim.port_definition).
message PortDefinition {
  string        id                = 10;
  string        device_catalog_id = 20;
  string        name              = 30;
  PortType      port_type         = 40;
  string        media_type        = 50;
  // speed is absent for non-network ports.
  string        speed             = 60 [features.field_presence = EXPLICIT];
  // max_power_w is present only for power ports.
  double        max_power_w       = 70 [features.field_presence = EXPLICIT];
  PortDirection direction         = 80;
  int32         ordinal           = 90;
}

// PortCompatibility records that a sub-component catalog entry fits a port (dcim.port_compatibility).
message PortCompatibility {
  string port_definition_id    = 10;
  string compatible_catalog_id = 20;
  string notes                 = 30;
}

// ── CatalogService ────────────────────────────────────────────────────────────

service CatalogService {
  // Device catalog entries
  rpc ListCatalog              (ListCatalogRequest)              returns (ListCatalogResponse);
  rpc GetCatalogEntry          (GetCatalogEntryRequest)          returns (GetCatalogEntryResponse);
  rpc CreateCatalogEntry       (CreateCatalogEntryRequest)       returns (CreateCatalogEntryResponse);
  rpc UpdateCatalogEntry       (UpdateCatalogEntryRequest)       returns (UpdateCatalogEntryResponse);
  rpc DeleteCatalogEntry       (DeleteCatalogEntryRequest)       returns (DeleteCatalogEntryResponse);
  rpc ListAssetsByCatalogEntry (ListAssetsByCatalogEntryRequest) returns (ListAssetsByCatalogEntryResponse);
  // Port definitions
  rpc ListPortDefinitions      (ListPortDefinitionsRequest)      returns (ListPortDefinitionsResponse);
  rpc GetPortDefinition        (GetPortDefinitionRequest)        returns (GetPortDefinitionResponse);
  rpc CreatePortDefinition     (CreatePortDefinitionRequest)     returns (CreatePortDefinitionResponse);
  rpc UpdatePortDefinition     (UpdatePortDefinitionRequest)     returns (UpdatePortDefinitionResponse);
  rpc DeletePortDefinition     (DeletePortDefinitionRequest)     returns (DeletePortDefinitionResponse);
  // Port compatibilities
  rpc ListPortCompatibilities  (ListPortCompatibilitiesRequest)  returns (ListPortCompatibilitiesResponse);
  rpc CreatePortCompatibility  (CreatePortCompatibilityRequest)  returns (CreatePortCompatibilityResponse);
  rpc DeletePortCompatibility  (DeletePortCompatibilityRequest)  returns (DeletePortCompatibilityResponse);
}

// ── Device catalog requests/responses ─────────────────────────────────────────

message ListCatalogRequest {
  AssetCategory category_filter = 10 [features.field_presence = EXPLICIT];
  string        search          = 20;
}

message ListCatalogResponse {
  message CatalogSummary {
    DeviceCatalog entry    = 10;
    int32         total    = 20;
    int32         deployed = 30;
    int32         in_stock = 40;
    int32         issues   = 50;
  }

  repeated CatalogSummary entries = 10;
}

message GetCatalogEntryRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetCatalogEntryResponse {
  DeviceCatalog entry = 10;
}

message CreateCatalogEntryRequest {
  string              manufacturer = 10 [(buf.validate.field).string.min_len = 1];
  string              model        = 20 [(buf.validate.field).string.min_len = 1];
  string              part_number  = 30 [(buf.validate.field).string.min_len = 1];
  AssetCategory       category     = 40 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string              form_factor  = 50;
  int32               rack_units   = 60 [features.field_presence = EXPLICIT];
  double              weight_kg    = 70;
  double              power_draw_w = 80;
  map<string, string> specs        = 90;
}

message CreateCatalogEntryResponse {
  DeviceCatalog entry = 10;
}

message UpdateCatalogEntryRequest {
  string              id           = 10 [(buf.validate.field).string.min_len = 1];
  string              manufacturer = 20 [features.field_presence = EXPLICIT];
  string              model        = 30 [features.field_presence = EXPLICIT];
  string              part_number  = 40 [features.field_presence = EXPLICIT];
  string              form_factor  = 50 [features.field_presence = EXPLICIT];
  int32               rack_units   = 60 [features.field_presence = EXPLICIT];
  double              weight_kg    = 70 [features.field_presence = EXPLICIT];
  double              power_draw_w = 80 [features.field_presence = EXPLICIT];
  map<string, string> specs        = 90;
}

message UpdateCatalogEntryResponse {
  DeviceCatalog entry = 10;
}

message DeleteCatalogEntryRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteCatalogEntryResponse {}

message ListAssetsByCatalogEntryRequest {
  string device_catalog_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListAssetsByCatalogEntryResponse {
  repeated Asset assets = 10;
}

// ── Port definition requests/responses ────────────────────────────────────────

message ListPortDefinitionsRequest {
  string device_catalog_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListPortDefinitionsResponse {
  repeated PortDefinition port_definitions = 10;
}

message GetPortDefinitionRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetPortDefinitionResponse {
  PortDefinition port_definition = 10;
}

message CreatePortDefinitionRequest {
  string        device_catalog_id = 10 [(buf.validate.field).string.min_len = 1];
  string        name              = 20 [(buf.validate.field).string.min_len = 1];
  PortType      port_type         = 30 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string        media_type        = 40;
  string        speed             = 50 [features.field_presence = EXPLICIT];
  double        max_power_w       = 60 [features.field_presence = EXPLICIT];
  PortDirection direction         = 70 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  int32         ordinal           = 80;
}

message CreatePortDefinitionResponse {
  PortDefinition port_definition = 10;
}

message UpdatePortDefinitionRequest {
  string        id          = 10 [(buf.validate.field).string.min_len = 1];
  string        name        = 20 [features.field_presence = EXPLICIT];
  PortType      port_type   = 30 [features.field_presence = EXPLICIT];
  string        media_type  = 40 [features.field_presence = EXPLICIT];
  string        speed       = 50 [features.field_presence = EXPLICIT];
  double        max_power_w = 60 [features.field_presence = EXPLICIT];
  PortDirection direction   = 70 [features.field_presence = EXPLICIT];
  int32         ordinal     = 80 [features.field_presence = EXPLICIT];
}

message UpdatePortDefinitionResponse {
  PortDefinition port_definition = 10;
}

message DeletePortDefinitionRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeletePortDefinitionResponse {}

// ── Port compatibility requests/responses ─────────────────────────────────────

message ListPortCompatibilitiesRequest {
  string port_definition_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListPortCompatibilitiesResponse {
  repeated PortCompatibility compatibilities = 10;
}

message CreatePortCompatibilityRequest {
  string port_definition_id    = 10 [(buf.validate.field).string.min_len = 1];
  string compatible_catalog_id = 20 [(buf.validate.field).string.min_len = 1];
  string notes                 = 30;
}

message CreatePortCompatibilityResponse {
  PortCompatibility compatibility = 10;
}

message DeletePortCompatibilityRequest {
  string port_definition_id    = 10 [(buf.validate.field).string.min_len = 1];
  string compatible_catalog_id = 20 [(buf.validate.field).string.min_len = 1];
}

message DeletePortCompatibilityResponse {}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 4: asset.proto — inventory tracking

**Files:**
- Create: `dcim-api/pkg/proto/v1/asset.proto`

- [ ] **Step 1: Create asset.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";
import "v1/common.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// Asset is a physical inventory item (dcim.asset).
// Physical location is tracked via Placement, not stored on the asset itself.
message Asset {
  string                    id                = 10;
  string                    device_catalog_id = 20;
  AssetStatus               status            = 30;
  // serial_number is absent for asset types that do not carry one (e.g. cables).
  string                    serial_number     = 40 [features.field_presence = EXPLICIT];
  string                    asset_tag         = 50 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp purchase_date     = 60 [features.field_presence = EXPLICIT];
  string                    purchase_order    = 70 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp warranty_expiry   = 80 [features.field_presence = EXPLICIT];
  string                    notes             = 90;
  google.protobuf.Timestamp created           = 100;
  google.protobuf.Timestamp deleted           = 110 [features.field_presence = EXPLICIT];
}

message AssetStats {
  int32 total          = 10;
  int32 in_stock       = 20;
  int32 deployed       = 30;
  int32 available      = 40;
  int32 rma            = 50;
  int32 decommissioned = 60;
}

enum AssetSortField {
  ASSET_SORT_FIELD_UNSPECIFIED     = 0;
  ASSET_SORT_FIELD_STATUS          = 10;
  ASSET_SORT_FIELD_SERIAL_NUMBER   = 20;
  ASSET_SORT_FIELD_ASSET_TAG       = 30;
  ASSET_SORT_FIELD_PURCHASE_DATE   = 40;
  ASSET_SORT_FIELD_WARRANTY_EXPIRY = 50;
}

enum SortDirection {
  SORT_DIRECTION_UNSPECIFIED = 0;
  SORT_DIRECTION_ASC         = 10;
  SORT_DIRECTION_DESC        = 20;
}

service AssetService {
  rpc ListAssets    (ListAssetsRequest)     returns (ListAssetsResponse);
  rpc GetAsset      (GetAssetRequest)       returns (GetAssetResponse);
  rpc CreateAsset   (CreateAssetRequest)    returns (CreateAssetResponse);
  rpc UpdateAsset   (UpdateAssetRequest)    returns (UpdateAssetResponse);
  rpc DeleteAsset   (DeleteAssetRequest)    returns (DeleteAssetResponse);
  rpc GetAssetEvents(GetAssetEventsRequest) returns (GetAssetEventsResponse);
  rpc GetAssetStats (GetAssetStatsRequest)  returns (GetAssetStatsResponse);
}

message ListAssetsRequest {
  AssetStatus    status_filter     = 10 [features.field_presence = EXPLICIT];
  AssetCategory  category_filter   = 20 [features.field_presence = EXPLICIT];
  string         device_catalog_id = 30 [features.field_presence = EXPLICIT];
  string         search            = 40;
  AssetSortField sort_by           = 50;
  SortDirection  sort_direction    = 60;
  bool           include_deleted   = 70;
}

message ListAssetsResponse {
  repeated Asset assets = 10;
}

message GetAssetRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetAssetResponse {
  Asset asset = 10;
}

message CreateAssetRequest {
  string                    device_catalog_id = 10 [(buf.validate.field).string.min_len = 1];
  AssetStatus               status            = 20 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string                    serial_number     = 30 [features.field_presence = EXPLICIT];
  string                    asset_tag         = 40 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp purchase_date     = 50 [features.field_presence = EXPLICIT];
  string                    purchase_order    = 60 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp warranty_expiry   = 70 [features.field_presence = EXPLICIT];
  string                    notes             = 80;
}

message CreateAssetResponse {
  Asset asset = 10;
}

message UpdateAssetRequest {
  string                    id              = 10 [(buf.validate.field).string.min_len = 1];
  AssetStatus               status          = 20 [features.field_presence = EXPLICIT];
  string                    serial_number   = 30 [features.field_presence = EXPLICIT];
  string                    asset_tag       = 40 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp warranty_expiry = 50 [features.field_presence = EXPLICIT];
  string                    notes           = 60 [features.field_presence = EXPLICIT];
}

message UpdateAssetResponse {
  Asset asset = 10;
}

message DeleteAssetRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteAssetResponse {}

message GetAssetEventsRequest {
  string asset_id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetAssetEventsResponse {
  repeated AssetEvent events = 10;
}

message GetAssetStatsRequest {
  // Optionally scope counts to assets with a placement in a specific site.
  string site_id = 10 [features.field_presence = EXPLICIT];
}

message GetAssetStatsResponse {
  AssetStats stats = 10;
}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 5: site.proto — physical hierarchy (Site, Room, RackRow)

**Files:**
- Create: `dcim-api/pkg/proto/v1/site.proto`

- [ ] **Step 1: Create site.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// Site is a physical data center location (dcim.site).
message Site {
  string                    id      = 10;
  string                    name    = 20;
  string                    address = 30;
  google.protobuf.Timestamp created = 40;
  google.protobuf.Timestamp deleted = 50 [features.field_presence = EXPLICIT];
}

// Room is a hall or room within a site (dcim.room).
message Room {
  string                    id      = 10;
  string                    site_id = 20;
  string                    name    = 30;
  string                    floor   = 40;
  google.protobuf.Timestamp created = 50;
  google.protobuf.Timestamp deleted = 60 [features.field_presence = EXPLICIT];
}

// RackRow is a row of racks within a room (dcim.rack_row).
// position_x and position_y enable cable length estimation between racks.
message RackRow {
  string                    id         = 10;
  string                    room_id    = 20;
  string                    name       = 30;
  double                    position_x = 40 [features.field_presence = EXPLICIT];
  double                    position_y = 50 [features.field_presence = EXPLICIT];
  google.protobuf.Timestamp created    = 60;
  google.protobuf.Timestamp deleted    = 70 [features.field_presence = EXPLICIT];
}

// ── SiteService ───────────────────────────────────────────────────────────────

service SiteService {
  rpc ListSites (ListSitesRequest)  returns (ListSitesResponse);
  rpc GetSite   (GetSiteRequest)    returns (GetSiteResponse);
  rpc CreateSite(CreateSiteRequest) returns (CreateSiteResponse);
  rpc UpdateSite(UpdateSiteRequest) returns (UpdateSiteResponse);
  rpc DeleteSite(DeleteSiteRequest) returns (DeleteSiteResponse);
}

message ListSitesRequest {}

message ListSitesResponse {
  repeated Site sites = 10;
}

message GetSiteRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetSiteResponse {
  Site site = 10;
}

message CreateSiteRequest {
  string name    = 10 [(buf.validate.field).string.min_len = 1];
  string address = 20;
}

message CreateSiteResponse {
  Site site = 10;
}

message UpdateSiteRequest {
  string id      = 10 [(buf.validate.field).string.min_len = 1];
  string name    = 20 [features.field_presence = EXPLICIT];
  string address = 30 [features.field_presence = EXPLICIT];
}

message UpdateSiteResponse {
  Site site = 10;
}

message DeleteSiteRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteSiteResponse {}

// ── RoomService ───────────────────────────────────────────────────────────────

service RoomService {
  rpc ListRooms (ListRoomsRequest)  returns (ListRoomsResponse);
  rpc GetRoom   (GetRoomRequest)    returns (GetRoomResponse);
  rpc CreateRoom(CreateRoomRequest) returns (CreateRoomResponse);
  rpc UpdateRoom(UpdateRoomRequest) returns (UpdateRoomResponse);
  rpc DeleteRoom(DeleteRoomRequest) returns (DeleteRoomResponse);
}

message ListRoomsRequest {
  string site_id = 10 [features.field_presence = EXPLICIT];
}

message ListRoomsResponse {
  repeated Room rooms = 10;
}

message GetRoomRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetRoomResponse {
  Room room = 10;
}

message CreateRoomRequest {
  string site_id = 10 [(buf.validate.field).string.min_len = 1];
  string name    = 20 [(buf.validate.field).string.min_len = 1];
  string floor   = 30;
}

message CreateRoomResponse {
  Room room = 10;
}

message UpdateRoomRequest {
  string id    = 10 [(buf.validate.field).string.min_len = 1];
  string name  = 20 [features.field_presence = EXPLICIT];
  string floor = 30 [features.field_presence = EXPLICIT];
}

message UpdateRoomResponse {
  Room room = 10;
}

message DeleteRoomRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteRoomResponse {}

// ── RackRowService ────────────────────────────────────────────────────────────

service RackRowService {
  rpc ListRackRows (ListRackRowsRequest)  returns (ListRackRowsResponse);
  rpc GetRackRow   (GetRackRowRequest)    returns (GetRackRowResponse);
  rpc CreateRackRow(CreateRackRowRequest) returns (CreateRackRowResponse);
  rpc UpdateRackRow(UpdateRackRowRequest) returns (UpdateRackRowResponse);
  rpc DeleteRackRow(DeleteRackRowRequest) returns (DeleteRackRowResponse);
}

message ListRackRowsRequest {
  string room_id = 10 [features.field_presence = EXPLICIT];
}

message ListRackRowsResponse {
  repeated RackRow rack_rows = 10;
}

message GetRackRowRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetRackRowResponse {
  RackRow rack_row = 10;
}

message CreateRackRowRequest {
  string room_id    = 10 [(buf.validate.field).string.min_len = 1];
  string name       = 20 [(buf.validate.field).string.min_len = 1];
  double position_x = 30 [features.field_presence = EXPLICIT];
  double position_y = 40 [features.field_presence = EXPLICIT];
}

message CreateRackRowResponse {
  RackRow rack_row = 10;
}

message UpdateRackRowRequest {
  string id         = 10 [(buf.validate.field).string.min_len = 1];
  string name       = 20 [features.field_presence = EXPLICIT];
  double position_x = 30 [features.field_presence = EXPLICIT];
  double position_y = 40 [features.field_presence = EXPLICIT];
}

message UpdateRackRowResponse {
  RackRow rack_row = 10;
}

message DeleteRackRowRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteRackRowResponse {}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 6: rack.proto — equipment racks

**Files:**
- Create: `dcim-api/pkg/proto/v1/rack.proto`

- [ ] **Step 1: Create rack.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// Rack is a physical equipment rack within a rack row (dcim.rack).
message Rack {
  string                    id              = 10;
  string                    row_id          = 20;
  string                    name            = 30;
  int32                     total_units     = 40;
  int32                     position_in_row = 50;
  google.protobuf.Timestamp created         = 60;
  google.protobuf.Timestamp deleted         = 70 [features.field_presence = EXPLICIT];
}

service RackService {
  rpc ListRacks (ListRacksRequest)  returns (ListRacksResponse);
  rpc GetRack   (GetRackRequest)    returns (GetRackResponse);
  rpc CreateRack(CreateRackRequest) returns (CreateRackResponse);
  rpc UpdateRack(UpdateRackRequest) returns (UpdateRackResponse);
  rpc DeleteRack(DeleteRackRequest) returns (DeleteRackResponse);
}

message ListRacksRequest {
  // Filter by row; omit to list all racks.
  string row_id = 10 [features.field_presence = EXPLICIT];
}

message ListRacksResponse {
  message RackSummary {
    Rack   rack            = 10;
    int32  used_units      = 20;
    int32  free_units      = 30;
    double power_draw_w    = 40;
    int32  device_count    = 50;
    double utilization_pct = 60;
  }

  repeated RackSummary racks = 10;
}

message GetRackRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetRackResponse {
  Rack rack = 10;
}

message CreateRackRequest {
  string row_id          = 10 [(buf.validate.field).string.min_len = 1];
  string name            = 20 [(buf.validate.field).string.min_len = 1];
  int32  total_units     = 30 [(buf.validate.field).int32.gt = 0];
  int32  position_in_row = 40;
}

message CreateRackResponse {
  Rack rack = 10;
}

message UpdateRackRequest {
  string id              = 10 [(buf.validate.field).string.min_len = 1];
  string name            = 20 [features.field_presence = EXPLICIT];
  int32  total_units     = 30 [features.field_presence = EXPLICIT];
  int32  position_in_row = 40 [features.field_presence = EXPLICIT];
}

message UpdateRackResponse {
  Rack rack = 10;
}

message DeleteRackRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteRackResponse {}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 7: placement.proto — asset-to-location mapping

**Files:**
- Create: `dcim-api/pkg/proto/v1/placement.proto`

- [ ] **Step 1: Create placement.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";
import "v1/common.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// Placement maps an asset to a physical location (dcim.placement).
// Exactly one of (rack_id + rack_unit_start + rack_slot_type) or
// (parent_placement_id + parent_port_name) must be set.
message Placement {
  string                    id                  = 10;
  string                    asset_id            = 20;
  // Top-level rack placement fields.
  string                    rack_id             = 30 [features.field_presence = EXPLICIT];
  int32                     rack_unit_start     = 40 [features.field_presence = EXPLICIT];
  RackSlotType              rack_slot_type      = 50 [features.field_presence = EXPLICIT];
  // Sub-component placement fields.
  string                    parent_placement_id = 60 [features.field_presence = EXPLICIT];
  string                    parent_port_name    = 70 [features.field_presence = EXPLICIT];
  // Optional link to the logical design.
  string                    logical_device_id   = 80 [features.field_presence = EXPLICIT];
  string                    notes               = 90;
  google.protobuf.Timestamp created             = 100;
  google.protobuf.Timestamp deleted             = 110 [features.field_presence = EXPLICIT];
}

service PlacementService {
  rpc CreatePlacement     (CreatePlacementRequest)      returns (CreatePlacementResponse);
  rpc GetPlacement        (GetPlacementRequest)         returns (GetPlacementResponse);
  rpc UpdatePlacement     (UpdatePlacementRequest)      returns (UpdatePlacementResponse);
  rpc DeletePlacement     (DeletePlacementRequest)      returns (DeletePlacementResponse);
  rpc ListPlacementsByRack(ListPlacementsByRackRequest) returns (ListPlacementsByRackResponse);
  rpc ListChildPlacements (ListChildPlacementsRequest)  returns (ListChildPlacementsResponse);
}

message CreatePlacementRequest {
  string       asset_id            = 10 [(buf.validate.field).string.min_len = 1];
  string       rack_id             = 20 [features.field_presence = EXPLICIT];
  int32        rack_unit_start     = 30 [features.field_presence = EXPLICIT];
  RackSlotType rack_slot_type      = 40 [features.field_presence = EXPLICIT];
  string       parent_placement_id = 50 [features.field_presence = EXPLICIT];
  string       parent_port_name    = 60 [features.field_presence = EXPLICIT];
  string       logical_device_id   = 70 [features.field_presence = EXPLICIT];
  string       notes               = 80;
}

message CreatePlacementResponse {
  Placement placement = 10;
}

message GetPlacementRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetPlacementResponse {
  Placement placement = 10;
}

message UpdatePlacementRequest {
  string       id                  = 10 [(buf.validate.field).string.min_len = 1];
  string       rack_id             = 20 [features.field_presence = EXPLICIT];
  int32        rack_unit_start     = 30 [features.field_presence = EXPLICIT];
  RackSlotType rack_slot_type      = 40 [features.field_presence = EXPLICIT];
  string       parent_placement_id = 50 [features.field_presence = EXPLICIT];
  string       parent_port_name    = 60 [features.field_presence = EXPLICIT];
  string       logical_device_id   = 70 [features.field_presence = EXPLICIT];
  string       notes               = 80 [features.field_presence = EXPLICIT];
}

message UpdatePlacementResponse {
  Placement placement = 10;
}

message DeletePlacementRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeletePlacementResponse {}

message ListPlacementsByRackRequest {
  string rack_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListPlacementsByRackResponse {
  repeated Placement placements = 10;
}

message ListChildPlacementsRequest {
  string parent_placement_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListChildPlacementsResponse {
  repeated Placement placements = 10;
}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 8: connection.proto — physical cable connections

**Files:**
- Create: `dcim-api/pkg/proto/v1/connection.proto`

- [ ] **Step 1: Create connection.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// PhysicalConnection records a cable between two placement ports (dcim.physical_connection).
message PhysicalConnection {
  string                    id                    = 10;
  string                    source_placement_id   = 20;
  string                    source_port_name      = 30;
  string                    target_placement_id   = 40;
  string                    target_port_name      = 50;
  // cable_asset_id is absent when the cable itself is not individually tracked.
  string                    cable_asset_id        = 60 [features.field_presence = EXPLICIT];
  // logical_connection_id links this physical cable to the intended logical design.
  string                    logical_connection_id = 70 [features.field_presence = EXPLICIT];
  string                    notes                 = 80;
  google.protobuf.Timestamp created               = 90;
  google.protobuf.Timestamp deleted               = 100 [features.field_presence = EXPLICIT];
}

service PhysicalConnectionService {
  rpc CreatePhysicalConnection  (CreatePhysicalConnectionRequest)  returns (CreatePhysicalConnectionResponse);
  rpc GetPhysicalConnection     (GetPhysicalConnectionRequest)     returns (GetPhysicalConnectionResponse);
  rpc UpdatePhysicalConnection  (UpdatePhysicalConnectionRequest)  returns (UpdatePhysicalConnectionResponse);
  rpc DeletePhysicalConnection  (DeletePhysicalConnectionRequest)  returns (DeletePhysicalConnectionResponse);
  rpc ListConnectionsByPlacement(ListConnectionsByPlacementRequest) returns (ListConnectionsByPlacementResponse);
}

message CreatePhysicalConnectionRequest {
  string source_placement_id   = 10 [(buf.validate.field).string.min_len = 1];
  string source_port_name      = 20 [(buf.validate.field).string.min_len = 1];
  string target_placement_id   = 30 [(buf.validate.field).string.min_len = 1];
  string target_port_name      = 40 [(buf.validate.field).string.min_len = 1];
  string cable_asset_id        = 50 [features.field_presence = EXPLICIT];
  string logical_connection_id = 60 [features.field_presence = EXPLICIT];
  string notes                 = 70;
}

message CreatePhysicalConnectionResponse {
  PhysicalConnection connection = 10;
}

message GetPhysicalConnectionRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetPhysicalConnectionResponse {
  PhysicalConnection connection = 10;
}

message UpdatePhysicalConnectionRequest {
  string id                    = 10 [(buf.validate.field).string.min_len = 1];
  string cable_asset_id        = 20 [features.field_presence = EXPLICIT];
  string logical_connection_id = 30 [features.field_presence = EXPLICIT];
  string notes                 = 40 [features.field_presence = EXPLICIT];
}

message UpdatePhysicalConnectionResponse {
  PhysicalConnection connection = 10;
}

message DeletePhysicalConnectionRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeletePhysicalConnectionResponse {}

message ListConnectionsByPlacementRequest {
  string placement_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListConnectionsByPlacementResponse {
  repeated PhysicalConnection connections = 10;
}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 9: note.proto — polymorphic comments

**Files:**
- Create: `dcim-api/pkg/proto/v1/note.proto`

- [ ] **Step 1: Create note.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";
import "v1/common.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// Note is a polymorphic comment attached to any DCIM entity (dcim.note).
// There is no FK on entity_id — integrity is enforced at the application layer.
message Note {
  string                    id          = 10;
  NoteEntityType            entity_type = 20;
  string                    entity_id   = 30;
  string                    body        = 40;
  string                    created_by  = 50;
  google.protobuf.Timestamp created     = 60;
  google.protobuf.Timestamp deleted     = 70 [features.field_presence = EXPLICIT];
}

service NoteService {
  rpc ListNotes (ListNotesRequest)  returns (ListNotesResponse);
  rpc CreateNote(CreateNoteRequest) returns (CreateNoteResponse);
  rpc DeleteNote(DeleteNoteRequest) returns (DeleteNoteResponse);
}

message ListNotesRequest {
  NoteEntityType entity_type = 10 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string         entity_id   = 20 [(buf.validate.field).string.min_len = 1];
}

message ListNotesResponse {
  repeated Note notes = 10;
}

message CreateNoteRequest {
  NoteEntityType entity_type = 10 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string         entity_id   = 20 [(buf.validate.field).string.min_len = 1];
  string         body        = 30 [(buf.validate.field).string.min_len = 1];
  string         created_by  = 40 [(buf.validate.field).string.min_len = 1];
}

message CreateNoteResponse {
  Note note = 10;
}

// DeleteNote soft-deletes the note by setting its deleted timestamp.
message DeleteNoteRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteNoteResponse {}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 10: design.proto — logical design layer

**Files:**
- Create: `dcim-api/pkg/proto/v1/design.proto`

- [ ] **Step 1: Create design.proto**

```protobuf
edition = "2023";

package dcim.v1;

import "buf/validate/validate.proto";
import "google/protobuf/go_features.proto";
import "google/protobuf/timestamp.proto";

option features.(pb.go).api_level = API_OPAQUE;
option features.field_presence = IMPLICIT;
option go_package = "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1;dcimv1";

// ── Enums ─────────────────────────────────────────────────────────────────────

// LogicalDesignStatus aligns with dcim.logical_design status check constraint.
enum LogicalDesignStatus {
  LOGICAL_DESIGN_STATUS_UNSPECIFIED = 0;
  LOGICAL_DESIGN_STATUS_DRAFT       = 10;
  LOGICAL_DESIGN_STATUS_ACTIVE      = 20;
  LOGICAL_DESIGN_STATUS_ARCHIVED    = 30;
}

// LogicalDeviceRole aligns with dcim.logical_device role check constraint.
enum LogicalDeviceRole {
  LOGICAL_DEVICE_ROLE_UNSPECIFIED    = 0;
  LOGICAL_DEVICE_ROLE_COMPUTE        = 10;
  LOGICAL_DEVICE_ROLE_TOR            = 20;
  LOGICAL_DEVICE_ROLE_SPINE          = 30;
  LOGICAL_DEVICE_ROLE_CORE           = 40;
  LOGICAL_DEVICE_ROLE_PDU            = 50;
  LOGICAL_DEVICE_ROLE_PATCH_PANEL    = 60;
  LOGICAL_DEVICE_ROLE_STORAGE        = 70;
  LOGICAL_DEVICE_ROLE_FIREWALL       = 80;
  LOGICAL_DEVICE_ROLE_LOAD_BALANCER  = 90;
  LOGICAL_DEVICE_ROLE_CONSOLE_SERVER = 100;
  LOGICAL_DEVICE_ROLE_CABLE_MANAGER  = 110;
  LOGICAL_DEVICE_ROLE_ADAPTER        = 120;
}

// LogicalConnectionType aligns with dcim.logical_connection connection_type check constraint.
enum LogicalConnectionType {
  LOGICAL_CONNECTION_TYPE_UNSPECIFIED = 0;
  LOGICAL_CONNECTION_TYPE_NETWORK     = 10;
  LOGICAL_CONNECTION_TYPE_POWER       = 20;
  LOGICAL_CONNECTION_TYPE_CONSOLE     = 30;
}

// ── Messages ──────────────────────────────────────────────────────────────────

// LogicalDesign is a versioned topology schema (dcim.logical_design).
// Only one design per name may have status ACTIVE at a time.
message LogicalDesign {
  string                    id          = 10;
  string                    name        = 20;
  int32                     version     = 30;
  string                    description = 40;
  LogicalDesignStatus       status      = 50;
  google.protobuf.Timestamp created     = 60;
  google.protobuf.Timestamp deleted     = 70 [features.field_presence = EXPLICIT];
}

// LogicalDevice is an abstract role within a design (dcim.logical_device).
// When device_catalog_id is absent, the device is generic and requirements
// describes what kind of device is needed (e.g. {"cpu_cores": "32", "ram_gb": "256"}).
message LogicalDevice {
  string                    id                = 10;
  string                    design_id         = 20;
  string                    label             = 30;
  LogicalDeviceRole         role              = 40;
  string                    device_catalog_id = 50 [features.field_presence = EXPLICIT];
  string                    requirements      = 60 [features.field_presence = EXPLICIT];
  string                    notes             = 70;
  google.protobuf.Timestamp created           = 80;
  google.protobuf.Timestamp deleted           = 90 [features.field_presence = EXPLICIT];
}

// LogicalConnection is an intended link between two logical devices (dcim.logical_connection).
message LogicalConnection {
  string                    id               = 10;
  string                    design_id        = 20;
  string                    source_device_id = 30;
  string                    source_port_role = 40;
  string                    target_device_id = 50;
  string                    target_port_role = 60;
  LogicalConnectionType     connection_type  = 70;
  string                    requirements     = 80 [features.field_presence = EXPLICIT];
  string                    label            = 90;
  google.protobuf.Timestamp created          = 100;
  google.protobuf.Timestamp deleted          = 110 [features.field_presence = EXPLICIT];
}

// LogicalDeviceLayout stores position data for topology visualization.
// Ephemeral view data — no soft delete.
message LogicalDeviceLayout {
  string                    design_id  = 10;
  string                    device_id  = 20;
  double                    position_x = 30;
  double                    position_y = 40;
  google.protobuf.Timestamp updated    = 50;
}

// ── LogicalDesignService ──────────────────────────────────────────────────────

service LogicalDesignService {
  rpc ListDesigns (ListDesignsRequest)  returns (ListDesignsResponse);
  rpc GetDesign   (GetDesignRequest)    returns (GetDesignResponse);
  rpc CreateDesign(CreateDesignRequest) returns (CreateDesignResponse);
  rpc UpdateDesign(UpdateDesignRequest) returns (UpdateDesignResponse);
  rpc DeleteDesign(DeleteDesignRequest) returns (DeleteDesignResponse);
}

message ListDesignsRequest {}

message ListDesignsResponse {
  repeated LogicalDesign designs = 10;
}

message GetDesignRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetDesignResponse {
  LogicalDesign design = 10;
}

message CreateDesignRequest {
  string name        = 10 [(buf.validate.field).string.min_len = 1];
  string description = 20;
}

message CreateDesignResponse {
  LogicalDesign design = 10;
}

message UpdateDesignRequest {
  string              id          = 10 [(buf.validate.field).string.min_len = 1];
  string              name        = 20 [features.field_presence = EXPLICIT];
  string              description = 30 [features.field_presence = EXPLICIT];
  LogicalDesignStatus status      = 40 [features.field_presence = EXPLICIT];
}

message UpdateDesignResponse {
  LogicalDesign design = 10;
}

message DeleteDesignRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteDesignResponse {}

// ── LogicalDeviceService ──────────────────────────────────────────────────────

service LogicalDeviceService {
  rpc ListDevices (ListDevicesRequest)  returns (ListDevicesResponse);
  rpc GetDevice   (GetDeviceRequest)    returns (GetDeviceResponse);
  rpc CreateDevice(CreateDeviceRequest) returns (CreateDeviceResponse);
  rpc UpdateDevice(UpdateDeviceRequest) returns (UpdateDeviceResponse);
  rpc DeleteDevice(DeleteDeviceRequest) returns (DeleteDeviceResponse);
}

message ListDevicesRequest {
  string design_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListDevicesResponse {
  repeated LogicalDevice devices = 10;
}

message GetDeviceRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetDeviceResponse {
  LogicalDevice device = 10;
}

message CreateDeviceRequest {
  string            design_id         = 10 [(buf.validate.field).string.min_len = 1];
  string            label             = 20 [(buf.validate.field).string.min_len = 1];
  LogicalDeviceRole role              = 30 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string            device_catalog_id = 40 [features.field_presence = EXPLICIT];
  string            requirements      = 50 [features.field_presence = EXPLICIT];
  string            notes             = 60;
}

message CreateDeviceResponse {
  LogicalDevice device = 10;
}

message UpdateDeviceRequest {
  string            id                = 10 [(buf.validate.field).string.min_len = 1];
  string            label             = 20 [features.field_presence = EXPLICIT];
  LogicalDeviceRole role              = 30 [features.field_presence = EXPLICIT];
  string            device_catalog_id = 40 [features.field_presence = EXPLICIT];
  string            requirements      = 50 [features.field_presence = EXPLICIT];
  string            notes             = 60 [features.field_presence = EXPLICIT];
}

message UpdateDeviceResponse {
  LogicalDevice device = 10;
}

message DeleteDeviceRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteDeviceResponse {}

// ── LogicalConnectionService ──────────────────────────────────────────────────

service LogicalConnectionService {
  rpc ListConnections (ListConnectionsRequest)  returns (ListConnectionsResponse);
  rpc CreateConnection(CreateConnectionRequest) returns (CreateConnectionResponse);
  rpc UpdateConnection(UpdateConnectionRequest) returns (UpdateConnectionResponse);
  rpc DeleteConnection(DeleteConnectionRequest) returns (DeleteConnectionResponse);
}

message ListConnectionsRequest {
  string design_id = 10 [(buf.validate.field).string.min_len = 1];
}

message ListConnectionsResponse {
  repeated LogicalConnection connections = 10;
}

message CreateConnectionRequest {
  string                design_id        = 10 [(buf.validate.field).string.min_len = 1];
  string                source_device_id = 20 [(buf.validate.field).string.min_len = 1];
  string                source_port_role = 30 [(buf.validate.field).string.min_len = 1];
  string                target_device_id = 40 [(buf.validate.field).string.min_len = 1];
  string                target_port_role = 50 [(buf.validate.field).string.min_len = 1];
  LogicalConnectionType connection_type  = 60 [(buf.validate.field).enum = {defined_only: true, not_in: [0]}];
  string                requirements     = 70 [features.field_presence = EXPLICIT];
  string                label            = 80;
}

message CreateConnectionResponse {
  LogicalConnection connection = 10;
}

message UpdateConnectionRequest {
  string                id               = 10 [(buf.validate.field).string.min_len = 1];
  string                source_port_role = 20 [features.field_presence = EXPLICIT];
  string                target_port_role = 30 [features.field_presence = EXPLICIT];
  LogicalConnectionType connection_type  = 40 [features.field_presence = EXPLICIT];
  string                requirements     = 50 [features.field_presence = EXPLICIT];
  string                label            = 60 [features.field_presence = EXPLICIT];
}

message UpdateConnectionResponse {
  LogicalConnection connection = 10;
}

message DeleteConnectionRequest {
  string id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteConnectionResponse {}

// ── LogicalDeviceLayoutService ────────────────────────────────────────────────

service LogicalDeviceLayoutService {
  rpc GetLayout   (GetLayoutRequest)    returns (GetLayoutResponse);
  rpc SaveLayout  (SaveLayoutRequest)   returns (SaveLayoutResponse);
  rpc DeleteLayout(DeleteLayoutRequest) returns (DeleteLayoutResponse);
}

message GetLayoutRequest {
  string design_id = 10 [(buf.validate.field).string.min_len = 1];
}

message GetLayoutResponse {
  repeated LogicalDeviceLayout positions = 10;
}

message SaveLayoutRequest {
  string design_id = 10 [(buf.validate.field).string.min_len = 1];

  message DevicePosition {
    string device_id  = 10 [(buf.validate.field).string.min_len = 1];
    double position_x = 20;
    double position_y = 30;
  }

  repeated DevicePosition positions = 20;
}

message SaveLayoutResponse {
  repeated LogicalDeviceLayout positions = 10;
}

// DeleteLayout removes all stored positions for a design (hard delete — resets to auto-layout).
message DeleteLayoutRequest {
  string design_id = 10 [(buf.validate.field).string.min_len = 1];
}

message DeleteLayoutResponse {}
```

- [ ] **Step 2: Verify buf lint passes**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors


---

### Task 11: Run buf generate and verify

**Files:**
- Generated: `dcim-api/pkg/proto/gen/v1/*.go` (output of buf generate)

- [ ] **Step 1: Install buf dependencies**

Run: `cd dcim-api/pkg/proto && buf dep update`
Expected: `buf.lock` created/updated with protovalidate dependency

- [ ] **Step 2: Run buf lint on all protos**

Run: `cd dcim-api/pkg/proto && buf lint`
Expected: no errors

- [ ] **Step 3: Run buf generate**

Run: `cd dcim-api/pkg/proto && buf generate`
Expected: Go and Connect files generated in `gen/v1/`

- [ ] **Step 4: Verify generated files exist**

Run: `ls dcim-api/pkg/proto/gen/v1/`
Expected: `.pb.go` and `connect.go` files for each proto:
- `common.pb.go`
- `catalog.pb.go`, `catalog.connect.go`
- `asset.pb.go`, `asset.connect.go`
- `site.pb.go`, `site.connect.go`
- `rack.pb.go`, `rack.connect.go`
- `placement.pb.go`, `placement.connect.go`
- `connection.pb.go`, `connection.connect.go`
- `note.pb.go`, `note.connect.go`
- `design.pb.go`, `design.connect.go`

- [ ] **Step 5: Commit everything**

```bash
git add dcim-api/
git commit -m "feat(dcim): add gRPC proto specs for DCIM system

Defines protobuf services for device catalog, asset inventory,
physical infrastructure (sites/rooms/rows/racks), placement,
physical connections, polymorphic notes, and logical design.
Includes generated Go and Connect code."
```
