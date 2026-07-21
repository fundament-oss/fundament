// ── Port types ────────────────────────────────────────────────────────────────

export type PortType =
  'console-port' | 'console-server-port' | 'power-port' | 'power-outlet' | 'network-interface';

export interface Port {
  id: string;
  deviceId: string;
  name: string;
  type: PortType;
  description?: string;
}

/**
 * Generates a collision-free, client-only id for a port added in the UI before
 * the server has assigned a real one. The catalog create endpoint ignores this
 * id and the site graph is reloaded afterwards to pick up the server-assigned
 * id. `crypto.randomUUID()` avoids the same-millisecond collisions a
 * timestamp-based id can produce — collisions would corrupt the id-keyed port
 * diff in patch-mapping.
 */
export function newLocalPortId(deviceId: string): string {
  return `p-${deviceId}-${crypto.randomUUID()}`;
}

const CONSOLE_PORT_TYPES = new Set<PortType>(['console-port', 'console-server-port']);
const POWER_PORT_TYPES = new Set<PortType>(['power-port', 'power-outlet']);

export function portsAreCompatible(a: PortType, b: PortType): boolean {
  if (POWER_PORT_TYPES.has(a)) return POWER_PORT_TYPES.has(b);
  if (CONSOLE_PORT_TYPES.has(a)) return CONSOLE_PORT_TYPES.has(b);
  return !POWER_PORT_TYPES.has(b) && !CONSOLE_PORT_TYPES.has(b);
}

// ── Cable types ───────────────────────────────────────────────────────────────

export type CableType =
  | 'cat5e'
  | 'cat6'
  | 'cat6a'
  | 'cat7'
  | 'cat8'
  | 'dac'
  | 'aoc'
  | 'mmf'
  | 'smf'
  | 'power'
  | 'console'
  | 'usb'
  | 'other';

export type CableStatus = 'planned' | 'connected' | 'decommissioned';

export type CableColor =
  | 'dark-grey'
  | 'light-grey'
  | 'red'
  | 'green'
  | 'blue'
  | 'yellow'
  | 'purple'
  | 'orange'
  | 'teal'
  | 'white';

export interface CableSide {
  deviceId: string;
  deviceName: string;
  portId: string;
  portName: string;
  portType: PortType;
}

export interface Cable {
  id: string;
  dcId: string;
  aSide: CableSide;
  bSide: CableSide;
  /** Undefined when the connection has no cable type set (NULL in the DB). */
  type?: CableType;
  /** Undefined when the connection has no status set (NULL in the DB). */
  status?: CableStatus;
  label?: string;
  description?: string;
  color?: CableColor;
  comments?: string;
  length?: number;
}

// ── Color palette ─────────────────────────────────────────────────────────────

export const CABLE_COLOR_HEX: Record<CableColor, string> = {
  'dark-grey': '#374151',
  'light-grey': '#9ca3af',
  red: '#ef4444',
  green: '#22c55e',
  blue: '#3b82f6',
  yellow: '#eab308',
  purple: '#a855f7',
  orange: '#f97316',
  teal: '#14b8a6',
  white: '#f8fafc',
};

/** Default/preset color per cable type, following common industry conventions
 *  (TIA-598 for fiber; vendor/cabling conventions otherwise). */
export const CABLE_TYPE_DEFAULT_COLOR: Record<CableType, CableColor> = {
  cat5e: 'blue',
  cat6: 'green',
  cat6a: 'light-grey',
  cat7: 'purple',
  cat8: 'white',
  dac: 'dark-grey', // twinax DAC — typically black
  aoc: 'teal', // active optical — aqua
  mmf: 'orange', // multimode OM1/OM2 — TIA-598
  smf: 'yellow', // single-mode — TIA-598
  power: 'red',
  console: 'light-grey', // Cisco console cable — light blue/grey
  usb: 'dark-grey',
  other: 'light-grey',
};

export const CABLE_TYPE_LABEL: Record<CableType, string> = {
  cat5e: 'Cat 5e',
  cat6: 'Cat 6',
  cat6a: 'Cat 6a',
  cat7: 'Cat 7',
  cat8: 'Cat 8',
  dac: 'DAC',
  aoc: 'AOC',
  mmf: 'MMF',
  smf: 'SMF',
  power: 'Power',
  console: 'Console',
  usb: 'USB',
  other: 'Other',
};

export const CABLE_STATUS_LABEL: Record<CableStatus, string> = {
  planned: 'Planned',
  connected: 'Connected',
  decommissioned: 'Decommissioned',
};

export const CABLE_STATUS_COLORS: Record<CableStatus, string> = {
  planned: 'bg-amber-100 dark:bg-amber-950 text-amber-700 dark:text-amber-300',
  connected: 'bg-teal-100 dark:bg-teal-950 text-teal-700 dark:text-teal-300',
  decommissioned: 'bg-slate-100 dark:bg-gray-800 text-slate-500 dark:text-gray-400',
};

/** Label shown for an unset (NULL) cable type/status. */
export const UNSPECIFIED_LABEL = 'Unspecified';

export function cableTypeLabel(type: CableType | undefined): string {
  return type ? CABLE_TYPE_LABEL[type] : UNSPECIFIED_LABEL;
}

export function cableStatusLabel(status: CableStatus | undefined): string {
  return status ? CABLE_STATUS_LABEL[status] : UNSPECIFIED_LABEL;
}

export function cableStatusColors(status: CableStatus | undefined): string {
  return status
    ? CABLE_STATUS_COLORS[status]
    : 'bg-slate-100 dark:bg-gray-800 text-slate-500 dark:text-gray-400';
}

// ── Port type labels ──────────────────────────────────────────────────────────

export const PORT_TYPE_LABEL: Record<PortType, string> = {
  'console-port': 'Console Port',
  'console-server-port': 'Console Server Port',
  'power-port': 'Power Port',
  'power-outlet': 'Power Outlet',
  'network-interface': 'Network Interface',
};

export const PORT_TABS: PortType[] = [
  'network-interface',
  'console-port',
  'console-server-port',
  'power-port',
  'power-outlet',
];
