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
