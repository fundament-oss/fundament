import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import InventoryApiService from './inventory-api.service';
import connectErrorMessage from '../../connect/error';

export type AssetStatus =
  | 'needs-repair'
  | 'decommissioned'
  | 'deployed'
  | 'available'
  | 'on-order'
  | 'requested';

export type AssetCategory =
  | 'Server'
  | 'Switch'
  | 'Storage'
  | 'Power'
  | 'Firewall'
  | 'Cooling'
  | 'KVM'
  | 'Other'
  | 'Memory'
  | 'Disk'
  | 'NIC'
  | 'PSU'
  | 'CPU'
  | 'GPU'
  | 'Transceiver';

export interface HistoryEntry {
  action: 'status-change' | 'location-change' | 'maintenance';
  description: string;
  user: string;
  daysAgo: number;
}

export interface Asset {
  id: string;
  model: string;
  assetTag: string;
  category: AssetCategory;
  datacenter: string;
  rack: string;
  status: AssetStatus;
  notes: string;
  parentId?: string;
}

const STATUS_SORT_ORDER: Record<AssetStatus, number> = {
  'needs-repair': 0,
  decommissioned: 1,
  deployed: 2,
  available: 3,
  'on-order': 4,
  requested: 5,
};

export interface NoteComment {
  author: string;
  initials: string;
  daysAgo: number;
  content: string;
}

export interface AssetNoteDetail {
  description: string;
  comments: NoteComment[];
}

export const MOCK_NOTES: Record<string, AssetNoteDetail> = {
  'AST-001': {
    description: 'Running VMware ESXi 8.0. RAM upgraded to 512 GB in Q1 2025.',
    comments: [
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 3,
        content: 'RAM upgrade completed. Both DIMMs seated correctly, no memory errors in POST.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 12,
        content: 'Scheduled for ESXi 8.0 Update 3 patch next maintenance window on April 6th.',
      },
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 31,
        content:
          'Migrated 4 VMs from AST-016 prior to decommission. All workloads confirmed healthy.',
      },
    ],
  },
  'AST-003': {
    description: 'Expected delivery 2025-04-15. Rack space reserved in AMS-02 row C.',
    comments: [
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 2,
        content:
          'Confirmed delivery slot with NetApp account manager. Cable management kit ships separately.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 8,
        content: 'PO #20250312 approved. Budget allocated from Q1 CapEx reserve.',
      },
    ],
  },
  'AST-004': {
    description: 'PSU failure reported 2025-03-18. Spare part ordered, awaiting arrival.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 1,
        content:
          'HP support case #7742301 escalated to priority. ETA for replacement PSU is 2 business days.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 8,
        content: 'Workloads migrated to AST-010 temporarily. No customer impact.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 8,
        content:
          'Node taken offline at 14:23 after second PSU also showed fault codes. Ticket #4520 linked.',
      },
    ],
  },
  'AST-007': {
    description: 'Kubernetes worker node in the AMS-01 production cluster (pool: standard).',
    comments: [
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 5,
        content:
          'Node cordoned briefly during kernel patch at 02:00. Uncordoned after successful reboot.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 19,
        content: 'Added to the production node pool. Taint removed after smoke tests passed.',
      },
    ],
  },
  'AST-009': {
    description: 'Primary SAN for the AMS-01 production cluster. All-flash, NVMe backend.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 4,
        content:
          'Snapshot schedule verified. Daily snapshots retained for 14 days, weekly for 90 days.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 14,
        content: 'Firmware updated to 6.6.12. No degradation in IOPS observed post-update.',
      },
    ],
  },
  'AST-012': {
    description:
      'Core spine switch for AMS-02. BGP sessions to upstream router and all leaf switches configured.',
    comments: [
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 6,
        content:
          'BGP peer AMS-02-LEAF-03 flapped twice between 03:12 and 03:18. Suspected fiber issue in bundle C. Monitoring.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 22,
        content: 'Config baseline saved to Netbox and GitLab repo. Running-config diff: clean.',
      },
    ],
  },
  'AST-013': {
    description: 'RAID controller fault detected. Ticket #4521 open with Lenovo support.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 0,
        content:
          'Lenovo engineer on-site tomorrow morning 09:00. Replacement controller arrived at loading dock.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 3,
        content:
          'RAID rebuild aborted at 34% — controller logs show CRC errors on SAS expander. Escalated to P1.',
      },
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 4,
        content: 'Data backed up to AST-017 before taking the array offline.',
      },
    ],
  },
  'AST-016': {
    description: 'Decommissioned. Replaced by AST-001. Asset ready for disposal or reallocation.',
    comments: [
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 17,
        content:
          'Data wiped with Blancco (certificate #BL-2025-1104). Drive sanitization log attached in ServiceNow.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 20,
        content: 'All VMs migrated off. iDRAC reset to factory defaults. Asset tag label updated.',
      },
    ],
  },
  'AST-020': {
    description: 'Leaf switch for pod A, AMS-01. Downstream of spine AST-012.',
    comments: [
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 9,
        content:
          'Port-channel to ESXi hosts reconfigured for LACP. Confirmed no packet loss during failover test.',
      },
    ],
  },
  'AST-025': {
    description: 'Reserved for new analytics project (Q2). Rack space allocated in AMS-01 row C.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 11,
        content:
          'Project kickoff pushed to May. Storage reserved until June 30 per agreement with Platform team.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 25,
        content:
          'Initial capacity sizing: 200 TB usable NVMe. Approved by architecture review board.',
      },
    ],
  },
  'AST-028': {
    description: 'Requested by Platform team for expanding the GitLab CI runner pool.',
    comments: [
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 7,
        content:
          'Request approved by Infra lead. Added to procurement backlog for Q2 budget cycle.',
      },
    ],
  },
  'AST-031': {
    description: 'Memory DIMM errors on slot A3. Replacement in progress.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 2,
        content: 'Replacement 64 GB LRDIMM ordered from Lenovo. ETA: 3 business days.',
      },
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 2,
        content:
          'Server running with reduced 384 GB RAM (slot A3 disabled). No workload impact confirmed.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 5,
        content:
          'DIMM errors confirmed in XClarity. EDAC counters showing uncorrectable errors on slot A3.',
      },
    ],
  },
  'AST-037': {
    description: 'Edge router for AMS-01. Upstream BGP peering to transit providers.',
    comments: [
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 13,
        content:
          'Route policy updated to prepend AS path for secondary transit. Failover tested and confirmed < 30s convergence.',
      },
    ],
  },
  'AST-048': {
    description: 'Coolant leak detected near rear manifold. Unit taken out of service.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 1,
        content:
          'Emerson field engineer confirmed manifold O-ring failure. Repair kit on order. Estimated 5 day downtime.',
      },
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 1,
        content:
          'Adjacent servers AST-050 and AST-026 inspected for moisture — no issues found. Cleanup completed.',
      },
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 3,
        content:
          'Thermal load redistributed to APC unit in adjacent row. Cabinet temps stable at 22°C.',
      },
    ],
  },
  'AST-057': {
    description: 'For new analytics cluster. ETA 2025-05-01. Will be deployed in AMS-02 row A.',
    comments: [
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 15,
        content:
          'Dell quote accepted. Spec: 2× Xeon Gold 6430, 1 TB RAM, 8× 25GbE. Lead time ~5 weeks.',
      },
    ],
  },
  'AST-068': {
    description:
      'Tape library for long-term backup. Weekly full backup target for all production clusters.',
    comments: [
      {
        author: 'Jan de Vries',
        initials: 'JV',
        daysAgo: 6,
        content:
          'Tape media inventory refreshed. 40 LTO-9 cartridges added. Vault shipment scheduled for Friday.',
      },
      {
        author: 'Roos van Dijk',
        initials: 'RD',
        daysAgo: 28,
        content: 'Quantum firmware updated to 900G.2.1.0. Barcode scanner calibration verified.',
      },
    ],
  },
  'AST-069': {
    description: 'NIC flapping issue under load. Ticket #4788 open with Dell support.',
    comments: [
      {
        author: 'Pieter Hoek',
        initials: 'PH',
        daysAgo: 0,
        content:
          'Dell engineer remote session at 15:00 today. Will attempt firmware rollback on Broadcom 57414 NIC.',
      },
      {
        author: 'Sarah Müller',
        initials: 'SM',
        daysAgo: 4,
        content:
          'Captured packet loss logs. Interface flaps correlate with TX queue depth > 80%. Suspect driver bug in 224.0.512.',
      },
    ],
  },
};

