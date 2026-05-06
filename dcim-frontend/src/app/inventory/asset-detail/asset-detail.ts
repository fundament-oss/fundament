import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  signal,
} from '@angular/core';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import {
  Asset,
  AssetCategory,
  AssetStatus,
  CatalogEntry,
  HistoryEntry,
  MOCK_ASSETS,
  MOCK_CATALOG,
  MOCK_HISTORY,
  MOCK_NOTES,
  AssetNoteDetail,
} from '../inventory';

interface AssetExtraDetail {
  serial: string;
  manufacturer: string;
  purchaseDate: string;
  purchaseCost: string;
  warrantyExpires: string;
  supportContract: string;
}

const MOCK_EXTRA_DETAILS: Record<string, AssetExtraDetail> = {
  'AST-001': {
    serial: 'SN-DELL-R750-00A12X',
    manufacturer: 'Dell Technologies',
    purchaseDate: '2024-03-15',
    purchaseCost: '€ 18.450',
    warrantyExpires: '2027-03-15',
    supportContract: 'ProSupport Plus 3yr',
  },
  'AST-002': {
    serial: 'SN-CSC-9300-B05YZ',
    manufacturer: 'Cisco Systems',
    purchaseDate: '2023-11-20',
    purchaseCost: '€ 9.200',
    warrantyExpires: '2026-11-20',
    supportContract: 'SmartNet 3yr',
  },
  'AST-003': {
    serial: 'SN-NTAP-A800-C08AB',
    manufacturer: 'NetApp',
    purchaseDate: '2025-01-08',
    purchaseCost: '€ 124.000',
    warrantyExpires: '2028-01-08',
    supportContract: 'SupportEdge Premium 3yr',
  },
  'AST-004': {
    serial: 'SN-HPE-DL380-D14CC',
    manufacturer: 'Hewlett Packard Enterprise',
    purchaseDate: '2022-07-10',
    purchaseCost: '€ 14.700',
    warrantyExpires: '2025-07-10',
    supportContract: 'HPE Foundation Care 3yr',
  },
  'AST-007': {
    serial: 'SN-DELL-R650-A13QR',
    manufacturer: 'Dell Technologies',
    purchaseDate: '2024-06-01',
    purchaseCost: '€ 11.800',
    warrantyExpires: '2027-06-01',
    supportContract: 'ProSupport Plus 3yr',
  },
  'AST-008': {
    serial: 'SN-PA-5250-F01MN',
    manufacturer: 'Palo Alto Networks',
    purchaseDate: '2023-09-05',
    purchaseCost: '€ 42.000',
    warrantyExpires: '2026-09-05',
    supportContract: 'Premium Support 3yr',
  },
  'AST-009': {
    serial: 'SN-PURE-X70-C04KL',
    manufacturer: 'Pure Storage',
    purchaseDate: '2024-01-22',
    purchaseCost: '€ 87.500',
    warrantyExpires: '2027-01-22',
    supportContract: 'Evergreen//One',
  },
  'AST-012': {
    serial: 'SN-ARIS-7050-B01PQ',
    manufacturer: 'Arista Networks',
    purchaseDate: '2023-04-14',
    purchaseCost: '€ 31.200',
    warrantyExpires: '2026-04-14',
    supportContract: 'Arista TAC 3yr',
  },
  'AST-013': {
    serial: 'SN-LNV-SR650-A05RR',
    manufacturer: 'Lenovo',
    purchaseDate: '2021-12-03',
    purchaseCost: '€ 12.600',
    warrantyExpires: '2024-12-03',
    supportContract: 'Foundation Service 3yr',
  },
  'AST-018': {
    serial: 'SN-FTN-FG600-F02ST',
    manufacturer: 'Fortinet',
    purchaseDate: '2023-08-17',
    purchaseCost: '€ 28.900',
    warrantyExpires: '2026-08-17',
    supportContract: 'FortiCare 360 3yr',
  },
};

@Component({
  selector: 'app-asset-detail',
  templateUrl: './asset-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { class: 'block bg-slate-50 min-h-screen' },
})
export default class AssetDetailComponent {
  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  readonly assetId = computed(() => this.route.snapshot.paramMap.get('id') ?? '');

