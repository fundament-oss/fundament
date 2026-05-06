// ── Types ─────────────────────────────────────────────────────────────────────

export interface PhysicalConnection {
  id: string;
  dcId: string;
  sourcePlacementId: string;
  sourceDeviceLabel: string;
  sourcePortName: string;
  targetPlacementId: string;
  targetDeviceLabel: string;
  targetPortName: string;
  cableAssetId?: string;
}

// ── Mock data ─────────────────────────────────────────────────────────────────

// TODO(api): PhysicalConnectionService.ListPhysicalConnections({ placement_id })
export const MOCK_PHYSICAL_CONNECTIONS: PhysicalConnection[] = [
  {
    id: 'pc-001',
    dcId: 'ams-01',
    sourcePlacementId: 'server-l1',
    sourceDeviceLabel: 'Server L1',
    sourcePortName: 'eth0',
    targetPlacementId: 'switch-l',
    targetDeviceLabel: 'Switch L',
    targetPortName: 'ge-0/0/1',
  },
  {
    id: 'pc-002',
    dcId: 'ams-01',
    sourcePlacementId: 'server-l2',
    sourceDeviceLabel: 'Server L2',
    sourcePortName: 'eth0',
    targetPlacementId: 'switch-l',
    targetDeviceLabel: 'Switch L',
    targetPortName: 'ge-0/0/2',
  },
  {
    id: 'pc-003',
    dcId: 'ams-01',
    sourcePlacementId: 'switch-l',
    sourceDeviceLabel: 'Switch L',
    sourcePortName: 'ge-0/0/24',
    targetPlacementId: 'pp-l',
    targetDeviceLabel: 'Patch Panel L',
    targetPortName: 'port-1',
  },
  {
    id: 'pc-004',
    dcId: 'ams-01',
    sourcePlacementId: 'pp-l',
    sourceDeviceLabel: 'Patch Panel L',
    sourcePortName: 'port-24',
    targetPlacementId: 'pp-r',
    targetDeviceLabel: 'Patch Panel R',
    targetPortName: 'port-1',
  },
  {
    id: 'pc-005',
    dcId: 'ams-01',
    sourcePlacementId: 'server-r1',
    sourceDeviceLabel: 'Server R1',
    sourcePortName: 'eth0',
    targetPlacementId: 'switch-r',
    targetDeviceLabel: 'Switch R',
    targetPortName: 'ge-0/0/1',
  },
];
