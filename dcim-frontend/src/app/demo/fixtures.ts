// Demo-only data: one coherent data center estate, built in memory so the whole
// app can be looked at without a backend. Never imported by the production
// entrypoint (main.ts).
//
// The estate is generated rather than typed out record by record: two sites,
// each with rooms, rows and racks, and a catalog that the assets are drawn from.
// Generating it keeps the relations right — every placement points at a rack
// that exists and an asset that exists — which is what makes the screens
// readable.
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { AssetSchema, AssetStatsSchema } from '../../generated/v1/asset_pb';
import { DeviceCatalogSchema, PortDefinitionSchema } from '../../generated/v1/catalog_pb';
import { PlacementSchema } from '../../generated/v1/placement_pb';
import { RackSchema } from '../../generated/v1/rack_pb';
import { RackRowSchema } from '../../generated/v1/rack_row_pb';
import { RoomSchema } from '../../generated/v1/room_pb';
import { SiteSchema } from '../../generated/v1/site_pb';
import { NoteSchema } from '../../generated/v1/note_pb';
import { TaskSchema, TaskStepSchema, TaskPriority, TaskStatus } from '../../generated/v1/task_pb';
import { UserSchema } from '../../generated/v1/user_pb';
import { PhysicalConnectionSchema } from '../../generated/v1/connection_pb';
import {
  AssetCategory,
  AssetEventType,
  AssetStatus,
  NoteEntityType,
  PortDirection,
  PortType,
  RackSlotType,
} from '../../generated/v1/common_pb';

// A fixed clock: the same data every run, so a screenshot from yesterday still
// matches what you see today.
const NOW = new Date('2026-08-12T09:00:00Z');
const daysAgo = (days: number) => timestampFromDate(new Date(NOW.getTime() - days * 86_400_000));
const daysAhead = (days: number) => timestampFromDate(new Date(NOW.getTime() + days * 86_400_000));

/** Ids are uuid-shaped because the API validates them as uuids, and readable in
 *  the middle so a request in the network tab still says what it is about. */
function id(kind: string, n: number): string {
  const tag = kind.slice(0, 8).padEnd(8, '0');
  const seq = String(n).padStart(12, '0');
  return `${tag}-0000-4000-8000-${seq}`;
}

// ── The staff ────────────────────────────────────────────────────────────────

export const users = [
  { name: 'Daan Hofman', email: 'daan.hofman@example.gov' },
  { name: 'Yara Nijhuis', email: 'yara.nijhuis@example.gov' },
  { name: 'Ruben de Groot', email: 'ruben.degroot@example.gov' },
  { name: 'Iris Wolters', email: 'iris.wolters@example.gov' },
  { name: 'Sem Bakker', email: 'sem.bakker@example.gov' },
].map((person, index) =>
  create(UserSchema, { id: id('user', index + 1), name: person.name, email: person.email }),
);

/** Whoever is signed in while presenting. */
export const currentUser = users[0];

// ── The catalog ──────────────────────────────────────────────────────────────

const CATALOG = [
  {
    manufacturer: 'Dell',
    model: 'PowerEdge R660',
    part: 'R660-2U',
    category: AssetCategory.SERVER,
    units: 2,
    weight: 21.5,
    power: 750,
  },
  {
    manufacturer: 'Dell',
    model: 'PowerEdge R760',
    part: 'R760-2U',
    category: AssetCategory.SERVER,
    units: 2,
    weight: 24,
    power: 900,
  },
  {
    manufacturer: 'HPE',
    model: 'ProLiant DL360',
    part: 'DL360-G11',
    category: AssetCategory.SERVER,
    units: 1,
    weight: 16.2,
    power: 500,
  },
  {
    manufacturer: 'Arista',
    model: '7050SX3-48YC8',
    part: '7050SX3',
    category: AssetCategory.SWITCH,
    units: 1,
    weight: 9.1,
    power: 250,
  },
  {
    manufacturer: 'Juniper',
    model: 'QFX5120-48Y',
    part: 'QFX5120',
    category: AssetCategory.SWITCH,
    units: 1,
    weight: 8.6,
    power: 230,
  },
  {
    manufacturer: 'APC',
    model: 'Rack PDU 9000',
    part: 'AP9000',
    category: AssetCategory.PDU,
    units: 0,
    weight: 4.5,
    power: 0,
  },
  {
    manufacturer: 'Panduit',
    model: 'Patchpaneel 24-poorts',
    part: 'PP24-LC',
    category: AssetCategory.PATCH_PANEL,
    units: 1,
    weight: 2.1,
    power: 0,
  },
  {
    manufacturer: 'Finisar',
    model: 'SFP28 25G SR',
    part: 'FTLF8536',
    category: AssetCategory.SFP,
    units: 0,
    weight: 0.02,
    power: 1,
  },
  {
    manufacturer: 'Samsung',
    model: 'PM9A3 3.84TB NVMe',
    part: 'MZQL23T8',
    category: AssetCategory.DISK,
    units: 0,
    weight: 0.15,
    power: 12,
  },
  {
    manufacturer: 'Corning',
    model: 'LC-LC OM4 3m',
    part: 'LC-OM4-3',
    category: AssetCategory.CABLE,
    units: 0,
    weight: 0.1,
    power: 0,
  },
];