  readonly asset = computed<Asset | undefined>(() =>
    MOCK_ASSETS.find((a) => a.id === this.assetId()),
  );

  readonly parentAsset = computed<Asset | undefined>(() => {
    const parentId = this.asset()?.parentId;
    return parentId ? MOCK_ASSETS.find((a) => a.id === parentId) : undefined;
  });

  readonly childAssets = computed<Asset[]>(() =>
    MOCK_ASSETS.filter((a) => a.parentId === this.assetId()),
  );

  readonly assetHistory = computed<HistoryEntry[]>(() => MOCK_HISTORY[this.assetId()] ?? []);

  readonly catalogEntry = computed<CatalogEntry | undefined>(() =>
    MOCK_CATALOG.find((e) => e.model === this.asset()?.model),
  );

  readonly extraDetail = computed<AssetExtraDetail | undefined>(
    () => MOCK_EXTRA_DETAILS[this.assetId()],
  );

  readonly noteDetail = computed<AssetNoteDetail | undefined>(() => MOCK_NOTES[this.assetId()]);

  readonly newNoteText = signal('');

  readonly statusLabel = (status: AssetStatus): string => {
    const labels: Record<AssetStatus, string> = {
      deployed: 'Deployed',
      available: 'Available',
      'needs-repair': 'Needs Repair',
      decommissioned: 'Decommissioned',
      'on-order': 'On Order',
      requested: 'Requested',
    };
    return labels[status];
  };

  readonly statusBadgeClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-50 text-teal-700',
      available: 'bg-green-50 text-green-700',
      'needs-repair': 'bg-amber-50 text-amber-700',
      decommissioned: 'bg-slate-100 text-slate-500',
      'on-order': 'bg-blue-50 text-blue-700',
      requested: 'bg-purple-50 text-purple-700',
    };
    return classes[status];
  };

  readonly statusDotClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-500',
      available: 'bg-green-500',
      'needs-repair': 'bg-amber-500',
      decommissioned: 'bg-slate-400',
      'on-order': 'bg-blue-500',
      requested: 'bg-purple-500',
    };
    return classes[status];
  };

  readonly statusIcon = (status: AssetStatus): string => {
    const icons: Record<AssetStatus, string> = {
      deployed: 'check-mark-circle',
      available: 'check-mark-circle',
      'needs-repair': 'exclamation-triangle',
      decommissioned: 'slash-circle',
      'on-order': 'arrow-right',
      requested: 'clock-arrow-counter-clockwise',
    };
    return icons[status];
  };

  readonly statusIconColor = (status: AssetStatus): string => {
    const colors: Record<AssetStatus, string> = {
      deployed: 'text-teal-500',
      available: 'text-green-500',
      'needs-repair': 'text-amber-500',
      decommissioned: 'text-slate-400',
      'on-order': 'text-blue-500',
      requested: 'text-purple-500',
    };
    return colors[status];
  };

  readonly statusIconBgClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-50',
      available: 'bg-green-50',
      'needs-repair': 'bg-amber-50',
      decommissioned: 'bg-slate-100',
      'on-order': 'bg-blue-50',
      requested: 'bg-purple-50',
    };
    return `flex h-14 w-14 items-center justify-center rounded-full ${classes[status]}`;
  };

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return 'Yesterday';
    if (daysAgo < 30) return `${daysAgo} days ago`;
    const months = Math.floor(daysAgo / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };

  readonly historyIcon = (action: HistoryEntry['action']): string => {
    const icons: Record<HistoryEntry['action'], string> = {
      'status-change': 'tag',
      'location-change': 'info-circle',
      maintenance: 'gear',
    };
    return icons[action];
  };

  readonly historyIconBg = (action: HistoryEntry['action']): string => {
    const classes: Record<HistoryEntry['action'], string> = {
      'status-change': 'bg-indigo-50 text-indigo-500',
      'location-change': 'bg-sky-50 text-sky-500',
      maintenance: 'bg-amber-50 text-amber-500',
    };
    return classes[action];
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
      Memory: 'folder-stack',
      Disk: 'cylinder-split',
      NIC: 'puzzle-piece',
      PSU: 'lock-closed',
      CPU: 'gear',
      GPU: 'gear',
      Transceiver: 'puzzle-piece',
    };
    return map[category] ?? 'rectangle-stack';
  };
}
