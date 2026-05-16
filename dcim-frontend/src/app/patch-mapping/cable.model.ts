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
  label?: string;
  type: PortType;
  description?: string;
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
  cat5e: 'Cat5e',
  cat6: 'Cat6',
  cat6a: 'Cat6a',
  cat7: 'Cat7',
  cat8: 'Cat8',
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

// ── Mock DEVICE_PORTS ─────────────────────────────────────────────────────────
// Device IDs match rack.model.ts RACKS

export const DEVICE_PORTS: Record<string, Port[]> = {
  // AMS-01-R01 ─────────────────────────────────────────────────────────────────
  // d-001: tor-switch-01
  'd-001': [
    { id: 'p-001-01', deviceId: 'd-001', name: 'Gi0/1', type: 'network-interface' },
    { id: 'p-001-02', deviceId: 'd-001', name: 'Gi0/2', type: 'network-interface' },
    { id: 'p-001-03', deviceId: 'd-001', name: 'Gi0/3', type: 'network-interface' },
    { id: 'p-001-04', deviceId: 'd-001', name: 'Gi0/4', type: 'network-interface' },
    {
      id: 'p-001-05',
      deviceId: 'd-001',
      name: 'Te1/0/1',
      type: 'network-interface',
      label: 'Uplink 1',
    },
    {
      id: 'p-001-06',
      deviceId: 'd-001',
      name: 'Te1/0/2',
      type: 'network-interface',
      label: 'Uplink 2',
    },
    { id: 'p-001-07', deviceId: 'd-001', name: 'Con0', type: 'console-port' },
    { id: 'p-001-08', deviceId: 'd-001', name: 'PSU-A', type: 'power-port' },
    { id: 'p-001-09', deviceId: 'd-001', name: 'PSU-B', type: 'power-port' },
  ],
  // d-002: patch-panel-01
  'd-002': [
    { id: 'p-002-01', deviceId: 'd-002', name: 'Port 1', type: 'network-interface' },
    { id: 'p-002-02', deviceId: 'd-002', name: 'Port 2', type: 'network-interface' },
    { id: 'p-002-03', deviceId: 'd-002', name: 'Port 3', type: 'network-interface' },
    { id: 'p-002-04', deviceId: 'd-002', name: 'Port 4', type: 'network-interface' },
    { id: 'p-002-05', deviceId: 'd-002', name: 'Port 5', type: 'network-interface' },
    { id: 'p-002-06', deviceId: 'd-002', name: 'Port 6', type: 'network-interface' },
  ],
  // d-003: server-01
  'd-003': [
    { id: 'p-003-01', deviceId: 'd-003', name: 'eth0', type: 'network-interface' },
    { id: 'p-003-02', deviceId: 'd-003', name: 'eth1', type: 'network-interface' },
    {
      id: 'p-003-03',
      deviceId: 'd-003',
      name: 'eth2',
      type: 'network-interface',
      label: 'Management',
    },
    { id: 'p-003-04', deviceId: 'd-003', name: 'eth3', type: 'network-interface' },
    { id: 'p-003-05', deviceId: 'd-003', name: 'COM1', type: 'console-port' },
    { id: 'p-003-06', deviceId: 'd-003', name: 'PSU-A', type: 'power-port' },
    { id: 'p-003-07', deviceId: 'd-003', name: 'PSU-B', type: 'power-port' },
  ],
  // d-005: server-03
  'd-005': [
    { id: 'p-005-01', deviceId: 'd-005', name: 'eth0', type: 'network-interface' },
    { id: 'p-005-02', deviceId: 'd-005', name: 'eth1', type: 'network-interface' },
    { id: 'p-005-03', deviceId: 'd-005', name: 'COM1', type: 'console-port' },
    { id: 'p-005-04', deviceId: 'd-005', name: 'PSU-A', type: 'power-port' },
    { id: 'p-005-05', deviceId: 'd-005', name: 'PSU-B', type: 'power-port' },
  ],
  // d-008: pdu-01
  'd-008': [
    { id: 'p-008-01', deviceId: 'd-008', name: 'Outlet 1', type: 'power-outlet' },
    { id: 'p-008-02', deviceId: 'd-008', name: 'Outlet 2', type: 'power-outlet' },
    { id: 'p-008-03', deviceId: 'd-008', name: 'Outlet 3', type: 'power-outlet' },
    { id: 'p-008-04', deviceId: 'd-008', name: 'Outlet 4', type: 'power-outlet' },
    { id: 'p-008-05', deviceId: 'd-008', name: 'Outlet 5', type: 'power-outlet' },
    { id: 'p-008-06', deviceId: 'd-008', name: 'Outlet 6', type: 'power-outlet' },
    { id: 'p-008-07', deviceId: 'd-008', name: 'Outlet 7', type: 'power-outlet' },
    { id: 'p-008-08', deviceId: 'd-008', name: 'Outlet 8', type: 'power-outlet' },
  ],
  // AMS-01-R02 ─────────────────────────────────────────────────────────────────
  // d-101: leaf-switch-01
  'd-101': [
    { id: 'p-101-01', deviceId: 'd-101', name: 'Gi0/1', type: 'network-interface' },
    { id: 'p-101-02', deviceId: 'd-101', name: 'Gi0/2', type: 'network-interface' },
    { id: 'p-101-03', deviceId: 'd-101', name: 'Gi0/3', type: 'network-interface' },
    { id: 'p-101-04', deviceId: 'd-101', name: 'Gi0/4', type: 'network-interface' },
    {
      id: 'p-101-05',
      deviceId: 'd-101',
      name: 'Te0/1',
      type: 'network-interface',
      label: 'Uplink',
    },
    { id: 'p-101-06', deviceId: 'd-101', name: 'Con0', type: 'console-port' },
    { id: 'p-101-07', deviceId: 'd-101', name: 'PSU-A', type: 'power-port' },
  ],
  // d-102: server-10
  'd-102': [
    { id: 'p-102-01', deviceId: 'd-102', name: 'eth0', type: 'network-interface' },
    { id: 'p-102-02', deviceId: 'd-102', name: 'eth1', type: 'network-interface' },
    { id: 'p-102-03', deviceId: 'd-102', name: 'COM1', type: 'console-port' },
    { id: 'p-102-04', deviceId: 'd-102', name: 'PSU-A', type: 'power-port' },
    { id: 'p-102-05', deviceId: 'd-102', name: 'PSU-B', type: 'power-port' },
  ],
  // d-103: server-11
  'd-103': [
    { id: 'p-103-01', deviceId: 'd-103', name: 'eth0', type: 'network-interface' },
    { id: 'p-103-02', deviceId: 'd-103', name: 'eth1', type: 'network-interface' },
    { id: 'p-103-03', deviceId: 'd-103', name: 'COM1', type: 'console-port' },
    { id: 'p-103-04', deviceId: 'd-103', name: 'PSU-A', type: 'power-port' },
  ],
  // AMS-01-R04 ─────────────────────────────────────────────────────────────────
  // d-301: spine-switch-01
  'd-301': [
    {
      id: 'p-301-01',
      deviceId: 'd-301',
      name: 'Et1/1',
      type: 'network-interface',
      label: 'Downlink 1',
    },
    {
      id: 'p-301-02',
      deviceId: 'd-301',
      name: 'Et1/2',
      type: 'network-interface',
      label: 'Downlink 2',
    },
    {
      id: 'p-301-03',
      deviceId: 'd-301',
      name: 'Et1/3',
      type: 'network-interface',
      label: 'Downlink 3',
    },
    { id: 'p-301-04', deviceId: 'd-301', name: 'Et1/4', type: 'network-interface' },
    { id: 'p-301-05', deviceId: 'd-301', name: 'Con0', type: 'console-port' },
    { id: 'p-301-06', deviceId: 'd-301', name: 'PSU-A', type: 'power-port' },
    { id: 'p-301-07', deviceId: 'd-301', name: 'PSU-B', type: 'power-port' },
  ],
  // FRA-01-R01 ─────────────────────────────────────────────────────────────────
  // d-601: tor-switch-01
  'd-601': [
    { id: 'p-601-01', deviceId: 'd-601', name: 'Gi0/1', type: 'network-interface' },
    { id: 'p-601-02', deviceId: 'd-601', name: 'Gi0/2', type: 'network-interface' },
    {
      id: 'p-601-03',
      deviceId: 'd-601',
      name: 'Te0/1',
      type: 'network-interface',
      label: 'Uplink',
    },
    { id: 'p-601-04', deviceId: 'd-601', name: 'Con0', type: 'console-port' },
    { id: 'p-601-05', deviceId: 'd-601', name: 'PSU-A', type: 'power-port' },
  ],
  // d-603: server-61
  'd-603': [
    { id: 'p-603-01', deviceId: 'd-603', name: 'eth0', type: 'network-interface' },
    { id: 'p-603-02', deviceId: 'd-603', name: 'eth1', type: 'network-interface' },
    { id: 'p-603-03', deviceId: 'd-603', name: 'COM1', type: 'console-port' },
    { id: 'p-603-04', deviceId: 'd-603', name: 'PSU-A', type: 'power-port' },
    { id: 'p-603-05', deviceId: 'd-603', name: 'PSU-B', type: 'power-port' },
  ],
};

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
