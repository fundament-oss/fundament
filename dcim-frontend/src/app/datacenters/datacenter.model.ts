import { RACKS } from '../racks/rack.model';

// ── Types ─────────────────────────────────────────────────────────────────────

export type DatacenterStatus = 'operational' | 'degraded' | 'maintenance';
export type RackOwnership = 'own' | 'other-client';
export type RackFloorStatus = 'operational' | 'issue';

export interface DatacenterInfo {
  id: string;
  name: string;
  fullName: string;
  city: string;
  country: string;
  tier: 1 | 2 | 3 | 4;
  established: number;
  status: DatacenterStatus;
  floorSqm: number;
  powerCapacityKw: number;
  coolingCapacityKw: number;
  pue: number;
  address: string;
}

export interface RackFloorPosition {
  dcId: string;
  rackId?: string;
  row: string;
  col: number;
  ownership: RackOwnership;
  floorStatus?: RackFloorStatus;
}

export interface AisleDefinition {
  afterRow: string;
  type: 'cold' | 'hot';
}

// ── View model shared between DatacentersComponent and IsometricCanvasComponent ──

export interface RackCell {
  rackId: string | undefined;
  rackName: string;
  row: string;
  col: number;
  fillPct: number;
  deviceCount: number;
  powerW: number;
  ownership: 'own' | 'other-client';
  floorStatus: RackFloorStatus | 'n/a';
}

// ── View model shared between DatacentersComponent and IsometricCanvasComponent ──

export interface RackCell {
  rackId: string | undefined;
  rackName: string;
  row: string;
  col: number;
  fillPct: number;
  deviceCount: number;
  powerW: number;
  ownership: 'own' | 'other-client';
  floorStatus: RackFloorStatus | 'n/a';
}

export interface Room {
  id: string;
  siteId: string;
  name: string;
  floor: number;
}

export interface RackRow {
  id: string;
  roomId: string;
  name: string;
  positionX: number;
  positionY: number;
}

export interface DatacenterFloorConfig {
  dcId: string;
  aisles: AisleDefinition[];
}

// ── Mock data ─────────────────────────────────────────────────────────────────

export const DATACENTER_INFO: DatacenterInfo[] = [
  {
    id: 'ams-01',
    name: 'AMS-01',
    fullName: 'Amsterdam West',
    city: 'Amsterdam',
    country: 'Netherlands',
    tier: 3,
    established: 2018,
    status: 'operational',
    floorSqm: 1200,
    powerCapacityKw: 500,
    coolingCapacityKw: 480,
    pue: 1.45,
    address: 'Westlandgracht 40, 1060 AD Amsterdam',
  },
  {
    id: 'ams-02',
    name: 'AMS-02',
    fullName: 'Amsterdam South-East',
    city: 'Amsterdam',
    country: 'Netherlands',
    tier: 3,
    established: 2020,
    status: 'operational',
    floorSqm: 800,
    powerCapacityKw: 300,
    coolingCapacityKw: 280,
    pue: 1.38,
    address: 'Computerweg 14, 1105 BG Amsterdam',
  },
  {
    id: 'fra-01',
    name: 'FRA-01',
    fullName: 'Frankfurt Central',
    city: 'Frankfurt',
    country: 'Germany',
    tier: 4,
    established: 2021,
    status: 'maintenance',
    floorSqm: 600,
    powerCapacityKw: 200,
    coolingCapacityKw: 190,
    pue: 1.32,
    address: 'Hanauer Landstraße 298, 60314 Frankfurt',
  },
];

export const FLOOR_CONFIGS: DatacenterFloorConfig[] = [
  {
    dcId: 'ams-01',
    aisles: [
      { afterRow: 'A', type: 'cold' },
      { afterRow: 'C', type: 'cold' },
    ],
  },
  {
    dcId: 'ams-02',
    aisles: [{ afterRow: 'A', type: 'cold' }],
  },
  {
    dcId: 'fra-01',
    aisles: [],
  },
];

// Helper: other-client rack (no rackId)
function oc(dcId: string, row: string, col: number): RackFloorPosition {
  return { dcId, row, col, ownership: 'other-client' };
}

