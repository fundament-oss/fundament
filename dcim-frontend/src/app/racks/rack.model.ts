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

export interface Partition {
  id: string;
  label: string;
}

export interface RackSlot {
  u: number;
  device: RackDevice | null;
  isFirst: boolean;
}

export interface DeviceComment {
  author: string;
  initials: string;
  daysAgo: number;
  content: string;
}

export interface DeviceNotes {
  description: string;
  comments: DeviceComment[];
}

export type DeviceHistoryAction =
  | 'state-change'
  | 'maintenance'
  | 'allocation'
  | 'hardware'
  | 'created';

export interface DeviceHistoryEntry {
  action: DeviceHistoryAction;
  description: string;
  user: string;
  daysAgo: number;
}

export type ConnectionType = 'network' | 'power' | 'management' | 'storage';
export type ConnectionStatus = 'up' | 'down' | 'unknown';

export interface DeviceConnection {
  localPort: string;
  remoteDeviceId: string;
  remotePort: string;
  type: ConnectionType;
  speed?: string;
  status: ConnectionStatus;
}

// ── Mock data ─────────────────────────────────────────────────────────────────

export const PARTITIONS: Partition[] = [
  { id: 'ams-01', label: 'AMS-01' },
  { id: 'ams-02', label: 'AMS-02' },
  { id: 'fra-01', label: 'FRA-01' },
];

