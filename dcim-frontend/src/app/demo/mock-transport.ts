// Demo-only in-memory ConnectRPC transport: every RPC is answered from the
// fixtures instead of the network, so the whole app can be walked through
// without a backend. Never imported by the production entrypoint (main.ts).
//
// The fixtures are copied into a store here rather than read straight through,
// so what you create or change during a session stays changed until you reload.
import { create, isFieldSet } from '@bufbuild/protobuf';
import { EmptySchema, timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Transport, createRouterTransport } from '@connectrpc/connect';
import {
  AssetService,
  AssetSchema,
  AssetSortField,
  ListAssetsResponseSchema,
  GetAssetResponseSchema,
  CreateAssetResponseSchema,
  GetAssetEventsResponseSchema,
  GetAssetLocationResponseSchema,
  GetAssetStatsResponseSchema,
} from '../../generated/v1/asset_pb';
import {
  CatalogService,
  DeviceCatalogSchema,
  ListCatalogResponseSchema,
  GetCatalogEntryResponseSchema,
  CreateCatalogEntryResponseSchema,
  ListAssetsByCatalogEntryResponseSchema,
  ListPortDefinitionsResponseSchema,
  GetPortDefinitionResponseSchema,
  CreatePortDefinitionResponseSchema,
  ListPortCompatibilitiesResponseSchema,
  PortCompatibilitySchema,
  PortDefinitionSchema,
} from '../../generated/v1/catalog_pb';
import {
  PhysicalConnectionService,
  PhysicalConnectionSchema,
  UpdatePhysicalConnectionRequestSchema,
  CreatePhysicalConnectionResponseSchema,
  GetPhysicalConnectionResponseSchema,
  ListConnectionsByPlacementResponseSchema,
  ListConnectionsBySiteResponseSchema,
} from '../../generated/v1/connection_pb';
import {
  LogicalDesignService,
  LogicalDeviceService,
  LogicalConnectionService,
  LogicalDeviceLayoutService,
  ListDesignsResponseSchema,
  GetDesignResponseSchema,
  CreateDesignResponseSchema,
  ListDevicesResponseSchema,
  GetDeviceResponseSchema,
  CreateDeviceResponseSchema,
  ListConnectionsResponseSchema,
  GetConnectionResponseSchema,
  CreateConnectionResponseSchema,
  GetLayoutResponseSchema,
  SaveLayoutResponseSchema,
} from '../../generated/v1/design_pb';
import {
  NoteService,
  NoteSchema,
  ListNotesResponseSchema,
  CreateNoteResponseSchema,
} from '../../generated/v1/note_pb';
import {
  PlacementService,
  PlacementSchema,
  CreatePlacementResponseSchema,
  GetPlacementResponseSchema,
  GetPlacementByAssetResponseSchema,
  ListPlacementsByRackResponseSchema,
  ListChildPlacementsResponseSchema,
} from '../../generated/v1/placement_pb';
import {
  RackService,
  RackSchema,
  ListRacksResponseSchema,
  GetRackResponseSchema,
  CreateRackResponseSchema,
} from '../../generated/v1/rack_pb';
import {
  RackRowService,
  RackRowSchema,
  ListRackRowsResponseSchema,
  GetRackRowResponseSchema,
  CreateRackRowResponseSchema,
} from '../../generated/v1/rack_row_pb';
import {
  RoomService,
  RoomSchema,
  ListRoomsResponseSchema,
  GetRoomResponseSchema,
  CreateRoomResponseSchema,
} from '../../generated/v1/room_pb';
import {
  SiteService,
  SiteSchema,
  ListSitesResponseSchema,
  GetSiteResponseSchema,
  CreateSiteResponseSchema,
} from '../../generated/v1/site_pb';
import {
  TaskService,
  TaskStepService,
  TaskSchema,
  TaskStepSchema,
  UpdateTaskRequestSchema,
  ListTasksResponseSchema,
  GetTaskResponseSchema,
  CreateTaskResponseSchema,
  ListTaskStepsResponseSchema,
  CreateTaskStepResponseSchema,
} from '../../generated/v1/task_pb';
import {
  UserService,
  ListUsersResponseSchema,
  GetCurrentUserResponseSchema,
} from '../../generated/v1/user_pb';
import { AssetStatus, SortDirection } from '../../generated/v1/common_pb';
import * as fx from './fixtures';

