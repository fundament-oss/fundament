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
 * The order the categories are listed in, from the things a rack is filled with
 * to the parts that go inside them, with the catch-all last. The catalog and the
 * inventory each kept their own copy and had already drifted: "Other" sat at the
 * bottom in one and halfway up in the other, so the same list read differently
 * on two pages that answer the same question.
 */
export const CATEGORIES: AssetCategory[] = [
  'Server',
  'Switch',
  'Storage',
  'Power',
  'Firewall',
  'Cooling',
  'KVM',
  'Memory',
  'Disk',
  'NIC',
  'PSU',
  'CPU',
  'GPU',
  'Transceiver',
  'Other',
];

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
  Firewall: 'firewall',
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
