// ── Types ────────────────────────────────────────────────────────────────────

export type DeviceState = 'allocated' | 'free' | 'offline' | 'locked' | 'reserved';
export type DeviceType = 'machine' | 'switch' | 'patch' | 'pdu';

export interface RackDevice {
  id: string;
  type: DeviceType;
  uStart: number;
  uSize: number;
  name: string;
  state: DeviceState;
  liveliness?: 'Alive' | 'Dead' | 'Unknown';
  allocation?: { project: string; role: string; hostname: string; image: string };
  hardware?: { cpu_cores: number; memory: number; disks: number; nics: number };
  ipmi?: { address: string; powerstate: 'ON' | 'OFF'; averageW: number };
  model?: string;
  assetTag?: string;
  warrantyExpiry?: string;
  lastMaintenance?: string;
}

export interface Rack {
  id: string;
  name: string;
  dcId: string;
  totalU: number;
  devices: RackDevice[];
}

export interface RackSlot {
  u: number;
  device: RackDevice | null;
  isFirst: boolean;
}

export type ConnectionType = 'network' | 'power' | 'management' | 'storage';
export type ConnectionStatus = 'up' | 'down' | 'unknown';