export const FLOOR_POSITIONS: RackFloorPosition[] = [
  // ── AMS-01 ─────────────────────────────────────────────────────────────────
  // Row A: 4 own racks (R03 has an issue) + 4 other-client
  {
    dcId: 'ams-01',
    rackId: 'ams-01-r01',
    row: 'A',
    col: 1,
    ownership: 'own',
    floorStatus: 'operational',
  },
  {
    dcId: 'ams-01',
    rackId: 'ams-01-r02',
    row: 'A',
    col: 2,
    ownership: 'own',
    floorStatus: 'operational',
  },
  {
    dcId: 'ams-01',
    rackId: 'ams-01-r03',
    row: 'A',
    col: 3,
    ownership: 'own',
    floorStatus: 'issue',
  },
  {
    dcId: 'ams-01',
    rackId: 'ams-01-r04',
    row: 'A',
    col: 4,
    ownership: 'own',
    floorStatus: 'operational',
  },
  oc('ams-01', 'A', 5),
  oc('ams-01', 'A', 6),
  oc('ams-01', 'A', 7),
  oc('ams-01', 'A', 8),
  // Row B: all other-client
  oc('ams-01', 'B', 1),
  oc('ams-01', 'B', 2),
  oc('ams-01', 'B', 3),
  oc('ams-01', 'B', 4),
  oc('ams-01', 'B', 5),
  oc('ams-01', 'B', 6),
  oc('ams-01', 'B', 7),
  oc('ams-01', 'B', 8),
  // Row C: all other-client
  oc('ams-01', 'C', 1),
  oc('ams-01', 'C', 2),
  oc('ams-01', 'C', 3),
  oc('ams-01', 'C', 4),
  oc('ams-01', 'C', 5),
  oc('ams-01', 'C', 6),
  oc('ams-01', 'C', 7),
  oc('ams-01', 'C', 8),
  // Row D: all other-client
  oc('ams-01', 'D', 1),
  oc('ams-01', 'D', 2),
  oc('ams-01', 'D', 3),
  oc('ams-01', 'D', 4),
  oc('ams-01', 'D', 5),
  oc('ams-01', 'D', 6),
  oc('ams-01', 'D', 7),
  oc('ams-01', 'D', 8),

  // ── AMS-02 ─────────────────────────────────────────────────────────────────
  // Row A: 2 own + 4 other-client
  {
    dcId: 'ams-02',
    rackId: 'ams-02-r01',
    row: 'A',
    col: 1,
    ownership: 'own',
    floorStatus: 'operational',
  },
  {
    dcId: 'ams-02',
    rackId: 'ams-02-r02',
    row: 'A',
    col: 2,
    ownership: 'own',
    floorStatus: 'operational',
  },
  oc('ams-02', 'A', 3),
  oc('ams-02', 'A', 4),
  oc('ams-02', 'A', 5),
  oc('ams-02', 'A', 6),
  // Row B: 1 own + 5 other-client
  {
    dcId: 'ams-02',
    rackId: 'ams-02-r03',
    row: 'B',
    col: 1,
    ownership: 'own',
    floorStatus: 'operational',
  },
  oc('ams-02', 'B', 2),
  oc('ams-02', 'B', 3),
  oc('ams-02', 'B', 4),
  oc('ams-02', 'B', 5),
  oc('ams-02', 'B', 6),

  // ── FRA-01 ─────────────────────────────────────────────────────────────────
  // Row A: 2 own + 4 other-client
  {
    dcId: 'fra-01',
    rackId: 'fra-01-r01',
    row: 'A',
    col: 1,
    ownership: 'own',
    floorStatus: 'operational',
  },
  {
    dcId: 'fra-01',
    rackId: 'fra-01-r02',
    row: 'A',
    col: 2,
    ownership: 'own',
    floorStatus: 'operational',
  },
  oc('fra-01', 'A', 3),
  oc('fra-01', 'A', 4),
  oc('fra-01', 'A', 5),
  oc('fra-01', 'A', 6),
];

// TODO(api): RoomService.ListRooms({ site_id })
export const MOCK_ROOMS: Room[] = [
  { id: 'ams-01-room-1', siteId: 'ams-01', name: 'Main Hall', floor: 1 },
  { id: 'ams-01-room-2', siteId: 'ams-01', name: 'Expansion Wing', floor: 1 },
  { id: 'ams-02-room-1', siteId: 'ams-02', name: 'Server Hall A', floor: 0 },
  { id: 'fra-01-room-1', siteId: 'fra-01', name: 'Colocation Hall', floor: 1 },
];

// TODO(api): RackRowService.ListRackRows({ room_id })
export const MOCK_RACK_ROWS: RackRow[] = [
  { id: 'ams-01-row-A', roomId: 'ams-01-room-1', name: 'Row A', positionX: 1, positionY: 1 },
  { id: 'ams-01-row-B', roomId: 'ams-01-room-1', name: 'Row B', positionX: 1, positionY: 2 },
  { id: 'ams-01-row-C', roomId: 'ams-01-room-1', name: 'Row C', positionX: 1, positionY: 3 },
  { id: 'ams-01-row-D', roomId: 'ams-01-room-1', name: 'Row D', positionX: 1, positionY: 4 },
  { id: 'ams-01-row-E', roomId: 'ams-01-room-2', name: 'Row E', positionX: 1, positionY: 1 },
  { id: 'ams-02-row-A', roomId: 'ams-02-room-1', name: 'Row A', positionX: 1, positionY: 1 },
  { id: 'ams-02-row-B', roomId: 'ams-02-room-1', name: 'Row B', positionX: 1, positionY: 2 },
  { id: 'fra-01-row-A', roomId: 'fra-01-room-1', name: 'Row A', positionX: 1, positionY: 1 },
];

// ── Derived helpers ───────────────────────────────────────────────────────────

export function rackFillPct(rackId: string): number {
  const rack = RACKS.find((r) => r.id === rackId);
  if (!rack) return 0;
  const usedU = rack.devices.reduce((sum, d) => sum + d.uSize, 0);
  return Math.round((usedU / rack.totalU) * 100);
}

export function rackDeviceCount(rackId: string): number {
  return RACKS.find((r) => r.id === rackId)?.devices.length ?? 0;
}

export function rackPowerW(rackId: string): number {
  return (
    RACKS.find((r) => r.id === rackId)?.devices.reduce(
      (sum, d) => sum + (d.ipmi?.averageW ?? 0),
      0,
    ) ?? 0
  );
}
