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

/** A rack shown in the datacenter detail page's racks list. */
export interface DatacenterRack {
  id: string;
  rowId: string;
  name: string;
  totalU: number;
}

export interface DatacenterFloorConfig {
  dcId: string;
  aisles: AisleDefinition[];
}

// ── Mock data ─────────────────────────────────────────────────────────────────

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