export const MOCK_HISTORY: Record<string, HistoryEntry[]> = {
  'AST-001': [
    {
      action: 'maintenance',
      description: 'RAM upgraded from 256 GB to 512 GB (8× 64 GB DIMMs installed)',
      user: 'Jan de Vries',
      daysAgo: 7,
    },
    {
      action: 'status-change',
      description: 'ESXi 8.0 Update 3 patch applied, server back in service',
      user: 'Sarah Müller',
      daysAgo: 18,
    },
    {
      action: 'location-change',
      description: 'Migrated from AMS-02 / Rack A07 to AMS-01 / A12',
      user: 'Pieter Hoek',
      daysAgo: 90,
    },
    {
      action: 'status-change',
      description: 'Status set to deployed after initial configuration complete',
      user: 'Roos van Dijk',
      daysAgo: 180,
    },
  ],
  'AST-004': [
    {
      action: 'maintenance',
      description: 'PSU fault detected on secondary power supply, escalated to HPE support',
      user: 'Jan de Vries',
      daysAgo: 23,
    },
    {
      action: 'status-change',
      description: 'Status changed to needs-repair, workloads migrated to AST-010',
      user: 'Sarah Müller',
      daysAgo: 23,
    },
    {
      action: 'status-change',
      description: 'Deployed as ESXi host in BRU-01 cluster',
      user: 'Pieter Hoek',
      daysAgo: 400,
    },
  ],
  'AST-007': [
    {
      action: 'maintenance',
      description: 'Kernel patched to 5.15.0-100, node cordoned and uncordoned at 02:00',
      user: 'Roos van Dijk',
      daysAgo: 5,
    },
    {
      action: 'status-change',
      description: 'Added to production Kubernetes node pool (taint removed after smoke tests)',
      user: 'Sarah Müller',
      daysAgo: 19,
    },
    {
      action: 'location-change',
      description: 'Racked in AMS-01 / A13, cabled to top-of-rack switch SW-001',
      user: 'Jan de Vries',
      daysAgo: 25,
    },
  ],
  'AST-013': [
    {
      action: 'maintenance',
      description: 'RAID rebuild aborted at 34% — CRC errors on SAS expander, escalated to P1',
      user: 'Jan de Vries',
      daysAgo: 3,
    },
    {
      action: 'status-change',
      description: 'Status changed to needs-repair, ticket #4521 opened with Lenovo',
      user: 'Pieter Hoek',
      daysAgo: 4,
    },
    {
      action: 'maintenance',
      description: 'Data backed up to AST-017 before array taken offline',
      user: 'Roos van Dijk',
      daysAgo: 4,
    },
    {
      action: 'status-change',
      description: 'Deployed as compute node in BRU-01',
      user: 'Sarah Müller',
      daysAgo: 420,
    },
  ],
  'AST-031': [
    {
      action: 'maintenance',
      description: 'DIMM errors confirmed in XClarity on slot A3, replacement ordered',
      user: 'Jan de Vries',
      daysAgo: 5,
    },
    {
      action: 'status-change',
      description: 'Status changed to needs-repair, server running with 384 GB (slot A3 disabled)',
      user: 'Roos van Dijk',
      daysAgo: 5,
    },
  ],
  'MEM-001': [
    {
      action: 'status-change',
      description: 'Installed in AST-001 slot A1, upgrade from 32 GB DIMM',
      user: 'Jan de Vries',
      daysAgo: 7,
    },
    {
      action: 'maintenance',
      description: 'POST memory test passed, no ECC errors detected post-install',
      user: 'Pieter Hoek',
      daysAgo: 7,
    },
  ],
  'MEM-002': [
    {
      action: 'status-change',
      description: 'Installed in AST-001 slot A2, upgrade from 32 GB DIMM',
      user: 'Jan de Vries',
      daysAgo: 7,
    },
  ],
  'DSK-001': [
    {
      action: 'status-change',
      description: 'Deployed as boot disk (bay 0) in AST-001',
      user: 'Sarah Müller',
      daysAgo: 180,
    },
  ],
  'DSK-002': [
    {
      action: 'status-change',
      description: 'Deployed as data disk (bay 1) in AST-001',
      user: 'Sarah Müller',
      daysAgo: 180,
    },
  ],
  'NIC-001': [
    {
      action: 'status-change',
      description: 'Installed in AST-001 PCIe slot 3, bonded with NIC-002',
      user: 'Pieter Hoek',
      daysAgo: 180,
    },
  ],
  'PSU-001': [
    {
      action: 'status-change',
      description: 'PSU fault detected, server switched to single-PSU mode',
      user: 'Jan de Vries',
      daysAgo: 23,
    },
    {
      action: 'status-change',
      description: 'Replacement ordered from HPE, expected in 2 business days',
      user: 'Pieter Hoek',
      daysAgo: 22,
    },
  ],
  'PSU-002': [
    {
      action: 'status-change',
      description: 'Confirmed operational, carrying full server load while PSU-001 is faulty',
      user: 'Jan de Vries',
      daysAgo: 23,
    },
  ],
  'CPU-001': [
    {
      action: 'status-change',
      description: 'Installed in SRV-003 socket 0 during initial deployment',
      user: 'Roos van Dijk',
      daysAgo: 90,
    },
  ],
  'CPU-002': [
    {
      action: 'status-change',
      description: 'Installed in SRV-003 socket 1 during initial deployment',
      user: 'Roos van Dijk',
      daysAgo: 90,
    },
  ],
};

export interface CatalogEntry {
  id: string;
  model: string;
  manufacturer: string;
  category: AssetCategory;
  specs: Record<string, string>;
}