export const catalog = CATALOG.map((entry, index) =>
  create(DeviceCatalogSchema, {
    id: id('catalog', index + 1),
    manufacturer: entry.manufacturer,
    model: entry.model,
    partNumber: entry.part,
    category: entry.category,
    formFactor: entry.units > 0 ? `${entry.units}U rackmount` : 'module',
    rackUnits: entry.units > 0 ? entry.units : undefined,
    weightKg: entry.weight,
    powerDrawW: entry.power,
    specs:
      entry.category === AssetCategory.SERVER
        ? { cpu: '2x Intel Xeon Gold 6430', ram: '512 GB', nic: '2x 25GbE' }
        : {},
    created: daysAgo(400 - index * 12),
  }),
);

/** Every network device gets its ports, so the patch mapping has something to
 *  draw and the catalog page has something to show under an entry. */
export const portDefinitions = catalog.flatMap((entry) => {
  const isSwitch = entry.category === AssetCategory.SWITCH;
  const isPanel = entry.category === AssetCategory.PATCH_PANEL;
  const isServer = entry.category === AssetCategory.SERVER;
  const count = (isSwitch && 8) || (isPanel && 24) || (isServer && 2) || 0;

  return Array.from({ length: count }, (unused, port) =>
    create(PortDefinitionSchema, {
      id: id(`port${entry.id.slice(-2)}`, port + 1),
      deviceCatalogId: entry.id,
      name: isServer ? `NIC ${port + 1}` : `Port ${port + 1}`,
      portType: PortType.NETWORK,
      mediaType: 'SFP28',
      speed: isServer ? '25G' : '100G',
      direction: PortDirection.BIDIR,
      ordinal: port + 1,
    }),
  );
});

// ── The estate: sites, rooms, rows, racks ────────────────────────────────────

const SITES = [
  {
    name: 'AMS1',
    fullName: 'Amsterdam Zuidoost',
    city: 'Amsterdam',
    address: 'Paasheuvelweg 12',
    tier: 'Tier III',
    sqm: 1800,
  },
  {
    name: 'GRN1',
    fullName: 'Groningen Noord',
    city: 'Groningen',
    address: 'Zernikelaan 40',
    tier: 'Tier II',
    sqm: 950,
  },
];

export const sites = SITES.map((site, index) =>
  create(SiteSchema, {
    id: id('site', index + 1),
    name: site.name,
    fullName: site.fullName,
    address: site.address,
    city: site.city,
    country: 'Nederland',
    tier: site.tier,
    floorSqm: site.sqm,
    established: daysAgo(1200 - index * 300),
    status: 'active',
    created: daysAgo(1200 - index * 300),
  }),
);

export const rooms = sites.flatMap((site, siteIndex) =>
  ['Hal A', 'Hal B'].map((name, roomIndex) =>
    create(RoomSchema, {
      id: id('room', siteIndex * 2 + roomIndex + 1),
      siteId: site.id,
      name,
      floor: roomIndex === 0 ? 'Begane grond' : '1e verdieping',
      created: daysAgo(1100),
    }),
  ),
);

export const rackRows = rooms.flatMap((room, roomIndex) =>
  ['Rij 1', 'Rij 2'].map((name, rowIndex) =>
    create(RackRowSchema, {
      id: id('rackrow', roomIndex * 2 + rowIndex + 1),
      roomId: room.id,
      name,
      positionX: rowIndex * 4,
      positionY: 0,
      created: daysAgo(1080),
    }),
  ),
);

export const racks = rackRows.flatMap((row, rowIndex) =>
  Array.from({ length: 3 }, (unused, rackIndex) =>
    create(RackSchema, {
      id: id('rack', rowIndex * 3 + rackIndex + 1),
      rowId: row.id,
      name: `R${String(rowIndex + 1).padStart(2, '0')}-${rackIndex + 1}`,
      totalUnits: 42,
      positionInRow: rackIndex + 1,
      created: daysAgo(1060),
    }),
  ),
);

// ── Assets and where they sit ────────────────────────────────────────────────

const STATUS_MIX = [
  AssetStatus.DEPLOYED,
  AssetStatus.DEPLOYED,
  AssetStatus.DEPLOYED,
  AssetStatus.DEPLOYED,
  AssetStatus.AVAILABLE,
  AssetStatus.AVAILABLE,
  AssetStatus.NEEDS_REPAIR,
  AssetStatus.ON_ORDER,
  AssetStatus.REQUESTED,
  AssetStatus.DECOMMISSIONED,
];