export const RACKS: Rack[] = [
  // ─── AMS-01 ───────────────────────────────────────────────────────────────
  {
    id: 'ams-01-r01',
    name: 'AMS-01-R01',
    dcId: 'ams-01',
    totalU: 42,
    devices: [
      {
        id: 'd-001',
        type: 'switch',
        uStart: 41,
        uSize: 2,
        name: 'tor-switch-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.1.1', powerstate: 'ON', averageW: 120 },
        allocation: {
          project: 'infra',
          role: 'tor-switch',
          hostname: 'tor-switch-01.ams-01',
          image: 'cumulus-4.2',
        },
        model: 'Cisco Catalyst 9300-48P',
        assetTag: 'SW-001',
      },
      {
        id: 'd-002',
        type: 'patch',
        uStart: 40,
        uSize: 1,
        name: 'patch-panel-01',
        state: 'allocated',
        model: 'Panduit DP24888WH',
        assetTag: 'PP-001',
      },
      {
        id: 'd-003',
        type: 'machine',
        uStart: 37,
        uSize: 2,
        name: 'server-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.1.10', powerstate: 'ON', averageW: 350 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-01.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 4 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-003',
        warrantyExpiry: '2026-12-31',
        lastMaintenance: '2025-09-15',
      },
      {
        id: 'd-004',
        type: 'machine',
        uStart: 36,
        uSize: 1,
        name: 'server-02',
        state: 'offline',
        liveliness: 'Dead',
        ipmi: { address: '10.0.1.11', powerstate: 'OFF', averageW: 0 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-004',
      },
      {
        id: 'd-005',
        type: 'machine',
        uStart: 34,
        uSize: 2,
        name: 'server-03',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.1.12', powerstate: 'ON', averageW: 320 },
        allocation: {
          project: 'team-beta',
          role: 'storage',
          hostname: 'server-03.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 8, nics: 2 },
        model: 'Dell PowerEdge R650',
        assetTag: 'SRV-005',
      },
      {
        id: 'd-006',
        type: 'machine',
        uStart: 33,
        uSize: 1,
        name: 'server-04',
        state: 'reserved',
        ipmi: { address: '10.0.1.13', powerstate: 'OFF', averageW: 0 },
        model: 'Supermicro SYS-620P',
        assetTag: 'SRV-006',
      },
      {
        id: 'd-007',
        type: 'machine',
        uStart: 31,
        uSize: 2,
        name: 'server-05',
        state: 'locked',
        liveliness: 'Alive',
        ipmi: { address: '10.0.1.14', powerstate: 'ON', averageW: 280 },
        allocation: {
          project: 'compliance',
          role: 'audit',
          hostname: 'server-05.ams-01',
          image: 'rhel-9',
        },
        hardware: { cpu_cores: 16, memory: 64, disks: 2, nics: 2 },
        model: 'Lenovo ThinkSystem SR650',
        assetTag: 'SRV-007',
      },
      {
        id: 'd-008',
        type: 'pdu',
        uStart: 3,
        uSize: 1,
        name: 'pdu-01',
        state: 'allocated',
        model: 'APC AP8858',
        assetTag: 'PDU-001',
      },
    ],
  },
  {
    id: 'ams-01-r02',
    name: 'AMS-01-R02',
    dcId: 'ams-01',
    totalU: 42,
    devices: [
      {
        id: 'd-101',
        type: 'switch',
        uStart: 42,
        uSize: 1,
        name: 'leaf-switch-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.2.1', powerstate: 'ON', averageW: 90 },
        allocation: {
          project: 'infra',
          role: 'leaf-switch',
          hostname: 'leaf-sw-01.ams-01',
          image: 'sonic',
        },
        model: 'Juniper EX4300-48T',
        assetTag: 'SW-101',
      },
      {
        id: 'd-102',
        type: 'machine',
        uStart: 39,
        uSize: 2,
        name: 'server-10',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.2.10', powerstate: 'ON', averageW: 400 },
        allocation: {
          project: 'team-gamma',
          role: 'k8s-worker',
          hostname: 'server-10.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 128, memory: 512, disks: 2, nics: 4 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-102',
      },
      {
        id: 'd-103',
        type: 'machine',
        uStart: 37,
        uSize: 2,
        name: 'server-11',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.2.11', powerstate: 'ON', averageW: 395 },
        allocation: {
          project: 'team-gamma',
          role: 'k8s-worker',
          hostname: 'server-11.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 128, memory: 512, disks: 2, nics: 4 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-103',
      },
      {
        id: 'd-104',
        type: 'machine',
        uStart: 35,
        uSize: 2,
        name: 'server-12',
        state: 'free',
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-104',
      },
    ],
  },
  {
    id: 'ams-01-r03',
    name: 'AMS-01-R03',
    dcId: 'ams-01',
    totalU: 42,
    devices: [
      {
        id: 'd-201',
        type: 'machine',
        uStart: 41,
        uSize: 2,
        name: 'server-20',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.10', powerstate: 'ON', averageW: 300 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-20.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-201',
      },
      {
        id: 'd-202',
        type: 'machine',
        uStart: 39,
        uSize: 2,
        name: 'server-21',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.11', powerstate: 'ON', averageW: 310 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-21.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-202',
      },
      {
        id: 'd-203',
        type: 'machine',
        uStart: 37,
        uSize: 2,
        name: 'server-22',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.12', powerstate: 'ON', averageW: 295 },
        allocation: {
          project: 'team-beta',
          role: 'compute',
          hostname: 'server-22.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-203',
      },
      {
        id: 'd-204',
        type: 'machine',
        uStart: 35,
        uSize: 2,
        name: 'server-23',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.13', powerstate: 'ON', averageW: 305 },
        allocation: {
          project: 'team-beta',
          role: 'compute',
          hostname: 'server-23.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-204',
      },
      {
        id: 'd-205',
        type: 'machine',
        uStart: 33,
        uSize: 2,
        name: 'server-24',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.14', powerstate: 'ON', averageW: 320 },
        allocation: {
          project: 'team-gamma',
          role: 'compute',
          hostname: 'server-24.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Supermicro SYS-620P',
        assetTag: 'SRV-205',
      },
      {
        id: 'd-206',
        type: 'machine',
        uStart: 31,
        uSize: 2,
        name: 'server-25',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.15', powerstate: 'ON', averageW: 315 },
        allocation: {
          project: 'team-gamma',
          role: 'compute',
          hostname: 'server-25.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Supermicro SYS-620P',
        assetTag: 'SRV-206',
      },
      {
        id: 'd-207',
        type: 'machine',
        uStart: 29,
        uSize: 2,
        name: 'server-26',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.16', powerstate: 'ON', averageW: 290 },
        allocation: {
          project: 'team-alpha',
          role: 'storage',
          hostname: 'server-26.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 8, nics: 2 },
        model: 'Dell PowerEdge R740xd',
        assetTag: 'SRV-207',
      },
      {
        id: 'd-208',
        type: 'machine',
        uStart: 27,
        uSize: 2,
        name: 'server-28',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.17', powerstate: 'ON', averageW: 280 },
        allocation: {
          project: 'team-alpha',
          role: 'storage',
          hostname: 'server-27.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 8, nics: 2 },
        model: 'Dell PowerEdge R740xd',
        assetTag: 'SRV-208',
      },
      {
        id: 'd-209',
        type: 'machine',
        uStart: 25,
        uSize: 2,
        name: 'server-28',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.18', powerstate: 'ON', averageW: 270 },
        allocation: {
          project: 'team-beta',
          role: 'storage',
          hostname: 'server-28.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 8, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-209',
      },
      {
        id: 'd-210',
        type: 'machine',
        uStart: 23,
        uSize: 2,
        name: 'server-29',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.19', powerstate: 'ON', averageW: 285 },
        allocation: {
          project: 'team-beta',
          role: 'storage',
          hostname: 'server-29.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 8, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-210',
      },
      {
        id: 'd-211',
        type: 'machine',
        uStart: 21,
        uSize: 2,
        name: 'server-30',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.20', powerstate: 'ON', averageW: 300 },
        allocation: {
          project: 'team-gamma',
          role: 'network',
          hostname: 'server-30.ams-01',
          image: 'rhel-9',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 4, nics: 4 },
        model: 'Dell PowerEdge R650',
        assetTag: 'SRV-211',
      },
      {
        id: 'd-212',
        type: 'machine',
        uStart: 19,
        uSize: 2,
        name: 'server-31',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.21', powerstate: 'ON', averageW: 310 },
        allocation: {
          project: 'team-gamma',
          role: 'network',
          hostname: 'server-31.ams-01',
          image: 'rhel-9',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 4, nics: 4 },
        model: 'Dell PowerEdge R650',
        assetTag: 'SRV-212',
      },
      {
        id: 'd-213',
        type: 'machine',
        uStart: 17,
        uSize: 2,
        name: 'server-32',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.22', powerstate: 'ON', averageW: 295 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-32.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-213',
      },
      {
        id: 'd-214',
        type: 'machine',
        uStart: 15,
        uSize: 2,
        name: 'server-33',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.23', powerstate: 'ON', averageW: 305 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-33.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-214',
      },
      {
        id: 'd-215',
        type: 'machine',
        uStart: 13,
        uSize: 2,
        name: 'server-34',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.24', powerstate: 'ON', averageW: 320 },
        allocation: {
          project: 'team-beta',
          role: 'compute',
          hostname: 'server-34.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-215',
      },
      {
        id: 'd-216',
        type: 'machine',
        uStart: 11,
        uSize: 2,
        name: 'server-35',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.25', powerstate: 'ON', averageW: 315 },
        allocation: {
          project: 'team-beta',
          role: 'compute',
          hostname: 'server-35.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-216',
      },
      {
        id: 'd-217',
        type: 'machine',
        uStart: 9,
        uSize: 2,
        name: 'server-36',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.26', powerstate: 'ON', averageW: 290 },
        allocation: {
          project: 'team-gamma',
          role: 'compute',
          hostname: 'server-36.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Supermicro SYS-620P',
        assetTag: 'SRV-217',
      },
      {
        id: 'd-218',
        type: 'machine',
        uStart: 7,
        uSize: 2,
        name: 'server-37',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.27', powerstate: 'ON', averageW: 280 },
        allocation: {
          project: 'team-gamma',
          role: 'compute',
          hostname: 'server-37.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Supermicro SYS-620P',
        assetTag: 'SRV-218',
      },
      {
        id: 'd-219',
        type: 'machine',
        uStart: 5,
        uSize: 2,
        name: 'server-38',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.28', powerstate: 'ON', averageW: 270 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-38.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-219',
      },
      {
        id: 'd-220',
        type: 'machine',
        uStart: 3,
        uSize: 2,
        name: 'server-39',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.29', powerstate: 'ON', averageW: 285 },
        allocation: {
          project: 'team-alpha',
          role: 'compute',
          hostname: 'server-39.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-220',
      },
      {
        id: 'd-221',
        type: 'machine',
        uStart: 1,
        uSize: 2,
        name: 'server-40',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.3.30', powerstate: 'ON', averageW: 300 },
        allocation: {
          project: 'team-beta',
          role: 'compute',
          hostname: 'server-40.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 2 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-221',
      },
    ],
  },
  {
    id: 'ams-01-r04',
    name: 'AMS-01-R04',
    dcId: 'ams-01',
    totalU: 42,
    devices: [
      {
        id: 'd-301',
        type: 'switch',
        uStart: 41,
        uSize: 2,
        name: 'spine-switch-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.4.1', powerstate: 'ON', averageW: 180 },
        allocation: {
          project: 'infra',
          role: 'spine-switch',
          hostname: 'spine-sw-01.ams-01',
          image: 'arista-eos',
        },
        model: 'Arista 7050CX3-32S',
        assetTag: 'SW-301',
      },
      {
        id: 'd-302',
        type: 'machine',
        uStart: 39,
        uSize: 1,
        name: 'mgmt-server-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.0.4.10', powerstate: 'ON', averageW: 150 },
        allocation: {
          project: 'infra',
          role: 'management',
          hostname: 'mgmt-01.ams-01',
          image: 'debian-11',
        },
        hardware: { cpu_cores: 8, memory: 32, disks: 2, nics: 2 },
        model: 'HP ProLiant DL360 Gen10',
        assetTag: 'SRV-302',
      },
      {
        id: 'd-303',
        type: 'machine',
        uStart: 37,
        uSize: 1,
        name: 'bastion-01',
        state: 'locked',
        liveliness: 'Alive',
        ipmi: { address: '10.0.4.11', powerstate: 'ON', averageW: 120 },
        allocation: {
          project: 'security',
          role: 'bastion',
          hostname: 'bastion-01.ams-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 4, memory: 16, disks: 1, nics: 2 },
        model: 'Dell PowerEdge R340',
        assetTag: 'SRV-303',
      },
    ],
  },
  // ─── AMS-02 ───────────────────────────────────────────────────────────────
  {
    id: 'ams-02-r01',
    name: 'AMS-02-R01',
    dcId: 'ams-02',
    totalU: 42,
    devices: [
      {
        id: 'd-401',
        type: 'switch',
        uStart: 41,
        uSize: 2,
        name: 'tor-switch-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.1.1.1', powerstate: 'ON', averageW: 110 },
        allocation: {
          project: 'infra',
          role: 'tor-switch',
          hostname: 'tor-sw-01.ams-02',
          image: 'cumulus-4.2',
        },
        model: 'Cisco Catalyst 9300-48P',
        assetTag: 'SW-401',
      },
      {
        id: 'd-402',
        type: 'machine',
        uStart: 38,
        uSize: 2,
        name: 'server-40',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.1.1.10', powerstate: 'ON', averageW: 380 },
        allocation: {
          project: 'team-delta',
          role: 'compute',
          hostname: 'server-40.ams-02',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 4 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-402',
      },
      {
        id: 'd-403',
        type: 'machine',
        uStart: 35,
        uSize: 2,
        name: 'server-41',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.1.1.11', powerstate: 'ON', averageW: 370 },
        allocation: {
          project: 'team-delta',
          role: 'compute',
          hostname: 'server-41.ams-02',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 64, memory: 256, disks: 4, nics: 4 },
        model: 'Dell PowerEdge R750',
        assetTag: 'SRV-403',
      },
    ],
  },
  {
    id: 'ams-02-r02',
    name: 'AMS-02-R02',
    dcId: 'ams-02',
    totalU: 42,
    devices: [
      {
        id: 'd-501',
        type: 'machine',
        uStart: 39,
        uSize: 4,
        name: 'storage-server-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.1.2.10', powerstate: 'ON', averageW: 600 },
        allocation: {
          project: 'storage',
          role: 'nas',
          hostname: 'nas-01.ams-02',
          image: 'truenas-13',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 24, nics: 4 },
        model: 'NetApp AFF A800',
        assetTag: 'SRV-501',
      },
      {
        id: 'd-502',
        type: 'machine',
        uStart: 36,
        uSize: 2,
        name: 'server-50',
        state: 'reserved',
        ipmi: { address: '10.1.2.20', powerstate: 'OFF', averageW: 0 },
        model: 'HP ProLiant DL380 Gen10',
        assetTag: 'SRV-502',
      },
    ],
  },
  {
    id: 'ams-02-r03',
    name: 'AMS-02-R03',
    dcId: 'ams-02',
    totalU: 42,
    devices: [],
  },
  // ─── FRA-01 ───────────────────────────────────────────────────────────────
  {
    id: 'fra-01-r01',
    name: 'FRA-01-R01',
    dcId: 'fra-01',
    totalU: 42,
    devices: [
      {
        id: 'd-601',
        type: 'switch',
        uStart: 42,
        uSize: 1,
        name: 'tor-switch-01',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.2.1.1', powerstate: 'ON', averageW: 90 },
        allocation: {
          project: 'infra',
          role: 'tor-switch',
          hostname: 'tor-sw-01.fra-01',
          image: 'sonic',
        },
        model: 'Juniper EX4300-48T',
        assetTag: 'SW-601',
      },
      {
        id: 'd-602',
        type: 'machine',
        uStart: 40,
        uSize: 1,
        name: 'server-60',
        state: 'reserved',
        ipmi: { address: '10.2.1.10', powerstate: 'OFF', averageW: 0 },
        model: 'Dell PowerEdge R650',
        assetTag: 'SRV-602',
      },
      {
        id: 'd-603',
        type: 'machine',
        uStart: 37,
        uSize: 2,
        name: 'server-61',
        state: 'allocated',
        liveliness: 'Alive',
        ipmi: { address: '10.2.1.11', powerstate: 'ON', averageW: 250 },
        allocation: {
          project: 'fra-core',
          role: 'compute',
          hostname: 'server-61.fra-01',
          image: 'ubuntu-22.04',
        },
        hardware: { cpu_cores: 32, memory: 128, disks: 2, nics: 2 },
        model: 'Dell PowerEdge R650',
        assetTag: 'SRV-603',
      },
    ],
  },
  {
    id: 'fra-01-r02',
    name: 'FRA-01-R02',
    dcId: 'fra-01',
    totalU: 42,
    devices: [],
  },
];

export const DEVICE_NOTES: Record<string, DeviceNotes> = {
  'd-003': {
    description:
      'Primary compute node for team-alpha. RAM upgraded to 256 GB in Q1 2025. Runs VMware ESXi 8.0 with 12 VMs.',
    comments: [
      {
        author: 'Alex van Dijk',
        initials: 'AV',
        daysAgo: 2,
        content:
          'Noticed elevated temperatures on CPU0 during last check. Thermal paste reapplication scheduled for next maintenance window.',
      },
      {
        author: 'Sara Müller',
        initials: 'SM',
        daysAgo: 14,
        content: 'Firmware updated to latest version. All checks passed. No issues found.',
      },
      {
        author: 'Tom de Graaf',
        initials: 'TG',
        daysAgo: 42,
        content: 'Moved from team-beta allocation to team-alpha. Config updated accordingly.',
      },
    ],
  },
  'd-001': {
    description:
      'Top-of-rack switch for AMS-01-R01. Provides 48x1G downlinks and 4x10G uplinks to spine layer.',
    comments: [
      {
        author: 'Sara Müller',
        initials: 'SM',
        daysAgo: 5,
        content:
          'VLAN configuration updated to include new DMZ segment. Change approved in ticket #4821.',
      },
      {
        author: 'Alex van Dijk',
        initials: 'AV',
        daysAgo: 60,
        content:
          'IOS-XE upgraded from 17.3.5 to 17.6.2. Rollback tested successfully before upgrade.',
      },
    ],
  },
  'd-004': {
    description:
      'Server currently offline after PSU failure. Replacement PSU on order (ETA: 2025-11-10). Do not reallocate.',
    comments: [
      {
        author: 'Tom de Graaf',
        initials: 'TG',
        daysAgo: 1,
        content:
          'Confirmed PSU-A failed. PSU-B also showing warnings. Ordered two replacements. Server marked offline.',
      },
      {
        author: 'Alex van Dijk',
        initials: 'AV',
        daysAgo: 3,
        content: 'Workloads migrated to server-05 temporarily. No data loss.',
      },
    ],
  },
  'd-007': {
    description:
      'Compliance-locked node. Access restricted to security team. Contains audit log storage and SIEM agent.',
    comments: [
      {
        author: 'Sara Müller',
        initials: 'SM',
        daysAgo: 30,
        content:
          'Annual compliance review completed. All controls passed. Node remains locked per policy.',
      },
    ],
  },
  'd-102': {
    description:
      'High-memory k8s worker node for team-gamma. Part of the primary cluster. Node labels: role=worker, tier=high-mem.',
    comments: [
      {
        author: 'Tom de Graaf',
        initials: 'TG',
        daysAgo: 7,
        content:
          'Added to Kubernetes cluster v1.31. Joined as node k8s-worker-10. Running 42 pods currently.',
      },
    ],
  },
};

export const DEVICE_HISTORY: Record<string, DeviceHistoryEntry[]> = {
  'd-003': [
    {
      action: 'maintenance',
      description: 'Firmware updated to BIOS 2.12.0 and iDRAC 6.10.00',
      user: 'Sara Müller',
      daysAgo: 14,
    },
    {
      action: 'hardware',
      description: 'RAM upgraded: 128 GB → 256 GB (8× 32 GB DIMMs added)',
      user: 'Tom de Graaf',
      daysAgo: 90,
    },
    {
      action: 'allocation',
      description: 'Project changed: team-beta → team-alpha',
      user: 'Tom de Graaf',
      daysAgo: 42,
    },
    {
      action: 'state-change',
      description: 'State changed: reserved → allocated',
      user: 'Alex van Dijk',
      daysAgo: 180,
    },
    {
      action: 'created',
      description: 'Device registered and racked at U37–U38',
      user: 'Alex van Dijk',
      daysAgo: 365,
    },
  ],
  'd-001': [
    {
      action: 'maintenance',
      description: 'IOS-XE upgraded from 17.3.5 to 17.6.2',
      user: 'Alex van Dijk',
      daysAgo: 60,
    },
    {
      action: 'allocation',
      description: 'VLAN config updated — DMZ segment added',
      user: 'Sara Müller',
      daysAgo: 5,
    },
    {
      action: 'created',
      description: 'Device registered and racked at U41–U42',
      user: 'Tom de Graaf',
      daysAgo: 400,
    },
  ],
  'd-004': [
    {
      action: 'state-change',
      description: 'State changed: allocated → offline (PSU failure)',
      user: 'Tom de Graaf',
      daysAgo: 1,
    },
    {
      action: 'maintenance',
      description: 'Diagnostics run — PSU-A failed, PSU-B degraded',
      user: 'Tom de Graaf',
      daysAgo: 3,
    },
    {
      action: 'allocation',
      description: 'Workloads migrated to d-005 (server-03)',
      user: 'Alex van Dijk',
      daysAgo: 3,
    },
    {
      action: 'state-change',
      description: 'State changed: free → allocated (team-beta)',
      user: 'Sara Müller',
      daysAgo: 210,
    },
    {
      action: 'created',
      description: 'Device registered and racked at U36',
      user: 'Sara Müller',
      daysAgo: 420,
    },
  ],
  'd-007': [
    {
      action: 'state-change',
      description: 'State changed: allocated → locked (compliance policy)',
      user: 'Sara Müller',
      daysAgo: 90,
    },
    {
      action: 'maintenance',
      description: 'Annual compliance audit completed — all controls passed',
      user: 'Sara Müller',
      daysAgo: 30,
    },
    {
      action: 'created',
      description: 'Device registered and racked at U31–U32',
      user: 'Alex van Dijk',
      daysAgo: 380,
    },
  ],
  'd-102': [
    {
      action: 'allocation',
      description: 'Joined Kubernetes cluster v1.31 as k8s-worker-10',
      user: 'Tom de Graaf',
      daysAgo: 7,
    },
    {
      action: 'hardware',
      description: 'NIC upgrade: 2× 25G → 4× 25G (dual-port card added)',
      user: 'Alex van Dijk',
      daysAgo: 60,
    },
    {
      action: 'created',
      description: 'Device registered and racked at U39–U40',
      user: 'Sara Müller',
      daysAgo: 200,
    },
  ],
};

// ── Device Connections (mock) ─────────────────────────────────────────────────

export const DEVICE_CONNECTIONS: Record<string, DeviceConnection[]> = {
  // server-01 (d-003) — Dell PowerEdge R750, 4× NIC, in AMS-01-R01
  'd-003': [
    {
      localPort: 'eth0',
      remoteDeviceId: 'd-001',
      remotePort: 'Gi0/10',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'eth1',
      remoteDeviceId: 'd-001',
      remotePort: 'Gi0/11',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'eth2',
      remoteDeviceId: 'd-002',
      remotePort: 'Port 3',
      type: 'management',
      speed: '1GbE',
      status: 'up',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 3',
      type: 'power',
      status: 'up',
    },
    {
      localPort: 'PSU-B',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 4',
      type: 'power',
      status: 'up',
    },
  ],
  // server-02 (d-004) — offline, PSU failed
  'd-004': [
    {
      localPort: 'eth0',
      remoteDeviceId: 'd-001',
      remotePort: 'Gi0/12',
      type: 'network',
      speed: '10GbE',
      status: 'down',
    },
    {
      localPort: 'eth1',
      remoteDeviceId: 'd-002',
      remotePort: 'Port 4',
      type: 'management',
      speed: '1GbE',
      status: 'down',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 5',
      type: 'power',
      status: 'down',
    },
    {
      localPort: 'PSU-B',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 6',
      type: 'power',
      status: 'unknown',
    },
  ],
  // server-03 (d-005) — storage node, 2× NIC
  'd-005': [
    {
      localPort: 'eth0',
      remoteDeviceId: 'd-001',
      remotePort: 'Gi0/13',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'eth1',
      remoteDeviceId: 'd-002',
      remotePort: 'Port 5',
      type: 'management',
      speed: '1GbE',
      status: 'up',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 7',
      type: 'power',
      status: 'up',
    },
    {
      localPort: 'PSU-B',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 8',
      type: 'power',
      status: 'up',
    },
  ],
  // tor-switch-01 (d-001) — uplinks to spine + patch panel IPMI connections
  'd-001': [
    {
      localPort: 'Te1/0/1',
      remoteDeviceId: 'd-301',
      remotePort: 'Et1/1',
      type: 'network',
      speed: '100GbE',
      status: 'up',
    },
    {
      localPort: 'Te1/0/2',
      remoteDeviceId: 'd-301',
      remotePort: 'Et1/2',
      type: 'network',
      speed: '100GbE',
      status: 'up',
    },
    {
      localPort: 'Gi0/10',
      remoteDeviceId: 'd-003',
      remotePort: 'eth0',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'Gi0/11',
      remoteDeviceId: 'd-003',
      remotePort: 'eth1',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'Gi0/12',
      remoteDeviceId: 'd-004',
      remotePort: 'eth0',
      type: 'network',
      speed: '10GbE',
      status: 'down',
    },
    {
      localPort: 'Gi0/13',
      remoteDeviceId: 'd-005',
      remotePort: 'eth0',
      type: 'network',
      speed: '10GbE',
      status: 'up',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 1',
      type: 'power',
      status: 'up',
    },
  ],
  // server-10 (d-102) — k8s worker, 4× 25G NIC
  'd-102': [
    {
      localPort: 'eth0',
      remoteDeviceId: 'd-101',
      remotePort: 'Gi0/10',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'eth1',
      remoteDeviceId: 'd-101',
      remotePort: 'Gi0/11',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'eth2',
      remoteDeviceId: 'd-101',
      remotePort: 'Gi0/12',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'eth3',
      remoteDeviceId: 'd-101',
      remotePort: 'Gi0/13',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 9',
      type: 'power',
      status: 'up',
    },
    {
      localPort: 'PSU-B',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 10',
      type: 'power',
      status: 'up',
    },
  ],
  // storage-server-01 (d-501) — NetApp, 4× NIC + storage fabric
  'd-501': [
    {
      localPort: 'e0a',
      remoteDeviceId: 'd-401',
      remotePort: 'Gi0/1',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'e0b',
      remoteDeviceId: 'd-401',
      remotePort: 'Gi0/2',
      type: 'network',
      speed: '25GbE',
      status: 'up',
    },
    {
      localPort: 'e0c',
      remoteDeviceId: 'd-501',
      remotePort: 'e0c',
      type: 'storage',
      speed: '32G FC',
      status: 'up',
    },
    {
      localPort: 'e0d',
      remoteDeviceId: 'd-501',
      remotePort: 'e0d',
      type: 'storage',
      speed: '32G FC',
      status: 'up',
    },
    {
      localPort: 'PSU-A',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 11',
      type: 'power',
      status: 'up',
    },
    {
      localPort: 'PSU-B',
      remoteDeviceId: 'd-008',
      remotePort: 'Outlet 12',
      type: 'power',
      status: 'up',
    },
  ],
};