export interface PortDefinition {
  id: string;
  catalogEntryId: string;
  name: string;
  portType: string;
  speedGbps?: number;
  powerWatts?: number;
}

export interface PortCompatibility {
  id: string;
  portDefinitionId: string;
  compatibleCatalogEntryId: string;
}

// TODO(api): CatalogService.ListPortDefinitions({ catalog_entry_id })
export const MOCK_PORT_DEFINITIONS: PortDefinition[] = [
  { id: 'pd-001', catalogEntryId: 'CAT-001', name: 'nic0', portType: 'SFP+', speedGbps: 10 },
  { id: 'pd-002', catalogEntryId: 'CAT-001', name: 'nic1', portType: 'SFP+', speedGbps: 10 },
  { id: 'pd-003', catalogEntryId: 'CAT-001', name: 'psu0', portType: 'IEC C13', powerWatts: 800 },
  { id: 'pd-004', catalogEntryId: 'CAT-006', name: 'uplink0', portType: 'QSFP+', speedGbps: 40 },
  { id: 'pd-005', catalogEntryId: 'CAT-006', name: 'uplink1', portType: 'QSFP+', speedGbps: 40 },
  { id: 'pd-006', catalogEntryId: 'CAT-007', name: 'p0-p31', portType: 'QSFP28', speedGbps: 100 },
];

// TODO(api): CatalogService.ListPortCompatibilities({ port_definition_id })
export const MOCK_PORT_COMPATIBILITIES: PortCompatibility[] = [
  { id: 'pc-001', portDefinitionId: 'pd-001', compatibleCatalogEntryId: 'CAT-006' },
  { id: 'pc-002', portDefinitionId: 'pd-004', compatibleCatalogEntryId: 'CAT-001' },
  { id: 'pc-003', portDefinitionId: 'pd-006', compatibleCatalogEntryId: 'CAT-007' },
];

export const MOCK_CATALOG: CatalogEntry[] = [
  {
    id: 'CAT-001',
    model: 'Dell PowerEdge R750',
    manufacturer: 'Dell Technologies',
    category: 'Server',
    specs: { 'CPU Sockets': '2', 'Max RAM': '8 TB', 'Drive Bays': '24× 2.5"', 'Form Factor': '2U' },
  },
  {
    id: 'CAT-002',
    model: 'Dell PowerEdge R650',
    manufacturer: 'Dell Technologies',
    category: 'Server',
    specs: { 'CPU Sockets': '2', 'Max RAM': '4 TB', 'Drive Bays': '10× 2.5"', 'Form Factor': '1U' },
  },
  {
    id: 'CAT-003',
    model: 'Dell PowerEdge R740xd',
    manufacturer: 'Dell Technologies',
    category: 'Server',
    specs: { 'CPU Sockets': '2', 'Max RAM': '3 TB', 'Drive Bays': '24× 3.5"', 'Form Factor': '2U' },
  },
  {
    id: 'CAT-004',
    model: 'HP ProLiant DL380 Gen10',
    manufacturer: 'Hewlett Packard Enterprise',
    category: 'Server',
    specs: { 'CPU Sockets': '2', 'Max RAM': '3 TB', 'Drive Bays': '24× SFF', 'Form Factor': '2U' },
  },
  {
    id: 'CAT-005',
    model: 'Supermicro SYS-221H-TN',
    manufacturer: 'Supermicro',
    category: 'Server',
    specs: { 'CPU Sockets': '2', 'Max RAM': '4 TB', 'Drive Bays': '12× NVMe', 'Form Factor': '2U' },
  },
  {
    id: 'CAT-006',
    model: 'Cisco Catalyst 9300-48P',
    manufacturer: 'Cisco Systems',
    category: 'Switch',
    specs: { Ports: '48× 1G PoE+', Uplinks: '4× 40G', 'Switching Capacity': '424 Gbps' },
  },
  {
    id: 'CAT-007',
    model: 'Arista 7050CX3-32S',
    manufacturer: 'Arista Networks',
    category: 'Switch',
    specs: { Ports: '32× 100G QSFP28', 'Switching Capacity': '6.4 Tbps', 'Form Factor': '1U' },
  },
  {
    id: 'CAT-008',
    model: 'Samsung 64GB DDR5-4800 RDIMM',
    manufacturer: 'Samsung',
    category: 'Memory',
    specs: { Capacity: '64 GB', Type: 'DDR5 RDIMM', Speed: '4800 MT/s', ECC: 'Yes' },
  },
  {
    id: 'CAT-009',
    model: 'WD 4TB SAS 12Gbps 7200rpm',
    manufacturer: 'Western Digital',
    category: 'Disk',
    specs: { Capacity: '4 TB', Interface: 'SAS 12Gbps', RPM: '7200', 'Form Factor': '3.5"' },
  },
  {
    id: 'CAT-010',
    model: 'Intel X710 Dual-Port 10GbE',
    manufacturer: 'Intel',
    category: 'NIC',
    specs: { Ports: '2× 10GbE SFP+', Interface: 'PCIe 3.0 x8', Offloads: 'TCP/UDP checksum, TSO' },
  },
  {
    id: 'CAT-011',
    model: 'HPE 800W Flex Slot Platinum Plus PSU',
    manufacturer: 'Hewlett Packard Enterprise',
    category: 'PSU',
    specs: { 'Output Power': '800 W', Efficiency: '94% (Platinum Plus)', Input: '100–240 V AC' },
  },
  {
    id: 'CAT-012',
    model: 'Intel Xeon Gold 6338 (32C)',
    manufacturer: 'Intel',
    category: 'CPU',
    specs: {
      Cores: '32',
      'Base Freq': '2.0 GHz',
      Turbo: '3.2 GHz',
      TDP: '205 W',
      Socket: 'LGA4189',
    },
  },
  {
    id: 'CAT-013',
    model: 'NetApp AFF A800',
    manufacturer: 'NetApp',
    category: 'Storage',
    specs: { 'Max Capacity': '1.5 PB', Protocol: 'NFS/CIFS/iSCSI/FC', 'Form Factor': '4U HA pair' },
  },
  {
    id: 'CAT-014',
    model: 'Pure Storage FlashArray//X70',
    manufacturer: 'Pure Storage',
    category: 'Storage',
    specs: { 'Effective Capacity': '1.6 PB', Protocol: 'iSCSI/FC/NVMe-oF', Latency: '<500 µs' },
  },
  {
    id: 'CAT-015',
    model: 'Palo Alto PA-5250',
    manufacturer: 'Palo Alto Networks',
    category: 'Firewall',
    specs: { Throughput: '20 Gbps', Sessions: '32M', Interfaces: '16× 1G/10G SFP+' },
  },
];

