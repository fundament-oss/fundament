/** The kinds of equipment the catalog and the inventory deal in. */
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

/**
 * The icon for a kind of equipment, in one place. The catalog, the inventory and
 * both their detail pages each carried this table, and they had already drifted
 * apart: memory was a folder in one and a stack of folders in another.
 *
 * Alias names where the alias is the word this app uses (`server`, `memory`,
 * `nic`), the icon's own name where that is already the word (`cpu`, `psu`).
 */
const ICONS: Record<AssetCategory, string> = {
  Server: 'server',
  Switch: 'network-switch',
  Storage: 'storage',
  Power: 'power',
  Firewall: 'shield-check-mark',
  Cooling: 'cooling',
  KVM: 'kvm-switch',
  Memory: 'memory',
  Disk: 'hard-drive',
  NIC: 'nic',
  PSU: 'psu',
  CPU: 'cpu',
  GPU: 'gpu',
  Transceiver: 'transceiver-module',
  Other: 'ellipsis',
};

export default function categoryIcon(category: AssetCategory): string {
  return ICONS[category] ?? 'ellipsis';
}