export const assets = Array.from({ length: 70 }, (unused, index) => {
  const entry = catalog[index % catalog.length];
  const status = STATUS_MIX[index % STATUS_MIX.length];

  return create(AssetSchema, {
    id: id('asset', index + 1),
    deviceCatalogId: entry.id,
    status,
    serialNumber:
      entry.category === AssetCategory.CABLE ? undefined : `SN${String(100000 + index * 37)}`,
    assetTag: `NL-${String(index + 1).padStart(5, '0')}`,
    purchaseDate: daysAgo(320 - index),
    purchaseOrder: `PO-2025-${String(1000 + (index % 12))}`,
    warrantyExpiry: daysAhead(400 - index * 4),
    notes: '',
    created: daysAgo(320 - index),
  });
});

/** Only what is deployed sits in a rack, so the rack views show a plausible
 *  filling rather than a wall of hardware. */
export const placements = assets
  .filter((asset) => asset.status === AssetStatus.DEPLOYED)
  .map((asset, index) => {
    const entry = catalog.find((c) => c.id === asset.deviceCatalogId);
    const rack = racks[index % racks.length];
    const height = entry?.rackUnits ?? 1;

    return create(PlacementSchema, {
      id: id('place', index + 1),
      assetId: asset.id,
      location: {
        case: 'rack',
        value: {
          rackId: rack.id,
          rackUnitStart: 1 + ((index * 3) % (42 - height)),
          rackSlotType: RackSlotType.UNIT,
        },
      },
      notes: '',
      created: daysAgo(300 - index),
    });
  });

export const assetStats = create(AssetStatsSchema, {
  total: assets.length,
  available: assets.filter((a) => a.status === AssetStatus.AVAILABLE).length,
  deployed: assets.filter((a) => a.status === AssetStatus.DEPLOYED).length,
  needsRepair: assets.filter((a) => a.status === AssetStatus.NEEDS_REPAIR).length,
  onOrder: assets.filter((a) => a.status === AssetStatus.ON_ORDER).length,
  requested: assets.filter((a) => a.status === AssetStatus.REQUESTED).length,
  decommissioned: assets.filter((a) => a.status === AssetStatus.DECOMMISSIONED).length,
});

/** The history one asset carries, for the detail page's timeline. */
export function eventsForAsset(assetId: string) {
  const index = assets.findIndex((asset) => asset.id === assetId);
  if (index < 0) return [];

  const kinds = [
    { type: AssetEventType.REQUESTED, details: 'Aangevraagd voor uitbreiding Hal A' },
    { type: AssetEventType.RECEIVED, details: 'Ontvangen en ingeboekt' },
    { type: AssetEventType.DEPLOYED, details: 'Geplaatst en aangesloten' },
    { type: AssetEventType.NOTE, details: 'Firmware bijgewerkt naar 2.4.1' },
  ];

  return kinds.map((kind, step) => ({
    id: id(`event${index}`, step + 1),
    assetId,
    eventType: kind.type,
    details: kind.details,
    performedBy: users[(index + step) % users.length].name,
    created: daysAgo(300 - index - step * 20),
  }));
}

// ── Cabling ──────────────────────────────────────────────────────────────────

/** The ports a placed device actually has: a cable that ends on a port of some
 *  other model reads as a raw id on screen, because there is nothing to resolve
 *  the name against. */
function portsOfPlacement(placement: (typeof placements)[number]) {
  const asset = assets.find((candidate) => candidate.id === placement.assetId);
  return portDefinitions.filter((port) => port.deviceCatalogId === asset?.deviceCatalogId);
}

/** A handful of runs between devices that have ports, enough for the patch
 *  mapping to draw something real. */
export const connections = placements
  .map((placement, index) => ({
    source: placement,
    target: placements[(index + 4) % placements.length],
  }))
  .filter(
    ({ source, target }) =>
      source !== target &&
      portsOfPlacement(source).length > 0 &&
      portsOfPlacement(target).length > 0,
  )
  .slice(0, 8)
  .map(({ source, target }, index) => {
    const sourcePorts = portsOfPlacement(source);
    const targetPorts = portsOfPlacement(target);

    return create(PhysicalConnectionSchema, {
      id: id('conn', index + 1),
      sourcePlacementId: source.id,
      sourcePortDefinitionId: sourcePorts[index % sourcePorts.length].id,
      targetPlacementId: target.id,
      targetPortDefinitionId: targetPorts[index % targetPorts.length].id,
      notes: '',
      created: daysAgo(200 - index * 5),
    });
  });

// ── Work in progress ─────────────────────────────────────────────────────────