export const MOCK_ASSETS: Asset[] = [
  {
    id: 'AST-001',
    model: 'Dell PowerEdge R750',
    assetTag: 'AST-001',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: 'Running VMware ESXi 8.0. RAM upgraded to 512 GB in Q1 2025.',
  },
  {
    id: 'AST-002',
    model: 'Cisco Catalyst 9300-48P',
    assetTag: 'AST-002',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'B05',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-003',
    model: 'NetApp AFF A800',
    assetTag: 'AST-003',
    category: 'Storage',
    datacenter: 'AMS-02',
    rack: 'C08',
    status: 'on-order',
    notes: 'Expected delivery 2025-04-15. Rack space reserved.',
  },
  {
    id: 'AST-004',
    model: 'HP ProLiant DL380 Gen10',
    assetTag: 'AST-004',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'D14',
    status: 'needs-repair',
    notes: 'PSU failure reported 2025-03-18. Spare ordered.',
  },
  {
    id: 'AST-005',
    model: 'Juniper EX4300-48T',
    assetTag: 'AST-005',
    category: 'Switch',
    datacenter: 'BRU-01',
    rack: 'A02',
    status: 'requested',
    notes: '',
  },
  {
    id: 'AST-006',
    model: 'Eaton 9PX 6000i',
    assetTag: 'AST-006',
    category: 'Power',
    datacenter: 'AMS-02',
    rack: 'A01',
    status: 'decommissioned',
    notes: '',
  },
  {
    id: 'AST-007',
    model: 'Dell PowerEdge R650',
    assetTag: 'AST-007',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A13',
    status: 'deployed',
    notes: 'Kubernetes worker node.',
  },
  {
    id: 'AST-008',
    model: 'Palo Alto PA-5250',
    assetTag: 'AST-008',
    category: 'Firewall',
    datacenter: 'AMS-01',
    rack: 'F01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-009',
    model: 'Pure Storage FlashArray//X70',
    assetTag: 'AST-009',
    category: 'Storage',
    datacenter: 'AMS-01',
    rack: 'C04',
    status: 'deployed',
    notes: 'Primary SAN for production cluster.',
  },
  {
    id: 'AST-010',
    model: 'Supermicro SYS-620P',
    assetTag: 'AST-010',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'B09',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-011',
    model: 'APC InRow RC 6kW',
    assetTag: 'AST-011',
    category: 'Cooling',
    datacenter: 'BRU-01',
    rack: 'Z01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-012',
    model: 'Arista 7050CX3-32S',
    assetTag: 'AST-012',
    category: 'Switch',
    datacenter: 'AMS-02',
    rack: 'B01',
    status: 'deployed',
    notes: 'Core spine switch. BGP peers configured.',
  },
  {
    id: 'AST-013',
    model: 'Lenovo ThinkSystem SR650',
    assetTag: 'AST-013',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'A05',
    status: 'needs-repair',
    notes: 'RAID controller fault. Ticket #4521 open.',
  },
  {
    id: 'AST-014',
    model: 'Raritan Dominion KX IV',
    assetTag: 'AST-014',
    category: 'KVM',
    datacenter: 'AMS-01',
    rack: 'M01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-015',
    model: 'APC Smart-UPS 3000VA',
    assetTag: 'AST-015',
    category: 'Power',
    datacenter: 'AMS-01',
    rack: 'A02',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-016',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'AST-016',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A07',
    status: 'decommissioned',
    notes: 'Replaced by AST-001. Ready for disposal.',
  },
  {
    id: 'AST-017',
    model: 'HPE MSA 2060 SAS',
    assetTag: 'AST-017',
    category: 'Storage',
    datacenter: 'BRU-01',
    rack: 'C11',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-018',
    model: 'Fortinet FortiGate 600F',
    assetTag: 'AST-018',
    category: 'Firewall',
    datacenter: 'AMS-02',
    rack: 'F02',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-019',
    model: 'HP ProLiant DL360 Gen10',
    assetTag: 'AST-019',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A15',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-020',
    model: 'Cisco Nexus 9336C-FX2',
    assetTag: 'AST-020',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'B02',
    status: 'deployed',
    notes: 'Leaf switch for pod A.',
  },
  {
    id: 'AST-021',
    model: 'Dell PowerEdge R750xs',
    assetTag: 'AST-021',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A10',
    status: 'on-order',
    notes: '',
  },
  {
    id: 'AST-022',
    model: 'Schneider Electric Galaxy VS',
    assetTag: 'AST-022',
    category: 'Cooling',
    datacenter: 'AMS-02',
    rack: 'Z02',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-023',
    model: 'Supermicro SYS-221H',
    assetTag: 'AST-023',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'B07',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-024',
    model: 'Juniper QFX5120-48Y',
    assetTag: 'AST-024',
    category: 'Switch',
    datacenter: 'BRU-01',
    rack: 'B03',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-025',
    model: 'Dell EMC PowerStore 3000T',
    assetTag: 'AST-025',
    category: 'Storage',
    datacenter: 'AMS-01',
    rack: 'C06',
    status: 'available',
    notes: 'Reserved for new project Q2.',
  },
  {
    id: 'AST-026',
    model: 'HP ProLiant DL580 Gen10',
    assetTag: 'AST-026',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A18',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-027',
    model: 'Vertiv Geist PDU',
    assetTag: 'AST-027',
    category: 'Power',
    datacenter: 'BRU-01',
    rack: 'A01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-028',
    model: 'Dell PowerEdge R450',
    assetTag: 'AST-028',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A14',
    status: 'requested',
    notes: 'Requested by Platform team for CI runners.',
  },
  {
    id: 'AST-029',
    model: 'Check Point 6800',
    assetTag: 'AST-029',
    category: 'Firewall',
    datacenter: 'BRU-01',
    rack: 'F01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-030',
    model: 'Arista 7280CR3-32P4',
    assetTag: 'AST-030',
    category: 'Switch',
    datacenter: 'AMS-02',
    rack: 'B04',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-031',
    model: 'Lenovo ThinkSystem SD530',
    assetTag: 'AST-031',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A20',
    status: 'needs-repair',
    notes: 'Memory DIMM errors. Replacement in progress.',
  },
  {
    id: 'AST-032',
    model: 'Avocent ACS 8000',
    assetTag: 'AST-032',
    category: 'KVM',
    datacenter: 'AMS-02',
    rack: 'M01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-033',
    model: 'IBM Storwize V7000',
    assetTag: 'AST-033',
    category: 'Storage',
    datacenter: 'BRU-01',
    rack: 'C03',
    status: 'decommissioned',
    notes: '',
  },
  {
    id: 'AST-034',
    model: 'Dell PowerEdge MX750c',
    assetTag: 'AST-034',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A11',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-035',
    model: 'Stulz CyberAir 3 PRO',
    assetTag: 'AST-035',
    category: 'Cooling',
    datacenter: 'BRU-01',
    rack: 'Z03',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-036',
    model: 'HP ProLiant BL460c Gen10',
    assetTag: 'AST-036',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A22',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-037',
    model: 'Cisco ASR 9001',
    assetTag: 'AST-037',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'B06',
    status: 'deployed',
    notes: 'Edge router for AMS-01.',
  },
  {
    id: 'AST-038',
    model: 'Eaton 93PM 80kVA',
    assetTag: 'AST-038',
    category: 'Power',
    datacenter: 'AMS-02',
    rack: 'A03',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-039',
    model: 'Dell PowerEdge R6625',
    assetTag: 'AST-039',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'A09',
    status: 'on-order',
    notes: '',
  },
  {
    id: 'AST-040',
    model: 'NetApp FAS8700',
    assetTag: 'AST-040',
    category: 'Storage',
    datacenter: 'AMS-01',
    rack: 'C02',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-041',
    model: 'Supermicro SYS-420GP',
    assetTag: 'AST-041',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A16',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-042',
    model: 'Palo Alto PA-3260',
    assetTag: 'AST-042',
    category: 'Firewall',
    datacenter: 'BRU-01',
    rack: 'F02',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-043',
    model: 'Juniper EX9253',
    assetTag: 'AST-043',
    category: 'Switch',
    datacenter: 'BRU-01',
    rack: 'B08',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-044',
    model: 'Dell PowerEdge R760',
    assetTag: 'AST-044',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A25',
    status: 'requested',
    notes: '',
  },
  {
    id: 'AST-045',
    model: 'Raritan Dominion SX II',
    assetTag: 'AST-045',
    category: 'KVM',
    datacenter: 'BRU-01',
    rack: 'M01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-046',
    model: 'HPE Nimble Storage AF40',
    assetTag: 'AST-046',
    category: 'Storage',
    datacenter: 'AMS-02',
    rack: 'C09',
    status: 'on-order',
    notes: '',
  },
  {
    id: 'AST-047',
    model: 'Lenovo ThinkSystem SR850',
    assetTag: 'AST-047',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A19',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-048',
    model: 'Emerson Liebert PEX',
    assetTag: 'AST-048',
    category: 'Cooling',
    datacenter: 'AMS-01',
    rack: 'Z01',
    status: 'needs-repair',
    notes: 'Coolant leak detected. Out of service pending repair.',
  },
  {
    id: 'AST-049',
    model: 'Cisco Catalyst 9500-48Y4C',
    assetTag: 'AST-049',
    category: 'Switch',
    datacenter: 'AMS-02',
    rack: 'B07',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-050',
    model: 'HP ProLiant DL325 Gen10+',
    assetTag: 'AST-050',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A27',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-051',
    model: 'APC Symmetra PX 100kW',
    assetTag: 'AST-051',
    category: 'Power',
    datacenter: 'BRU-01',
    rack: 'A04',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-052',
    model: 'Dell PowerEdge R550',
    assetTag: 'AST-052',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'A12',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-053',
    model: 'Hitachi VSP G600',
    assetTag: 'AST-053',
    category: 'Storage',
    datacenter: 'AMS-01',
    rack: 'C10',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-054',
    model: 'Fortinet FortiGate 1800F',
    assetTag: 'AST-054',
    category: 'Firewall',
    datacenter: 'AMS-01',
    rack: 'F03',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-055',
    model: 'Arista 7060CX2-32S',
    assetTag: 'AST-055',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'B09',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-056',
    model: 'Supermicro SYS-221BT',
    assetTag: 'AST-056',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'B02',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-057',
    model: 'Dell PowerEdge R960',
    assetTag: 'AST-057',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A21',
    status: 'on-order',
    notes: 'For new analytics cluster. ETA 2025-05-01.',
  },
  {
    id: 'AST-058',
    model: 'Vertiv Liebert DSE 100kW',
    assetTag: 'AST-058',
    category: 'Cooling',
    datacenter: 'AMS-02',
    rack: 'Z04',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-059',
    model: 'Avocent MergePoint Unity 108',
    assetTag: 'AST-059',
    category: 'KVM',
    datacenter: 'AMS-02',
    rack: 'M02',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-060',
    model: 'HP ProLiant ML350 Gen10',
    assetTag: 'AST-060',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'A15',
    status: 'decommissioned',
    notes: 'License expired.',
  },
  {
    id: 'AST-061',
    model: 'Juniper MX480',
    assetTag: 'AST-061',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'B10',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-062',
    model: 'Pure Storage FlashArray//C60',
    assetTag: 'AST-062',
    category: 'Storage',
    datacenter: 'BRU-01',
    rack: 'C05',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-063',
    model: 'Dell PowerEdge R250',
    assetTag: 'AST-063',
    category: 'Server',
    datacenter: 'BRU-01',
    rack: 'A17',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-064',
    model: 'Eaton 9PX EBM',
    assetTag: 'AST-064',
    category: 'Power',
    datacenter: 'AMS-01',
    rack: 'A05',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-065',
    model: 'Cisco Firepower 4150',
    assetTag: 'AST-065',
    category: 'Firewall',
    datacenter: 'AMS-02',
    rack: 'F03',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-066',
    model: 'Lenovo ThinkSystem SR630 V2',
    assetTag: 'AST-066',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'A30',
    status: 'requested',
    notes: '',
  },
  {
    id: 'AST-067',
    model: 'Cisco Catalyst 9200L-24P',
    assetTag: 'AST-067',
    category: 'Switch',
    datacenter: 'BRU-01',
    rack: 'B11',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-068',
    model: 'Quantum Scalar i6000',
    assetTag: 'AST-068',
    category: 'Storage',
    datacenter: 'AMS-01',
    rack: 'C12',
    status: 'deployed',
    notes: 'Tape library. Weekly backup target.',
  },
  {
    id: 'AST-069',
    model: 'Dell PowerEdge R7625',
    assetTag: 'AST-069',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'A23',
    status: 'needs-repair',
    notes: 'NIC flapping issue. Ticket #4788 open.',
  },
  {
    id: 'AST-070',
    model: 'APC Uniflair 30kW',
    assetTag: 'AST-070',
    category: 'Cooling',
    datacenter: 'BRU-01',
    rack: 'Z05',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'AST-071',
    model: 'HP ProLiant DL160 Gen10',
    assetTag: 'AST-071',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'B11',
    status: 'available',
    notes: '',
  },
  {
    id: 'AST-072',
    model: 'Vertiv Liebert EXM 60kVA',
    assetTag: 'AST-072',
    category: 'Power',
    datacenter: 'AMS-02',
    rack: 'A06',
    status: 'deployed',
    notes: '',
  },

  // ── AMS-01-R01 ────────────────────────────────────────────────────────────
  {
    id: 'SW-001',
    model: 'Cisco Catalyst 9336C-FX2',
    assetTag: 'SW-001',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: 'Top-of-rack switch for AMS-01-R01.',
  },
  {
    id: 'PP-001',
    model: 'Panduit 24-port Cat6A Patch Panel',
    assetTag: 'PP-001',
    category: 'Other',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: '',
  },
  {
    id: 'SRV-003',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-003',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: 'Compute node for team-alpha. Running ubuntu-22.04.',
  },
  {
    id: 'SRV-004',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-004',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'needs-repair',
    notes: 'Offline since 2026-04-03 — PSU fault detected. Ticket open.',
  },
  {
    id: 'SRV-005',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'SRV-005',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: 'Storage node for team-beta. Running debian-11.',
  },
  {
    id: 'SRV-006',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-006',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'available',
    notes: 'Reserved for upcoming allocation.',
  },
  {
    id: 'SRV-007',
    model: 'Dell PowerEdge R450',
    assetTag: 'SRV-007',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: 'Compliance audit server — access locked. Running rhel-9.',
  },
  {
    id: 'PDU-001',
    model: 'Vertiv Geist rPDU 24-outlet',
    assetTag: 'PDU-001',
    category: 'Power',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: '',
  },

  // ── AMS-01-R02 ────────────────────────────────────────────────────────────
  {
    id: 'SW-101',
    model: 'Arista 7050CX3-32S',
    assetTag: 'SW-101',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R02',
    status: 'deployed',
    notes: 'Leaf switch for AMS-01-R02.',
  },
  {
    id: 'SRV-102',
    model: 'Supermicro SYS-221H-TN',
    assetTag: 'SRV-102',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R02',
    status: 'deployed',
    notes: 'Kubernetes worker node for team-gamma.',
  },
  {
    id: 'SRV-103',
    model: 'Supermicro SYS-221H-TN',
    assetTag: 'SRV-103',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R02',
    status: 'deployed',
    notes: 'Kubernetes worker node for team-gamma.',
  },
  {
    id: 'SRV-104',
    model: 'Supermicro SYS-221H-TN',
    assetTag: 'SRV-104',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R02',
    status: 'available',
    notes: '',
  },

  // ── AMS-01-R03 ────────────────────────────────────────────────────────────
  {
    id: 'SRV-201',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-201',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-202',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-202',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-203',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-203',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-beta.',
  },
  {
    id: 'SRV-204',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-204',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-beta.',
  },
  {
    id: 'SRV-205',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-205',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-gamma.',
  },
  {
    id: 'SRV-206',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-206',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-gamma.',
  },
  {
    id: 'SRV-207',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'SRV-207',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Storage node for team-alpha.',
  },
  {
    id: 'SRV-208',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'SRV-208',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Storage node for team-alpha.',
  },
  {
    id: 'SRV-209',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'SRV-209',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Storage node for team-beta.',
  },
  {
    id: 'SRV-210',
    model: 'Dell PowerEdge R740xd',
    assetTag: 'SRV-210',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Storage node for team-beta.',
  },
  {
    id: 'SRV-211',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-211',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Network node for team-gamma.',
  },
  {
    id: 'SRV-212',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-212',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Network node for team-gamma.',
  },
  {
    id: 'SRV-213',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-213',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-214',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-214',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-215',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-215',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-beta.',
  },
  {
    id: 'SRV-216',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-216',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-beta.',
  },
  {
    id: 'SRV-217',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-217',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-gamma.',
  },
  {
    id: 'SRV-218',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-218',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-gamma.',
  },
  {
    id: 'SRV-219',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-219',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-220',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-220',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-alpha.',
  },
  {
    id: 'SRV-221',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-221',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R03',
    status: 'deployed',
    notes: 'Compute node for team-beta.',
  },

  // ── AMS-01-R04 ────────────────────────────────────────────────────────────
  {
    id: 'SW-301',
    model: 'Arista 7280CR3-32P4',
    assetTag: 'SW-301',
    category: 'Switch',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R04',
    status: 'deployed',
    notes: 'Spine switch for AMS-01. BGP peering configured.',
  },
  {
    id: 'SRV-302',
    model: 'HP ProLiant DL360 Gen10',
    assetTag: 'SRV-302',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R04',
    status: 'deployed',
    notes: 'Management server for AMS-01 infrastructure.',
  },
  {
    id: 'SRV-303',
    model: 'HP ProLiant DL360 Gen10',
    assetTag: 'SRV-303',
    category: 'Server',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R04',
    status: 'deployed',
    notes: 'Bastion host — access restricted. Running ubuntu-22.04.',
  },

  // ── AMS-02-R01 ────────────────────────────────────────────────────────────
  {
    id: 'SW-401',
    model: 'Cisco Catalyst 9300-48P',
    assetTag: 'SW-401',
    category: 'Switch',
    datacenter: 'AMS-02',
    rack: 'AMS-02-R01',
    status: 'deployed',
    notes: 'Top-of-rack switch for AMS-02-R01.',
  },
  {
    id: 'SRV-402',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-402',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'AMS-02-R01',
    status: 'deployed',
    notes: 'Compute node for team-delta.',
  },
  {
    id: 'SRV-403',
    model: 'Dell PowerEdge R750',
    assetTag: 'SRV-403',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'AMS-02-R01',
    status: 'deployed',
    notes: 'Compute node for team-delta.',
  },

  // ── AMS-02-R02 ────────────────────────────────────────────────────────────
  {
    id: 'SRV-501',
    model: 'NetApp AFF A400',
    assetTag: 'SRV-501',
    category: 'Storage',
    datacenter: 'AMS-02',
    rack: 'AMS-02-R02',
    status: 'deployed',
    notes: 'Primary NAS for AMS-02. All-flash, running TrueNAS.',
  },
  {
    id: 'SRV-502',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-502',
    category: 'Server',
    datacenter: 'AMS-02',
    rack: 'AMS-02-R02',
    status: 'available',
    notes: '',
  },

  // ── FRA-01-R01 ────────────────────────────────────────────────────────────
  {
    id: 'SW-601',
    model: 'Cisco Catalyst 9300-24P',
    assetTag: 'SW-601',
    category: 'Switch',
    datacenter: 'FRA-01',
    rack: 'FRA-01-R01',
    status: 'deployed',
    notes: 'Top-of-rack switch for FRA-01-R01.',
  },
  {
    id: 'SRV-602',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-602',
    category: 'Server',
    datacenter: 'FRA-01',
    rack: 'FRA-01-R01',
    status: 'available',
    notes: 'Reserved for fra-core expansion.',
  },
  {
    id: 'SRV-603',
    model: 'Dell PowerEdge R650',
    assetTag: 'SRV-603',
    category: 'Server',
    datacenter: 'FRA-01',
    rack: 'FRA-01-R01',
    status: 'deployed',
    notes: 'Compute node for fra-core project. Running ubuntu-22.04.',
  },

  // ── Components of AST-001 (Dell PowerEdge R750) ───────────────────────────
  {
    id: 'MEM-001',
    model: 'Samsung 64GB DDR5-4800 RDIMM',
    assetTag: 'MEM-001',
    category: 'Memory',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: '',
    parentId: 'AST-001',
  },
  {
    id: 'MEM-002',
    model: 'Samsung 64GB DDR5-4800 RDIMM',
    assetTag: 'MEM-002',
    category: 'Memory',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: '',
    parentId: 'AST-001',
  },
  {
    id: 'DSK-001',
    model: 'WD 4TB SAS 12Gbps 7200rpm',
    assetTag: 'DSK-001',
    category: 'Disk',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: '',
    parentId: 'AST-001',
  },
  {
    id: 'DSK-002',
    model: 'WD 4TB SAS 12Gbps 7200rpm',
    assetTag: 'DSK-002',
    category: 'Disk',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: '',
    parentId: 'AST-001',
  },
  {
    id: 'NIC-001',
    model: 'Intel X710 Dual-Port 10GbE',
    assetTag: 'NIC-001',
    category: 'NIC',
    datacenter: 'AMS-01',
    rack: 'A12',
    status: 'deployed',
    notes: '',
    parentId: 'AST-001',
  },

  // ── Components of AST-004 (HP ProLiant DL380 Gen10) ──────────────────────
  {
    id: 'PSU-001',
    model: 'HPE 800W Flex Slot Platinum Plus PSU',
    assetTag: 'PSU-001',
    category: 'PSU',
    datacenter: 'BRU-01',
    rack: 'D14',
    status: 'needs-repair',
    notes: 'Faulty PSU — replacement on order.',
    parentId: 'AST-004',
  },
  {
    id: 'PSU-002',
    model: 'HPE 800W Flex Slot Platinum Plus PSU',
    assetTag: 'PSU-002',
    category: 'PSU',
    datacenter: 'BRU-01',
    rack: 'D14',
    status: 'deployed',
    notes: '',
    parentId: 'AST-004',
  },

  // ── Components of SRV-003 (Dell PowerEdge R750) ───────────────────────────
  {
    id: 'CPU-001',
    model: 'Intel Xeon Gold 6338 (32C)',
    assetTag: 'CPU-001',
    category: 'CPU',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: '',
    parentId: 'SRV-003',
  },
  {
    id: 'CPU-002',
    model: 'Intel Xeon Gold 6338 (32C)',
    assetTag: 'CPU-002',
    category: 'CPU',
    datacenter: 'AMS-01',
    rack: 'AMS-01-R01',
    status: 'deployed',
    notes: '',
    parentId: 'SRV-003',
  },
];