/** Enough to see a loading state go by, not enough to sit and wait for it. */
const LATENCY_MS = 180;
const delay = (ms = LATENCY_MS) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

/** What this session has changed. Reloading the page brings back the fixtures. */
const store = {
  sites: [...fx.sites],
  rooms: [...fx.rooms],
  rackRows: [...fx.rackRows],
  racks: [...fx.racks],
  assets: [...fx.assets],
  catalog: [...fx.catalog],
  portDefinitions: [...fx.portDefinitions],
  portCompatibilities: [] as ReturnType<typeof create<typeof PortCompatibilitySchema>>[],
  placements: [...fx.placements],
  connections: [...fx.connections],
  tasks: [...fx.tasks],
  taskSteps: [...fx.taskSteps],
  notes: [...fx.notes],
};

let sequence = 0;
/** A uuid-shaped id for something made during the session. */
const nextId = () => {
  sequence += 1;
  return `feedbeef-0000-4000-8000-${String(sequence).padStart(12, '0')}`;
};

const now = () => timestampFromDate(new Date());

/**
 * The status as it reads on screen, which is what "Status A–Z" sorts on. The
 * enum's own order is a numbering of states, not an alphabet, so sorting on it
 * would put Available before Deployed by accident and Decommissioned last for
 * no reason a reader can see.
 */
const STATUS_SORT_LABEL: Record<number, string> = {
  [AssetStatus.AVAILABLE]: 'Available',
  [AssetStatus.DEPLOYED]: 'Deployed',
  [AssetStatus.NEEDS_REPAIR]: 'Needs repair',
  [AssetStatus.ON_ORDER]: 'On order',
  [AssetStatus.REQUESTED]: 'Requested',
  [AssetStatus.DECOMMISSIONED]: 'Decommissioned',
};

/**
 * Sorts a list the way the API says it will. Ties fall back to the asset tag,
 * so rows with the same status keep one fixed order instead of shuffling
 * between requests.
 */
function sortAssets(
  assets: (typeof store.assets)[number][],
  sortBy: AssetSortField,
  direction: SortDirection,
) {
  const key = (asset: (typeof store.assets)[number]): string => {
    switch (sortBy) {
      case AssetSortField.SERIAL_NUMBER:
        return asset.serialNumber ?? '';
      case AssetSortField.ASSET_TAG:
        return asset.assetTag ?? '';
      case AssetSortField.STATUS:
        return STATUS_SORT_LABEL[asset.status] ?? '';
      default:
        return '';
    }
  };
  const sign = direction === SortDirection.DESC ? -1 : 1;
  return [...assets].sort((a, b) => {
    const cmp = key(a).localeCompare(key(b));
    return cmp !== 0 ? sign * cmp : (a.assetTag ?? '').localeCompare(b.assetTag ?? '');
  });
}

/** Racks under a site, resolved through rooms and rows. */
function racksOfSite(siteId: string) {
  const roomIds = new Set(store.rooms.filter((room) => room.siteId === siteId).map((r) => r.id));
  const rowIds = new Set(
    store.rackRows.filter((row) => roomIds.has(row.roomId)).map((row) => row.id),
  );
  return store.racks.filter((rack) => rowIds.has(rack.rowId));
}

/** What a rack holds, which is what the rack lists report per rack. */
function rackSummary(rack: (typeof store.racks)[number]) {
  const placements = store.placements.filter(
    (placement) =>
      placement.location.case === 'rack' && placement.location.value.rackId === rack.id,
  );
  const entries = placements.map((placement) => {
    const asset = store.assets.find((a) => a.id === placement.assetId);
    return store.catalog.find((entry) => entry.id === asset?.deviceCatalogId);
  });
  const usedUnits = entries.reduce((total, entry) => total + (entry?.rackUnits ?? 0), 0);
  const powerDrawW = entries.reduce((total, entry) => total + (entry?.powerDrawW ?? 0), 0);

  return {
    rack,
    usedUnits,
    freeUnits: Math.max(rack.totalUnits - usedUnits, 0),
    powerDrawW,
    deviceCount: placements.length,
    utilizationPct: rack.totalUnits ? (usedUnits / rack.totalUnits) * 100 : 0,
  };
}

