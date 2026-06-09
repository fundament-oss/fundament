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

export interface DeviceRoleColor {
  bg: string;
  border: string;
  text: string;
}

export const DEVICE_ROLE_COLORS: Record<LogicalDeviceRole, DeviceRoleColor> = {
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

// Dark-mode counterparts: deep tinted surface (~950), mid border (~700), light
// text (~300), keeping each role's hue from the light palette above.
export const DEVICE_ROLE_COLORS_DARK: Record<LogicalDeviceRole, DeviceRoleColor> = {
  Compute: { bg: '#172554', border: '#1d4ed8', text: '#93c5fd' },
  ToR: { bg: '#052e16', border: '#15803d', text: '#86efac' },
  Spine: { bg: '#2e1065', border: '#6d28d9', text: '#c4b5fd' },
  Core: { bg: '#4a044e', border: '#a21caf', text: '#f0abfc' },
  PDU: { bg: '#451a03', border: '#b45309', text: '#fcd34d' },
  'Patch Panel': { bg: '#0f172a', border: '#475569', text: '#cbd5e1' },
  Storage: { bg: '#431407', border: '#c2410c', text: '#fdba74' },
  Firewall: { bg: '#450a0a', border: '#b91c1c', text: '#fca5a5' },
  'Load Balancer': { bg: '#083344', border: '#0e7490', text: '#67e8f9' },
  'Console Server': { bg: '#0f172a', border: '#475569', text: '#cbd5e1' },
  'Cable Manager': { bg: '#0f172a', border: '#475569', text: '#cbd5e1' },
  Adapter: { bg: '#0f172a', border: '#475569', text: '#cbd5e1' },
};

const DEVICE_ROLE_FALLBACK: DeviceRoleColor = { bg: '#f8fafc', border: '#94a3b8', text: '#475569' };
const DEVICE_ROLE_FALLBACK_DARK: DeviceRoleColor = {
  bg: '#0f172a',
  border: '#475569',
  text: '#cbd5e1',
};

export function deviceRoleColors(role: LogicalDeviceRole, isDark: boolean): DeviceRoleColor {
  if (isDark) return DEVICE_ROLE_COLORS_DARK[role] ?? DEVICE_ROLE_FALLBACK_DARK;
  return DEVICE_ROLE_COLORS[role] ?? DEVICE_ROLE_FALLBACK;
}

// Badge classes per design status, shared by the designs list and design detail.
export const LOGICAL_DESIGN_STATUS_BADGE_CLASS: Record<LogicalDesignStatus, string> = {
  draft: 'bg-slate-100 dark:bg-gray-800 text-slate-600 dark:text-gray-300',
  active: 'bg-green-50 dark:bg-green-950 text-green-700 dark:text-green-300',
  archived: 'bg-amber-50 dark:bg-amber-950 text-amber-700 dark:text-amber-300',
};