type SortableColumn = 'model' | 'category' | 'datacenter' | 'status';

interface FlatRow {
  asset: Asset;
  depth: number;
  hasChildren: boolean;
  isExpanded: boolean;
}

@Component({
  selector: 'app-inventory',
  templateUrl: './inventory.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col min-h-screen bg-white',
    '(document:keydown.escape)': 'closeNotes()',
  },
})
export default class InventoryComponent {
  private readonly inventoryApi = inject(InventoryApiService);

  readonly ITEMS_PER_PAGE = 50;

  // ── Mutable asset list ─────────────────────────────────────────────────────
  readonly mutableAssets = signal([...MOCK_ASSETS]);

  searchQuery = signal('');

  statusFilter = signal<AssetStatus | 'all'>('all');

  categoryFilter = signal<AssetCategory | 'all'>('all');

  locationFilter = signal<string>('all');

  sortColumn = signal<SortableColumn>('status');

  sortDirection = signal<'asc' | 'desc'>('asc');

  currentPage = signal(1);

  activeNotesAsset = signal<Asset | null>(null);

  expandedIds = signal<Set<string>>(
    new Set(
      MOCK_ASSETS.filter((a) => MOCK_ASSETS.some((b) => b.parentId === a.id)).map((a) => a.id),
    ),
  );

