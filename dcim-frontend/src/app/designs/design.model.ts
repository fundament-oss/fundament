// ── Types ─────────────────────────────────────────────────────────────────────

export type LogicalDesignStatus = 'draft' | 'active' | 'archived';

export type LogicalDeviceRole =
  | 'Compute'
  | 'ToR'
  | 'Spine'
  | 'Core'
  | 'PDU'
  | 'Patch Panel'
  | 'Storage'
  | 'Firewall'
  | 'Load Balancer'
  | 'Console Server'
  | 'Cable Manager'
  | 'Adapter';

export type LogicalConnectionType = 'network' | 'power' | 'console';

export interface LogicalDesign {
  id: string;
  name: string;
  version: number;
  status: LogicalDesignStatus;
  created: string;
}

export interface LogicalDevice {
  id: string;
  designId: string;
  name: string;
  role: LogicalDeviceRole;
  deviceCatalogId?: string;
}

export interface LogicalConnection {
  id: string;
  designId: string;
  sourceDeviceId: string;
  sourcePortRole: string;
  targetDeviceId: string;
  targetPortRole: string;
  connectionType: LogicalConnectionType;
}

export interface LogicalDeviceLayout {
  deviceId: string;
  x: number;
  y: number;
}

// ── Mock data ─────────────────────────────────────────────────────────────────

// TODO(api): LogicalDesignService.ListLogicalDesigns({})
export const MOCK_DESIGNS: LogicalDesign[] = [
  {
    id: 'design-001',
    name: 'AMS-01 Compute Cluster v1',
    version: 1,
    status: 'active',
    created: '2025-01-15',
  },
  {
    id: 'design-002',
    name: 'AMS-01 Network Spine',
    version: 2,
    status: 'active',
    created: '2025-02-03',
  },
  {
    id: 'design-003',
    name: 'AMS-02 Expansion Draft',
    version: 1,
    status: 'draft',
    created: '2025-03-20',
  },
  {
    id: 'design-004',
    name: 'FRA-01 Colocation Layout',
    version: 1,
    status: 'archived',
    created: '2024-11-10',
  },
];

// TODO(api): LogicalDeviceService.ListLogicalDevices({ design_id })
export const MOCK_LOGICAL_DEVICES: LogicalDevice[] = [
  { id: 'dev-001', designId: 'design-001', name: 'Spine-1', role: 'Spine' },
  { id: 'dev-002', designId: 'design-001', name: 'ToR-A1', role: 'ToR' },
  { id: 'dev-003', designId: 'design-001', name: 'ToR-A2', role: 'ToR' },
  { id: 'dev-004', designId: 'design-001', name: 'Compute-01', role: 'Compute' },
  { id: 'dev-005', designId: 'design-001', name: 'Compute-02', role: 'Compute' },
  { id: 'dev-006', designId: 'design-001', name: 'Compute-03', role: 'Compute' },
  { id: 'dev-007', designId: 'design-001', name: 'PDU-A', role: 'PDU' },
  { id: 'dev-008', designId: 'design-002', name: 'Core-1', role: 'Core' },
  { id: 'dev-009', designId: 'design-002', name: 'Core-2', role: 'Core' },
  { id: 'dev-010', designId: 'design-002', name: 'Spine-1', role: 'Spine' },
  { id: 'dev-011', designId: 'design-002', name: 'Spine-2', role: 'Spine' },
  { id: 'dev-012', designId: 'design-002', name: 'Firewall-1', role: 'Firewall' },
];

