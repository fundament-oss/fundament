// ── Port types ────────────────────────────────────────────────────────────────

export type PortType =
  | 'console-port'
  | 'console-server-port'
  | 'power-port'
  | 'power-outlet'
  | 'network-interface';

export interface Port {
  id: string;
  deviceId: string;
  name: string;
  type: PortType;
  description?: string;
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
  type: CableType;
  status: CableStatus;
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
  planned: 'bg-amber-100 text-amber-700',
  connected: 'bg-teal-100 text-teal-700',
  decommissioned: 'bg-slate-100 text-slate-500',
};

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

// ── Mock CABLES ───────────────────────────────────────────────────────────────

export const MOCK_CABLES: Cable[] = [
  // AMS-01: server-01 → tor-switch-01 (data)
  {
    id: 'cab-001',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-003',
      deviceName: 'server-01',
      portId: 'p-003-01',
      portName: 'eth0',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-001',
      deviceName: 'tor-switch-01',
      portId: 'p-001-01',
      portName: 'Gi0/1',
      portType: 'network-interface',
    },
    type: 'cat6a',
    label: 'srv01-data',
    color: 'blue',
    status: 'connected',
  },
  {
    id: 'cab-002',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-003',
      deviceName: 'server-01',
      portId: 'p-003-02',
      portName: 'eth1',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-001',
      deviceName: 'tor-switch-01',
      portId: 'p-001-02',
      portName: 'Gi0/2',
      portType: 'network-interface',
    },
    type: 'cat6a',
    color: 'blue',
    status: 'connected',
  },
  // AMS-01: server-01 mgmt → patch-panel-01
  {
    id: 'cab-003',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-003',
      deviceName: 'server-01',
      portId: 'p-003-03',
      portName: 'eth2',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-002',
      deviceName: 'patch-panel-01',
      portId: 'p-002-03',
      portName: 'Port 3',
      portType: 'network-interface',
    },
    type: 'cat5e',
    color: 'yellow',
    status: 'connected',
    label: 'srv01-mgmt',
    description: 'Management network via patch panel',
  },
  // AMS-01: server-01 power
  {
    id: 'cab-004',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-003',
      deviceName: 'server-01',
      portId: 'p-003-06',
      portName: 'PSU-A',
      portType: 'power-port',
    },
    bSide: {
      deviceId: 'd-008',
      deviceName: 'pdu-01',
      portId: 'p-008-03',
      portName: 'Outlet 3',
      portType: 'power-outlet',
    },
    type: 'power',
    color: 'orange',
    status: 'connected',
  },
  // AMS-01: tor-switch-01 → spine-switch-01 (uplinks)
  {
    id: 'cab-005',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-001',
      deviceName: 'tor-switch-01',
      portId: 'p-001-05',
      portName: 'Te1/0/1',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-301',
      deviceName: 'spine-switch-01',
      portId: 'p-301-01',
      portName: 'Et1/1',
      portType: 'network-interface',
    },
    type: 'dac',
    label: 'spine-uplink-1',
    color: 'teal',
    status: 'connected',
  },
  {
    id: 'cab-006',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-001',
      deviceName: 'tor-switch-01',
      portId: 'p-001-06',
      portName: 'Te1/0/2',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-301',
      deviceName: 'spine-switch-01',
      portId: 'p-301-02',
      portName: 'Et1/2',
      portType: 'network-interface',
    },
    type: 'dac',
    label: 'spine-uplink-2',
    color: 'teal',
    status: 'connected',
  },
  // AMS-01: leaf-switch-01 → spine-switch-01
  {
    id: 'cab-007',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-101',
      deviceName: 'leaf-switch-01',
      portId: 'p-101-05',
      portName: 'Te0/1',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-301',
      deviceName: 'spine-switch-01',
      portId: 'p-301-03',
      portName: 'Et1/3',
      portType: 'network-interface',
    },
    type: 'dac',
    label: 'leaf-spine-uplink',
    color: 'teal',
    status: 'connected',
  },
  // AMS-01: server-10 → leaf-switch-01
  {
    id: 'cab-008',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-102',
      deviceName: 'server-10',
      portId: 'p-102-01',
      portName: 'eth0',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-101',
      deviceName: 'leaf-switch-01',
      portId: 'p-101-01',
      portName: 'Gi0/1',
      portType: 'network-interface',
    },
    type: 'cat6a',
    color: 'green',
    status: 'connected',
  },
  // AMS-01: server-01 console (planned)
  {
    id: 'cab-009',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-003',
      deviceName: 'server-01',
      portId: 'p-003-05',
      portName: 'COM1',
      portType: 'console-port',
    },
    bSide: {
      deviceId: 'd-002',
      deviceName: 'patch-panel-01',
      portId: 'p-002-01',
      portName: 'Port 1',
      portType: 'network-interface',
    },
    type: 'console',
    color: 'light-grey',
    status: 'planned',
    label: 'srv01-console',
    comments: 'Awaiting console server installation in rack AMS-01-R01',
  },
  // AMS-01: decommissioned old link
  {
    id: 'cab-010',
    dcId: 'ams-01',
    aSide: {
      deviceId: 'd-001',
      deviceName: 'tor-switch-01',
      portId: 'p-001-03',
      portName: 'Gi0/3',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-002',
      deviceName: 'patch-panel-01',
      portId: 'p-002-04',
      portName: 'Port 4',
      portType: 'network-interface',
    },
    type: 'cat6',
    color: 'dark-grey',
    status: 'decommissioned',
    label: 'old-srv02-uplink',
    description: 'Replaced by direct DAC cable in Q4 2024',
  },
  // FRA-01: server-61 → tor-switch-01
  {
    id: 'cab-011',
    dcId: 'fra-01',
    aSide: {
      deviceId: 'd-603',
      deviceName: 'server-61',
      portId: 'p-603-01',
      portName: 'eth0',
      portType: 'network-interface',
    },
    bSide: {
      deviceId: 'd-601',
      deviceName: 'tor-switch-01',
      portId: 'p-601-01',
      portName: 'Gi0/1',
      portType: 'network-interface',
    },
    type: 'cat6a',
    color: 'green',
    status: 'connected',
  },
  // FRA-01: server-61 power (planned — no PDU yet)
  {
    id: 'cab-012',
    dcId: 'fra-01',
    aSide: {
      deviceId: 'd-603',
      deviceName: 'server-61',
      portId: 'p-603-04',
      portName: 'PSU-A',
      portType: 'power-port',
    },
    bSide: {
      deviceId: 'd-601',
      deviceName: 'tor-switch-01',
      portId: 'p-601-05',
      portName: 'PSU-A',
      portType: 'power-port',
    },
    type: 'power',
    color: 'orange',
    status: 'planned',
    description: 'Needs in-rack PDU — using switch PSU as temporary feed',
  },
];