  // ── CRUD state ─────────────────────────────────────────────────────────────
  editAsset = signal<Partial<Asset> | null>(null);

  deleteAsset = signal<Asset | null>(null);

  private readonly assetSheetEl = viewChild<ElementRef>('assetSheet');

  private readonly assetModalEl = viewChild<ElementRef>('assetModal');

  private readonly fAssetModel = viewChild<ElementRef>('fAssetModel');

  private readonly fAssetTag = viewChild<ElementRef>('fAssetTag');

  private readonly fAssetCat = viewChild<ElementRef>('fAssetCat');

  private readonly fAssetStatus = viewChild<ElementRef>('fAssetStatus');

  private readonly fAssetDc = viewChild<ElementRef>('fAssetDc');

  private readonly fAssetRack = viewChild<ElementRef>('fAssetRack');

  private readonly fAssetNotes = viewChild<ElementRef>('fAssetNotes');

  constructor() {
    effect(() => {
      const el = this.assetSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editAsset() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.assetModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.deleteAsset() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  readonly categories: AssetCategory[] = [
    'Server',
    'Switch',
    'Storage',
    'Power',
    'Firewall',
    'Cooling',
    'KVM',
    'Other',
    'Memory',
    'Disk',
    'NIC',
    'PSU',
    'CPU',
    'GPU',
    'Transceiver',
  ];

  readonly datacenters = [
    ...new Set(MOCK_ASSETS.filter((a) => !a.parentId).map((a) => a.datacenter)),
  ].sort();

  readonly statuses: { value: AssetStatus; label: string }[] = [
    { value: 'deployed', label: 'Deployed' },
    { value: 'available', label: 'Available' },
    { value: 'on-order', label: 'On Order' },
    { value: 'requested', label: 'Requested' },
    { value: 'needs-repair', label: 'Needs Repair' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  private readonly filtered = computed(() => {
    const q = this.searchQuery().toLowerCase();
    const status = this.statusFilter();
    const category = this.categoryFilter();
    const location = this.locationFilter();

    return this.mutableAssets().filter((a) => {
      if (a.parentId) return false; // sub-assets are shown as children, not in top-level list
      if (status !== 'all' && a.status !== status) return false;
      if (category !== 'all' && a.category !== category) return false;
      if (location !== 'all' && a.datacenter !== location) return false;
      if (q && !a.model.toLowerCase().includes(q) && !a.assetTag.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  });

  private readonly sorted = computed(() => {
    const col = this.sortColumn();
    const dir = this.sortDirection();
    return [...this.filtered()].sort((a, b) => {
      let cmp: number;
      if (col === 'status') {
        cmp = STATUS_SORT_ORDER[a.status] - STATUS_SORT_ORDER[b.status];
      } else {
        cmp = String(a[col]).localeCompare(String(b[col]));
      }
      return dir === 'asc' ? cmp : -cmp;
    });
  });

  readonly totalFiltered = computed(() => this.filtered().length);

  readonly totalPages = computed(() =>
    Math.max(1, Math.ceil(this.totalFiltered() / this.ITEMS_PER_PAGE)),
  );

  private readonly pagedTopLevel = computed(() => {
    const page = this.currentPage();
    const start = (page - 1) * this.ITEMS_PER_PAGE;
    return this.sorted().slice(start, start + this.ITEMS_PER_PAGE);
  });

  private buildChildRows(parentId: string, depth: number, expanded: Set<string>): FlatRow[] {
    const children = this.mutableAssets().filter((a) => a.parentId === parentId);
    const rows: FlatRow[] = [];
    children.forEach((child) => {
      const hasChildren = this.mutableAssets().some((a) => a.parentId === child.id);
      const isExpanded = expanded.has(child.id);
      rows.push({ asset: child, depth, hasChildren, isExpanded });
      if (isExpanded) {
        rows.push(...this.buildChildRows(child.id, depth + 1, expanded));
      }
    });
    return rows;
  }

  readonly flatRows = computed<FlatRow[]>(() => {
    const topLevel = this.pagedTopLevel();
    const expanded = this.expandedIds();
    const rows: FlatRow[] = [];
    topLevel.forEach((asset) => {
      const hasChildren = this.mutableAssets().some((a) => a.parentId === asset.id);
      const isExpanded = expanded.has(asset.id);
      rows.push({ asset, depth: 0, hasChildren, isExpanded });
      if (isExpanded) {
        rows.push(...this.buildChildRows(asset.id, 1, expanded));
      }
    });
    return rows;
  });

  readonly pageStart = computed(() => (this.currentPage() - 1) * this.ITEMS_PER_PAGE + 1);

  readonly pageEnd = computed(() =>
    Math.min(this.currentPage() * this.ITEMS_PER_PAGE, this.totalFiltered()),
  );

  readonly pageNumbers = computed(() => {
    const total = this.totalPages();
    return Array.from({ length: total }, (_, i) => i + 1);
  });

  // Summary counts (top-level assets only, not filtered)
  private readonly topLevelAssets = MOCK_ASSETS.filter((a) => !a.parentId);

  readonly statusCounts = computed(() => {
    const counts: Partial<Record<AssetStatus | 'all', number>> = {
      all: this.topLevelAssets.length,
    };
    this.topLevelAssets.forEach((a) => {
      counts[a.status] = (counts[a.status] ?? 0) + 1;
    });
    return counts;
  });

  readonly categoryCounts = computed(() => {
    const counts: Partial<Record<AssetCategory, number>> = {};
    this.topLevelAssets.forEach((a) => {
      counts[a.category] = (counts[a.category] ?? 0) + 1;
    });
    return counts;
  });

  readonly locationCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.topLevelAssets.forEach((a) => {
      counts[a.datacenter] = (counts[a.datacenter] ?? 0) + 1;
    });
    return counts;
  });

  readonly totalCount = this.topLevelAssets.length;

  readonly deployedCount = this.topLevelAssets.filter((a) => a.status === 'deployed').length;

  readonly availableCount = this.topLevelAssets.filter((a) => a.status === 'available').length;

  readonly issuesCount = this.topLevelAssets.filter(
    (a) => a.status === 'needs-repair' || a.status === 'decommissioned',
  ).length;

  // ── CRUD actions ───────────────────────────────────────────────────────────

  openCreateAsset(): void {
    this.editAsset.set({
      id: '',
      model: '',
      assetTag: '',
      category: 'Server',
      status: 'available',
      datacenter: 'AMS-01',
      rack: '',
      notes: '',
    });
  }

  openEditAsset(asset: Asset, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.editAsset.set({ ...asset });
  }

  closeAssetForm(): void {
    this.editAsset.set(null);
  }

  saveAsset(): void {
    const form = this.editAsset();
    if (!form) return;
    const updated: Asset = {
      id: form.id || `AST-${String(Date.now()).slice(-5)}`,
      model: (this.fAssetModel()?.nativeElement as HTMLInputElement)?.value ?? '',
      assetTag: (this.fAssetTag()?.nativeElement as HTMLInputElement)?.value ?? '',
      category: ((this.fAssetCat()?.nativeElement as HTMLInputElement)?.value ??
        'Server') as AssetCategory,
      status: ((this.fAssetStatus()?.nativeElement as HTMLInputElement)?.value ??
        'available') as AssetStatus,
      datacenter: (this.fAssetDc()?.nativeElement as HTMLInputElement)?.value ?? '',
      rack: (this.fAssetRack()?.nativeElement as HTMLInputElement)?.value ?? '',
      notes: (this.fAssetNotes()?.nativeElement as HTMLInputElement)?.value ?? '',
      parentId: form.parentId,
    };
    if (form.id) {
      firstValueFrom(this.inventoryApi.updateAsset(updated))
        .then(() => {
          this.mutableAssets.update((list) => list.map((a) => (a.id === form.id ? updated : a)));
          this.editAsset.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    } else {
      firstValueFrom(this.inventoryApi.createAsset(updated))
        .then((res) => {
          const created = { ...updated, id: res.asset?.id ?? updated.id };
          this.mutableAssets.update((list) => [...list, created]);
          this.editAsset.set(null);
        })
        // eslint-disable-next-line no-console
        .catch((err) => console.error(connectErrorMessage(err)));
    }
  }

  openDeleteAsset(asset: Asset, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.deleteAsset.set(asset);
  }

  cancelDeleteAsset(): void {
    this.deleteAsset.set(null);
  }

  confirmDeleteAsset(): void {
    const target = this.deleteAsset();
    if (!target) return;
    firstValueFrom(this.inventoryApi.deleteAsset(target.id))
      .then(() => {
        this.mutableAssets.update((list) =>
          list.filter((a) => a.id !== target.id && a.parentId !== target.id),
        );
        this.deleteAsset.set(null);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  toggleExpand(id: string, event: Event) {
    event.stopPropagation();
    this.expandedIds.update((set) => {
      const next = new Set(set);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  toggleSort(col: SortableColumn) {
    if (this.sortColumn() === col) {
      this.sortDirection.update((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      this.sortColumn.set(col);
      this.sortDirection.set('asc');
    }
    this.currentPage.set(1);
  }

  onFilterChange() {
    this.currentPage.set(1);
  }

  goToPage(page: number) {
    if (page >= 1 && page <= this.totalPages()) {
      this.currentPage.set(page);
    }
  }

  statusLabel(status: AssetStatus): string {
    return this.statuses.find((s) => s.value === status)?.label ?? status;
  }

  readonly statusBadgeClass = (status: AssetStatus): string => {
    const map: Record<AssetStatus, string> = {
      'needs-repair': 'bg-amber-50 text-amber-700',
      decommissioned: 'bg-red-50 text-red-600',
      deployed: 'bg-teal-50 text-teal-700',
      available: 'bg-green-50 text-green-700',
      'on-order': 'bg-indigo-50 text-indigo-600',
      requested: 'bg-slate-100 text-slate-600',
    };
    return map[status];
  };

  readonly statusDotClass = (status: AssetStatus): string => {
    const map: Record<AssetStatus, string> = {
      'needs-repair': 'bg-amber-400',
      decommissioned: 'bg-red-400',
      deployed: 'bg-teal-400',
      available: 'bg-green-400',
      'on-order': 'bg-indigo-400',
      requested: 'bg-slate-400',
    };
    return map[status];
  };

  readonly categoryIcon = (category: AssetCategory): string => {
    const map: Partial<Record<AssetCategory, string>> = {
      Server: 'cylinder-split',
      Switch: 'list',
      Storage: 'rectangle-stack',
      Power: 'lock-closed',
      Firewall: 'shield-check-mark',
      Cooling: 'cloud',
      KVM: 'puzzle-piece',
      Other: 'ellipsis',
      Memory: 'folder',
      Disk: 'cylinder-split',
      NIC: 'puzzle-piece',
      PSU: 'lock-closed',
      CPU: 'gear',
      GPU: 'gear',
      Transceiver: 'puzzle-piece',
    };
    return map[category] ?? 'rectangle-stack';
  };

  openNotes(asset: Asset) {
    this.activeNotesAsset.set(asset);
  }

  closeNotes() {
    this.activeNotesAsset.set(null);
  }

  readonly getAssetNotes = (assetId: string): AssetNoteDetail | null => MOCK_NOTES[assetId] ?? null;

  readonly formatDaysAgo = (days: number): string => {
    if (days === 0) return 'Today';
    if (days === 1) return '1 day ago';
    if (days < 30) return `${days} days ago`;
    const months = Math.floor(days / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };
}