// TODO(api): LogicalConnectionService.ListLogicalConnections({ design_id })
export const MOCK_LOGICAL_CONNECTIONS: LogicalConnection[] = [
  {
    id: 'conn-001',
    designId: 'design-001',
    sourceDeviceId: 'dev-001',
    sourcePortRole: 'downlink-1',
    targetDeviceId: 'dev-002',
    targetPortRole: 'uplink',
    connectionType: 'network',
  },
  {
    id: 'conn-002',
    designId: 'design-001',
    sourceDeviceId: 'dev-001',
    sourcePortRole: 'downlink-2',
    targetDeviceId: 'dev-003',
    targetPortRole: 'uplink',
    connectionType: 'network',
  },
  {
    id: 'conn-003',
    designId: 'design-001',
    sourceDeviceId: 'dev-002',
    sourcePortRole: 'server-1',
    targetDeviceId: 'dev-004',
    targetPortRole: 'nic-0',
    connectionType: 'network',
  },
  {
    id: 'conn-004',
    designId: 'design-001',
    sourceDeviceId: 'dev-002',
    sourcePortRole: 'server-2',
    targetDeviceId: 'dev-005',
    targetPortRole: 'nic-0',
    connectionType: 'network',
  },
  {
    id: 'conn-005',
    designId: 'design-001',
    sourceDeviceId: 'dev-003',
    sourcePortRole: 'server-1',
    targetDeviceId: 'dev-006',
    targetPortRole: 'nic-0',
    connectionType: 'network',
  },
  {
    id: 'conn-006',
    designId: 'design-001',
    sourceDeviceId: 'dev-007',
    sourcePortRole: 'outlet-1',
    targetDeviceId: 'dev-004',
    targetPortRole: 'psu-0',
    connectionType: 'power',
  },
  {
    id: 'conn-007',
    designId: 'design-002',
    sourceDeviceId: 'dev-008',
    sourcePortRole: 'peer',
    targetDeviceId: 'dev-009',
    targetPortRole: 'peer',
    connectionType: 'network',
  },
  {
    id: 'conn-008',
    designId: 'design-002',
    sourceDeviceId: 'dev-008',
    sourcePortRole: 'downlink-1',
    targetDeviceId: 'dev-010',
    targetPortRole: 'uplink',
    connectionType: 'network',
  },
  {
    id: 'conn-009',
    designId: 'design-002',
    sourceDeviceId: 'dev-012',
    sourcePortRole: 'inside',
    targetDeviceId: 'dev-010',
    targetPortRole: 'downlink-1',
    connectionType: 'network',
  },
];

// TODO(api): LogicalDeviceLayoutService.GetLogicalDeviceLayout({ device_id })
export const MOCK_DEVICE_LAYOUTS: LogicalDeviceLayout[] = [
  { deviceId: 'dev-001', x: 300, y: 60 },
  { deviceId: 'dev-002', x: 150, y: 200 },
  { deviceId: 'dev-003', x: 450, y: 200 },
  { deviceId: 'dev-004', x: 80, y: 360 },
  { deviceId: 'dev-005', x: 220, y: 360 },
  { deviceId: 'dev-006', x: 380, y: 360 },
  { deviceId: 'dev-007', x: 80, y: 500 },
  { deviceId: 'dev-008', x: 200, y: 60 },
  { deviceId: 'dev-009', x: 400, y: 60 },
  { deviceId: 'dev-010', x: 150, y: 220 },
  { deviceId: 'dev-011', x: 350, y: 220 },
  { deviceId: 'dev-012', x: 80, y: 380 },
];

export const DEVICE_ROLE_COLORS: Record<
  LogicalDeviceRole,
  { bg: string; border: string; text: string }
> = {
  Compute: { bg: '#eff6ff', border: '#93c5fd', text: '#1d4ed8' },
  ToR: { bg: '#f0fdf4', border: '#86efac', text: '#15803d' },
  Spine: { bg: '#faf5ff', border: '#c4b5fd', text: '#7c3aed' },
  Core: { bg: '#fdf4ff', border: '#e879f9', text: '#a21caf' },
  PDU: { bg: '#fffbeb', border: '#fcd34d', text: '#b45309' },
  'Patch Panel': { bg: '#f8fafc', border: '#94a3b8', text: '#475569' },
  Storage: { bg: '#fff7ed', border: '#fdba74', text: '#c2410c' },
  Firewall: { bg: '#fef2f2', border: '#fca5a5', text: '#dc2626' },
  'Load Balancer': { bg: '#ecfeff', border: '#67e8f9', text: '#0e7490' },
  'Console Server': { bg: '#f8fafc', border: '#94a3b8', text: '#475569' },
  'Cable Manager': { bg: '#f8fafc', border: '#94a3b8', text: '#475569' },
  Adapter: { bg: '#f8fafc', border: '#94a3b8', text: '#475569' },
};
