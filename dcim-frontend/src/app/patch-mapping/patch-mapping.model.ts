// ── Types ─────────────────────────────────────────────────────────────────────

export interface PhysicalConnection {
  id: string;
  dcId: string;
  sourcePlacementId: string;
  sourceDeviceLabel: string;
  sourcePortDefinitionId: string;
  targetPlacementId: string;
  targetDeviceLabel: string;
  targetPortDefinitionId: string;
  cableAssetId?: string;
}