// The menu offers four views, so the data has to fill all four. `assignee` is an
// index into users and null means nobody; `due` counts days from today and null
// means no date. Both were derived from the row index before, which was shorter
// to write and impossible to aim: an unassigned task without a date (Inbox) or
// one that sits with a colleague (Waiting) is a fact about that task, not about
// where it happens to fall in the list.
const TASKS = [
  {
    title: 'Switch R01-2 vervangen',
    tags: ['network', 'hardware'],
    status: TaskStatus.DOING,
    priority: TaskPriority.HIGH,
    due: 2,
    assignee: 0,
  },
  {
    title: 'Nieuwe servers uitpakken en inboeken',
    tags: ['hardware'],
    status: TaskStatus.TODO,
    priority: TaskPriority.MEDIUM,
    due: 5,
    assignee: 1,
  },
  {
    title: 'Koeling Hal B nakijken',
    tags: ['cooling'],
    status: TaskStatus.DOING,
    blockedReason: 'Waiting on an ordered part to arrive',
    priority: TaskPriority.URGENT,
    due: 1,
    assignee: 0,
  },
  {
    title: 'PDU-belasting rij 2 meten',
    tags: ['power'],
    status: TaskStatus.TODO,
    priority: TaskPriority.LOW,
    due: 9,
    assignee: 3,
  },
  {
    title: 'Toegangspassen controleren',
    tags: ['security', 'review'],
    status: TaskStatus.DOING,
    priority: TaskPriority.UNSPECIFIED,
    due: -1,
    assignee: 0,
  },
  {
    title: 'Patchkabels opruimen R02-1',
    tags: [],
    status: TaskStatus.DONE,
    priority: TaskPriority.LOW,
    due: -3,
    assignee: 4,
  },
  {
    title: 'Defecte disk vervangen in R01-3',
    tags: ['hardware'],
    status: TaskStatus.TODO,
    priority: TaskPriority.HIGH,
    due: 3,
    assignee: 0,
  },
  {
    title: 'Uplink naar GRN1 testen',
    tags: ['network'],
    status: TaskStatus.DOING,
    priority: TaskPriority.HIGH,
    due: 6,
    assignee: 2,
  },
  {
    title: 'Melding: brommend geluid bij R03',
    tags: [],
    status: TaskStatus.TODO,
    priority: TaskPriority.UNSPECIFIED,
    due: null,
    assignee: null,
  },
  {
    title: 'Reserveonderdelen bijbestellen',
    tags: ['hardware'],
    status: TaskStatus.TODO,
    priority: TaskPriority.LOW,
    due: null,
    assignee: 0,
  },
];

export const tasks = TASKS.map((task, index) =>
  create(TaskSchema, {
    id: id('task', index + 1),
    title: task.title,
    description:
      'Loop de stappen langs en leg per stap vast wat je hebt gedaan. Bij twijfel: eerst melden, dan pas ingrijpen.',
    status: task.status,
    blockedReason: task.blockedReason,
    priority: task.priority,
    tags: task.tags,
    assigneeId: task.assignee === null ? undefined : users[task.assignee].id,
    dueDate: task.due === null ? undefined : daysAhead(task.due),
    location: `${sites[index % sites.length].name} · ${racks[index % racks.length].name}`,
    created: daysAgo(20 - index),
  }),
);

const STEPS = [
  'Materiaal verzamelen en meenemen naar de vloer',
  'Apparaat spanningsloos maken',
  'Oude onderdeel verwijderen en labelen',
  'Nieuw onderdeel plaatsen en aansluiten',
  'Testen en de status bijwerken',
];

export const taskSteps = tasks.flatMap((task, taskIndex) =>
  STEPS.map((title, stepIndex) =>
    create(TaskStepSchema, {
      id: id(`step${taskIndex}`, stepIndex + 1),
      taskId: task.id,
      title,
      description: '',
      ordinal: stepIndex + 1,
      completed:
        task.status === TaskStatus.DONE || (task.status === TaskStatus.DOING && stepIndex < 2),
      created: daysAgo(20 - taskIndex),
    }),
  ),
);

// ── Notes ────────────────────────────────────────────────────────────────────

export const notes = [
  create(NoteSchema, {
    id: id('note', 1),
    entityType: NoteEntityType.ASSET,
    entityId: assets[0].id,
    body: 'Ventilator maakt af en toe lawaai, in de gaten houden.',
    createdBy: users[1].name,
    createdById: users[1].id,
    created: daysAgo(12),
  }),
  create(NoteSchema, {
    id: id('note', 2),
    entityType: NoteEntityType.TASK,
    entityId: tasks[0].id,
    body: 'Reserveswitch ligt klaar in het magazijn, plank C3.',
    createdBy: users[2].name,
    createdById: users[2].id,
    created: daysAgo(3),
  }),
];
