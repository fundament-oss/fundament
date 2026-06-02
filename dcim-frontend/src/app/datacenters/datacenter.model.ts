// ── Types ─────────────────────────────────────────────────────────────────────

export type DatacenterStatus = 'operational' | 'degraded' | 'maintenance';

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

// ── View model shared between DatacentersComponent and IsometricCanvasComponent ──

export interface RackCell {
  rackId: string;
  rackName: string;
  row: string;
  col: number;
  fillPct: number;
  deviceCount: number;
  powerW: number;
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
  positionInRow: number;
}
