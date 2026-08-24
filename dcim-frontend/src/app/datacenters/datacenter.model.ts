// ── Types ─────────────────────────────────────────────────────────────────────

export type DatacenterStatus = 'operational' | 'degraded' | 'maintenance';

/** `color` for an `nldd-tag` per datacenter status. */
export function statusTagColor(status: DatacenterStatus): string {
  switch (status) {
    case 'operational':
      return 'success';
    case 'degraded':
      return 'warning';
    case 'maintenance':
      return 'lichtblauw';
    default:
      // `satisfies never` fails the build when a status is added without a
      // color here. At runtime an unexpected value off the wire degrades to a
      // plain tag rather than throwing out of the template that renders it.
      status satisfies never;
      return 'neutral';
  }
}

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