export default function createDemoTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(UserService, {
      listUsers: async () => {
        await delay(60);
        return create(ListUsersResponseSchema, { users: fx.users });
      },
      getCurrentUser: async () => {
        await delay(60);
        return create(GetCurrentUserResponseSchema, { user: fx.currentUser });
      },
    });

    router.service(SiteService, {
      listSites: async () => {
        await delay();
        return create(ListSitesResponseSchema, { sites: store.sites });
      },
      getSite: async (request) => {
        await delay();
        return create(GetSiteResponseSchema, {
          site: store.sites.find((site) => site.id === request.id),
        });
      },
      createSite: async (request) => {
        await delay();
        const site = create(SiteSchema, {
          id: nextId(),
          name: request.name,
          fullName: request.fullName,
          address: request.address,
          city: request.city,
          country: request.country,
          status: 'active',
          created: now(),
        });
        store.sites = [...store.sites, site];
        return create(CreateSiteResponseSchema, { siteId: site.id });
      },
      updateSite: async () => {
        await delay();
        return create(EmptySchema, {});
      },
      deleteSite: async (request) => {
        await delay();
        store.sites = store.sites.filter((site) => site.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(RoomService, {
      listRooms: async (request) => {
        await delay();
        const rooms = request.siteId
          ? store.rooms.filter((room) => room.siteId === request.siteId)
          : store.rooms;
        return create(ListRoomsResponseSchema, { rooms });
      },
      getRoom: async (request) => {
        await delay();
        return create(GetRoomResponseSchema, {
          room: store.rooms.find((room) => room.id === request.id),
        });
      },
      createRoom: async (request) => {
        await delay();
        const room = create(RoomSchema, {
          id: nextId(),
          siteId: request.siteId,
          name: request.name,
          floor: request.floor,
          created: now(),
        });
        store.rooms = [...store.rooms, room];
        return create(CreateRoomResponseSchema, { roomId: room.id });
      },
      // Writes it back like the real service: the pages read their floor again
      // when you come back to it, so a no-op here is a rename that never was.
      updateRoom: async (request) => {
        await delay();
        store.rooms = store.rooms.map((room) =>
          room.id === request.id
            ? create(RoomSchema, { ...room, name: request.name, floor: request.floor })
            : room,
        );
        return create(EmptySchema, {});
      },
      deleteRoom: async (request) => {
        await delay();
        store.rooms = store.rooms.filter((room) => room.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(RackRowService, {
      listRackRows: async (request) => {
        await delay();
        let rows = store.rackRows;
        if (request.roomId) rows = rows.filter((row) => row.roomId === request.roomId);
        if (request.siteId) {
          const roomIds = new Set(
            store.rooms.filter((room) => room.siteId === request.siteId).map((room) => room.id),
          );
          rows = rows.filter((row) => roomIds.has(row.roomId));
        }
        return create(ListRackRowsResponseSchema, { rackRows: rows });
      },
      getRackRow: async (request) => {
        await delay();
        return create(GetRackRowResponseSchema, {
          rackRow: store.rackRows.find((row) => row.id === request.id),
        });
      },
      createRackRow: async (request) => {
        await delay();
        const rackRow = create(RackRowSchema, {
          id: nextId(),
          roomId: request.roomId,
          name: request.name,
          created: now(),
        });
        store.rackRows = [...store.rackRows, rackRow];
        return create(CreateRackRowResponseSchema, { rackRowId: rackRow.id });
      },
      updateRackRow: async (request) => {
        await delay();
        store.rackRows = store.rackRows.map((row) =>
          row.id === request.id
            ? create(RackRowSchema, {
                ...row,
                name: request.name,
                positionX: request.positionX,
                positionY: request.positionY,
              })
            : row,
        );
        return create(EmptySchema, {});
      },
      deleteRackRow: async (request) => {
        await delay();
        store.rackRows = store.rackRows.filter((row) => row.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(RackService, {
      listRacks: async (request) => {
        await delay();
        let racks = store.racks;
        if (request.rowId) racks = racks.filter((rack) => rack.rowId === request.rowId);
        if (request.siteId) {
          const inSite = new Set(racksOfSite(request.siteId).map((rack) => rack.id));
          racks = racks.filter((rack) => inSite.has(rack.id));
        }
        return create(ListRacksResponseSchema, { racks: racks.map(rackSummary) });
      },
      getRack: async (request) => {
        await delay();
        return create(GetRackResponseSchema, {
          rack: store.racks.find((rack) => rack.id === request.id),
        });
      },
      createRack: async (request) => {
        await delay();
        const rack = create(RackSchema, {
          id: nextId(),
          rowId: request.rowId,
          name: request.name,
          totalUnits: request.totalUnits,
          positionInRow: request.positionInRow,
          created: now(),
        });
        store.racks = [...store.racks, rack];
        return create(CreateRackResponseSchema, { rackId: rack.id });
      },
      updateRack: async (request) => {
        await delay();
        store.racks = store.racks.map((rack) =>
          rack.id === request.id
            ? create(RackSchema, {
                ...rack,
                name: request.name,
                totalUnits: request.totalUnits,
              })
            : rack,
        );
        return create(EmptySchema, {});
      },
      deleteRack: async (request) => {
        await delay();
        store.racks = store.racks.filter((rack) => rack.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(AssetService, {
      listAssets: async (request) => {
        await delay();
        // A filter that was never set arrives as UNSPECIFIED rather than as
        // nothing at all, so "no filter" is the zero value and not undefined.
        let assets = store.assets;
        if (request.statusFilter) {
          assets = assets.filter((asset) => asset.status === request.statusFilter);
        }
        if (request.deviceCatalogId) {
          assets = assets.filter((asset) => asset.deviceCatalogId === request.deviceCatalogId);
        }
        if (request.categoryFilter) {
          const inCategory = new Set(
            store.catalog
              .filter((entry) => entry.category === request.categoryFilter)
              .map((entry) => entry.id),
          );
          assets = assets.filter((asset) => inCategory.has(asset.deviceCatalogId));
        }
        if (request.search) {
          const needle = request.search.toLowerCase();
          assets = assets.filter((asset) => {
            const entry = store.catalog.find((c) => c.id === asset.deviceCatalogId);
            return [asset.serialNumber, asset.assetTag, entry?.model, entry?.manufacturer]
              .filter(Boolean)
              .some((value) => value!.toLowerCase().includes(needle));
          });
        }
        assets = sortAssets(assets, request.sortBy, request.sortDirection);
        return create(ListAssetsResponseSchema, { assets });
      },
      getAsset: async (request) => {
        await delay();
        return create(GetAssetResponseSchema, {
          asset: store.assets.find((asset) => asset.id === request.id),
        });
      },
      createAsset: async (request) => {
        await delay();
        const asset = create(AssetSchema, {
          id: nextId(),
          deviceCatalogId: request.deviceCatalogId,
          status: request.status ?? AssetStatus.AVAILABLE,
          serialNumber: request.serialNumber,
          assetTag: request.assetTag,
          notes: request.notes,
          created: now(),
        });
        store.assets = [...store.assets, asset];
        return create(CreateAssetResponseSchema, { assetId: asset.id });
      },
      updateAsset: async (request) => {
        await delay();
        store.assets = store.assets.map((asset) =>
          asset.id === request.id
            ? create(AssetSchema, {
                ...asset,
                status: request.status ?? asset.status,
                serialNumber: request.serialNumber ?? asset.serialNumber,
                assetTag: request.assetTag ?? asset.assetTag,
                notes: request.notes ?? asset.notes,
              })
            : asset,
        );
        return create(EmptySchema, {});
      },
      deleteAsset: async (request) => {
        await delay();
        store.assets = store.assets.filter((asset) => asset.id !== request.id);
        return create(EmptySchema, {});
      },
      getAssetEvents: async (request) => {
        await delay();
        return create(GetAssetEventsResponseSchema, { events: fx.eventsForAsset(request.assetId) });
      },
      getAssetLocation: async (request) => {
        await delay();
        const placement = store.placements.find((p) => p.assetId === request.assetId);
        const spot = placement?.location.case === 'rack' ? placement.location.value : undefined;
        if (!spot) return create(GetAssetLocationResponseSchema, {});

        const rack = store.racks.find((r) => r.id === spot.rackId);
        const row = store.rackRows.find((r) => r.id === rack?.rowId);
        const room = store.rooms.find((r) => r.id === row?.roomId);
        const site = store.sites.find((s) => s.id === room?.siteId);

        return create(GetAssetLocationResponseSchema, {
          location: {
            siteName: site?.name ?? '',
            roomName: room?.name ?? '',
            rackRowName: row?.name ?? '',
            rackName: rack?.name ?? '',
            rackUnitStart: spot.rackUnitStart,
            rackId: rack?.id ?? '',
            rackSlotType: spot.rackSlotType,
          },
        });
      },
      getAssetStats: async () => {
        await delay();
        return create(GetAssetStatsResponseSchema, { stats: fx.assetStats });
      },
    });

    router.service(CatalogService, {
      listCatalog: async (request) => {
        await delay();
        let entries = store.catalog;
        if (request.categoryFilter) {
          entries = entries.filter((entry) => entry.category === request.categoryFilter);
        }
        if (request.search) {
          const needle = request.search.toLowerCase();
          entries = entries.filter((entry) =>
            `${entry.manufacturer} ${entry.model} ${entry.partNumber}`
              .toLowerCase()
              .includes(needle),
          );
        }

        return create(ListCatalogResponseSchema, {
          entries: entries.map((entry) => {
            const owned = store.assets.filter((asset) => asset.deviceCatalogId === entry.id);
            return {
              entry,
              total: owned.length,
              deployed: owned.filter((asset) => asset.status === AssetStatus.DEPLOYED).length,
              available: owned.filter((asset) => asset.status === AssetStatus.AVAILABLE).length,
              needsRepair: owned.filter((asset) => asset.status === AssetStatus.NEEDS_REPAIR)
                .length,
            };
          }),
        });
      },
      getCatalogEntry: async (request) => {
        await delay();
        return create(GetCatalogEntryResponseSchema, {
          entry: store.catalog.find((entry) => entry.id === request.id),
        });
      },
      createCatalogEntry: async (request) => {
        await delay();
        const entry = create(DeviceCatalogSchema, {
          id: nextId(),
          manufacturer: request.manufacturer,
          model: request.model,
          partNumber: request.partNumber,
          category: request.category,
          formFactor: request.formFactor,
          rackUnits: request.rackUnits,
          weightKg: request.weightKg,
          powerDrawW: request.powerDrawW,
          specs: request.specs,
          created: now(),
        });
        store.catalog = [...store.catalog, entry];
        return create(CreateCatalogEntryResponseSchema, { catalogEntryId: entry.id });
      },
      updateCatalogEntry: async (request) => {
        await delay();
        // Writes it back, like the real service: the pages read their product
        // again after a write rather than patching it in place, so a no-op here
        // shows up as an edit that did nothing.
        store.catalog = store.catalog.map((entry) =>
          entry.id === request.id
            ? create(DeviceCatalogSchema, {
                ...entry,
                manufacturer: request.manufacturer,
                model: request.model,
                partNumber: request.partNumber,
                category: request.category,
                specs: request.specs,
              })
            : entry,
        );
        return create(EmptySchema, {});
      },
      deleteCatalogEntry: async (request) => {
        await delay();
        store.catalog = store.catalog.filter((entry) => entry.id !== request.id);
        return create(EmptySchema, {});
      },
      listAssetsByCatalogEntry: async (request) => {
        await delay();
        return create(ListAssetsByCatalogEntryResponseSchema, {
          assets: store.assets.filter((asset) => asset.deviceCatalogId === request.deviceCatalogId),
        });
      },
      listPortDefinitions: async (request) => {
        await delay();
        return create(ListPortDefinitionsResponseSchema, {
          portDefinitions: store.portDefinitions.filter(
            (port) => port.deviceCatalogId === request.deviceCatalogId,
          ),
        });
      },
      getPortDefinition: async (request) => {
        await delay();
        return create(GetPortDefinitionResponseSchema, {
          portDefinition: store.portDefinitions.find((port) => port.id === request.id),
        });
      },
      createPortDefinition: async (request) => {
        await delay();
        const portDefinition = create(PortDefinitionSchema, {
          id: nextId(),
          deviceCatalogId: request.deviceCatalogId,
          name: request.name,
          portType: request.portType,
          mediaType: request.mediaType,
          speed: request.speed,
          direction: request.direction,
          ordinal: request.ordinal,
        });
        store.portDefinitions = [...store.portDefinitions, portDefinition];
        return create(CreatePortDefinitionResponseSchema, { portDefinitionId: portDefinition.id });
      },
      updatePortDefinition: async () => {
        await delay();
        return create(EmptySchema, {});
      },
      deletePortDefinition: async (request) => {
        await delay();
        store.portDefinitions = store.portDefinitions.filter((port) => port.id !== request.id);
        return create(EmptySchema, {});
      },
      // Kept in the store like everything else: the pages read them back after a
      // write, so a no-op here shows up as a compatibility that will not stick.
      listPortCompatibilities: async (request) => {
        await delay();
        return create(ListPortCompatibilitiesResponseSchema, {
          compatibilities: store.portCompatibilities.filter(
            (c) => c.portDefinitionId === request.portDefinitionId,
          ),
        });
      },
      createPortCompatibility: async (request) => {
        await delay();
        const entry = store.catalog.find((c) => c.id === request.compatibleCatalogId);
        store.portCompatibilities = [
          ...store.portCompatibilities,
          create(PortCompatibilitySchema, {
            portDefinitionId: request.portDefinitionId,
            compatibleCatalogId: request.compatibleCatalogId,
            compatibleCategory: entry?.category,
          }),
        ];
        return create(EmptySchema, {});
      },
      deletePortCompatibility: async (request) => {
        await delay();
        store.portCompatibilities = store.portCompatibilities.filter(
          (c) =>
            !(
              c.portDefinitionId === request.portDefinitionId &&
              c.compatibleCatalogId === request.compatibleCatalogId
            ),
        );
        return create(EmptySchema, {});
      },
    });

    router.service(PlacementService, {
      createPlacement: async (request) => {
        await delay();
        const placement = create(PlacementSchema, {
          id: nextId(),
          assetId: request.assetId,
          location: request.location,
          notes: request.notes,
          created: now(),
        });
        store.placements = [...store.placements, placement];
        return create(CreatePlacementResponseSchema, { placementId: placement.id });
      },
      getPlacement: async (request) => {
        await delay();
        return create(GetPlacementResponseSchema, {
          placement: store.placements.find((placement) => placement.id === request.id),
        });
      },
      getPlacementByAsset: async (request) => {
        await delay();
        return create(GetPlacementByAssetResponseSchema, {
          placement: store.placements.find((placement) => placement.assetId === request.assetId),
        });
      },
      updatePlacement: async (request) => {
        await delay();
        store.placements = store.placements.map((placement) =>
          placement.id === request.id && request.location.case
            ? create(PlacementSchema, { ...placement, location: request.location })
            : placement,
        );
        return create(EmptySchema, {});
      },
      deletePlacement: async (request) => {
        await delay();
        store.placements = store.placements.filter((placement) => placement.id !== request.id);
        return create(EmptySchema, {});
      },
      listPlacementsByRack: async (request) => {
        await delay();
        return create(ListPlacementsByRackResponseSchema, {
          placements: store.placements.filter(
            (placement) =>
              placement.location.case === 'rack' &&
              placement.location.value.rackId === request.rackId,
          ),
        });
      },
      listChildPlacements: async () => {
        await delay();
        return create(ListChildPlacementsResponseSchema, { placements: [] });
      },
    });

    router.service(PhysicalConnectionService, {
      createPhysicalConnection: async (request) => {
        await delay();
        const connection = create(PhysicalConnectionSchema, {
          id: nextId(),
          sourcePlacementId: request.sourcePlacementId,
          sourcePortDefinitionId: request.sourcePortDefinitionId,
          targetPlacementId: request.targetPlacementId,
          targetPortDefinitionId: request.targetPortDefinitionId,
          notes: request.notes,
          created: now(),
        });
        store.connections = [...store.connections, connection];
        return create(CreatePhysicalConnectionResponseSchema, { connectionId: connection.id });
      },
      getPhysicalConnection: async (request) => {
        await delay();
        return create(GetPhysicalConnectionResponseSchema, {
          connection: store.connections.find((connection) => connection.id === request.id),
        });
      },
      updatePhysicalConnection: async (request) => {
        await delay();
        // It used to answer "fine" and change nothing, so every edit to a cable
        // came back the way it was on the next read: the status menu, the
        // colour, the label. Same explicit-presence rule as updateTask, so a
        // field nobody sent keeps what it had.
        const sent = (name: string) => {
          const field = UpdatePhysicalConnectionRequestSchema.fields.find(
            (f) => f.localName === name,
          );
          return !!field && isFieldSet(request, field);
        };
        store.connections = store.connections.map((connection) => {
          if (connection.id !== request.id) return connection;
          return create(PhysicalConnectionSchema, {
            ...connection,
            cableType: sent('cableType') ? request.cableType : connection.cableType,
            status: sent('status') ? request.status : connection.status,
            color: sent('color') ? request.color : connection.color,
            label: sent('label') ? request.label : connection.label,
            notes: sent('notes') ? request.notes : connection.notes,
          });
        });
        return create(EmptySchema, {});
      },
      deletePhysicalConnection: async (request) => {
        await delay();
        store.connections = store.connections.filter((connection) => connection.id !== request.id);
        return create(EmptySchema, {});
      },
      listConnectionsByPlacement: async (request) => {
        await delay();
        return create(ListConnectionsByPlacementResponseSchema, {
          connections: store.connections.filter(
            (connection) =>
              connection.sourcePlacementId === request.placementId ||
              connection.targetPlacementId === request.placementId,
          ),
        });
      },
      listConnectionsBySite: async (request) => {
        await delay();
        // Actually by site. It used to hand back every connection whatever was
        // asked for, which nothing noticed while one site was ever loaded at a
        // time: the caller stamped its own site onto them. Read two, and every
        // cable came back twice.
        const rackIds = new Set(racksOfSite(request.siteId).map((rack) => rack.id));
        const inSite = new Set(
          store.placements
            .filter(
              (placement) =>
                placement.location.case === 'rack' &&
                rackIds.has(placement.location.value.rackId),
            )
            .map((placement) => placement.id),
        );
        return create(ListConnectionsBySiteResponseSchema, {
          connections: store.connections.filter(
            (connection) =>
              inSite.has(connection.sourcePlacementId) || inSite.has(connection.targetPlacementId),
          ),
        });
      },
    });

    router.service(TaskService, {
      listTasks: async (request) => {
        await delay();
        let tasks = store.tasks;
        if (request.status) {
          tasks = tasks.filter((task) => task.status === request.status);
        }
        if (request.priority) {
          tasks = tasks.filter((task) => task.priority === request.priority);
        }
        if (request.tag) {
          tasks = tasks.filter((task) => task.tags.includes(request.tag!));
        }
        if (request.assigneeId) {
          tasks = tasks.filter((task) => task.assigneeId === request.assigneeId);
        }
        return create(ListTasksResponseSchema, { tasks });
      },
      getTask: async (request) => {
        await delay();
        return create(GetTaskResponseSchema, {
          task: store.tasks.find((task) => task.id === request.id),
        });
      },
      createTask: async (request) => {
        await delay();
        const task = create(TaskSchema, {
          id: nextId(),
          title: request.title,
          description: request.description,
          status: request.status,
          priority: request.priority,
          tags: request.tags,
          assigneeId: request.assigneeId,
          dueDate: request.dueDate,
          location: request.location,
          blockedReason: request.blockedReason,
          created: now(),
        });
        store.tasks = [...store.tasks, task];
        return create(CreateTaskResponseSchema, { taskId: task.id });
      },
      updateTask: async (request) => {
        await delay();
        // Every patchable field here carries explicit presence, so "not sent"
        // and "sent empty" are two different things and the runtime knows which
        // is which. The generated TypeScript types do not show it — they say
        // plain `string` — but the descriptor does, and isFieldSet reads it.
        //
        // That is what lets a clear be a clear. Unassigning somebody sends an
        // empty assignee on purpose, and truthiness cannot tell that from a
        // status-only update that never mentioned the assignee at all.
        const sent = (name: string) => {
          const field = UpdateTaskRequestSchema.fields.find((f) => f.localName === name);
          return !!field && isFieldSet(request, field);
        };
        // The one field presence cannot carry: an empty reason is a real value —
        // waiting, with nothing typed — so clearing it needs a flag of its own.
        const blockedReasonFor = (task: (typeof store.tasks)[number]) => {
          if (request.clearBlockedReason) return '';
          return sent('blockedReason') ? request.blockedReason : task.blockedReason;
        };
        store.tasks = store.tasks.map((task) =>
          task.id === request.id
            ? create(TaskSchema, {
                ...task,
                title: sent('title') ? request.title : task.title,
                description: sent('description') ? request.description : task.description,
                status: sent('status') ? request.status : task.status,
                priority: sent('priority') ? request.priority : task.priority,
                // Replaced, not merged: the request says what the task should
                // carry, so an empty list clears the tags.
                tags: request.tags.length > 0 ? request.tags : task.tags,
                assigneeId: sent('assigneeId') ? request.assigneeId : task.assigneeId,
                blockedReason: blockedReasonFor(task),
                dueDate: sent('dueDate') ? request.dueDate : task.dueDate,
                location: sent('location') ? request.location : task.location,
              })
            : task,
        );
        return create(EmptySchema, {});
      },
      deleteTask: async (request) => {
        await delay();
        store.tasks = store.tasks.filter((task) => task.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(TaskStepService, {
      listTaskSteps: async (request) => {
        await delay();
        return create(ListTaskStepsResponseSchema, {
          steps: store.taskSteps.filter((step) => step.taskId === request.taskId),
        });
      },
      createTaskStep: async (request) => {
        await delay();
        const step = create(TaskStepSchema, {
          id: nextId(),
          taskId: request.taskId,
          title: request.title,
          description: request.description,
          ordinal: request.ordinal,
          created: now(),
        });
        store.taskSteps = [...store.taskSteps, step];
        return create(CreateTaskStepResponseSchema, { taskStepId: step.id });
      },
      updateTaskStep: async (request) => {
        await delay();
        store.taskSteps = store.taskSteps.map((step) =>
          step.id === request.id
            ? create(TaskStepSchema, {
                ...step,
                title: request.title ?? step.title,
                description: request.description ?? step.description,
                completed: request.completed ?? step.completed,
              })
            : step,
        );
        return create(EmptySchema, {});
      },
      deleteTaskStep: async (request) => {
        await delay();
        store.taskSteps = store.taskSteps.filter((step) => step.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    router.service(NoteService, {
      listNotes: async (request) => {
        await delay();
        return create(ListNotesResponseSchema, {
          notes: store.notes.filter(
            (note) => note.entityType === request.entityType && note.entityId === request.entityId,
          ),
        });
      },
      createNote: async (request) => {
        await delay();
        const note = create(NoteSchema, {
          id: nextId(),
          entityType: request.entityType,
          entityId: request.entityId,
          body: request.body,
          createdBy: fx.currentUser.name,
          createdById: fx.currentUser.id,
          created: now(),
        });
        store.notes = [...store.notes, note];
        return create(CreateNoteResponseSchema, { noteId: note.id });
      },
      deleteNote: async (request) => {
        await delay();
        store.notes = store.notes.filter((note) => note.id !== request.id);
        return create(EmptySchema, {});
      },
    });

    // The logical design side has no fixtures yet: it answers with empty lists so
    // the screens render their own "nothing here" state instead of an error.
    router.service(LogicalDesignService, {
      listDesigns: async () => create(ListDesignsResponseSchema, { designs: [] }),
      getDesign: async () => create(GetDesignResponseSchema, {}),
      createDesign: async () => create(CreateDesignResponseSchema, {}),
      updateDesign: async () => create(EmptySchema, {}),
      deleteDesign: async () => create(EmptySchema, {}),
    });

    router.service(LogicalDeviceService, {
      listDevices: async () => create(ListDevicesResponseSchema, { devices: [] }),
      getDevice: async () => create(GetDeviceResponseSchema, {}),
      createDevice: async () => create(CreateDeviceResponseSchema, {}),
      updateDevice: async () => create(EmptySchema, {}),
      deleteDevice: async () => create(EmptySchema, {}),
    });

    router.service(LogicalConnectionService, {
      listConnections: async () => create(ListConnectionsResponseSchema, { connections: [] }),
      getConnection: async () => create(GetConnectionResponseSchema, {}),
      createConnection: async () => create(CreateConnectionResponseSchema, {}),
      updateConnection: async () => create(EmptySchema, {}),
      deleteConnection: async () => create(EmptySchema, {}),
    });

    router.service(LogicalDeviceLayoutService, {
      getLayout: async () => create(GetLayoutResponseSchema, {}),
      saveLayout: async () => create(SaveLayoutResponseSchema, {}),
      deleteLayout: async () => create(EmptySchema, {}),
    });
  });
}
