import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
  signal,
} from '@angular/core';
import {
  Cable,
  CableColor,
  CABLE_COLOR_HEX,
  CableStatus,
  CABLE_STATUS_COLORS,
  CABLE_STATUS_LABEL,
  CABLE_TYPE_LABEL,
  CableType,
  PORT_TYPE_LABEL,
} from '../cable.model';
import { DATACENTER_INFO, DatacenterStatus } from '../../datacenters/datacenter.model';

type SortField = 'label' | 'aSide' | 'bSide' | 'status' | 'type' | null;

interface DeviceOption {
  id: string;
  name: string;
}

@Component({
  selector: 'app-cable-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './cable-list.html',
})
export default class CableListComponent {
  readonly cables = input.required<Cable[]>();

  readonly dcId = input.required<string>();

  readonly editCable = output<Cable>();

  readonly deleteCable = output<Cable>();

  readonly dcSelected = output<string>();

  readonly searchText = signal('');

  readonly filterDeviceId = signal('');

  readonly filterStatus = signal<CableStatus | ''>('');

  readonly filterType = signal<CableType | ''>('');

  readonly filterColor = signal<CableColor | ''>('');

  readonly sortField = signal<SortField>(null);

  readonly sortDir = signal<'asc' | 'desc'>('asc');

  readonly dcDevices = computed<DeviceOption[]>(() => {
    const seen = new Set<string>();
    const result = this.cables()
      .flatMap((c) => [c.aSide, c.bSide])
      .filter((side) => {
        if (seen.has(side.deviceId)) return false;
        seen.add(side.deviceId);
        return true;
      })
      .map((side) => ({ id: side.deviceId, name: side.deviceName }));
    return result.sort((a, b) => a.name.localeCompare(b.name));
  });

  readonly filteredCables = computed(() => {
    const q = this.searchText().toLowerCase();
    const devId = this.filterDeviceId();
    const status = this.filterStatus();
    const type = this.filterType();
    const color = this.filterColor();

    return this.cables().filter((c) => {
      if (status && c.status !== status) return false;
      if (type && c.type !== type) return false;
      if (color && c.color !== color) return false;
      if (devId && c.aSide.deviceId !== devId && c.bSide.deviceId !== devId) return false;
      if (q) {
        const haystack = [
          c.label,
          c.aSide.deviceName,
          c.aSide.portName,
          c.bSide.deviceName,
          c.bSide.portName,
          c.type,
          c.status,
        ]
          .join(' ')
          .toLowerCase();
        if (!haystack.includes(q)) return false;
      }
      return true;
    });
  });

  readonly sortedFilteredCables = computed(() => {
    const list = [...this.filteredCables()];
    const field = this.sortField();
    const dir = this.sortDir();
    if (!field) return list;

    return list.sort((a, b) => {
      let valA: string;
      let valB: string;
      switch (field) {
        case 'label':
          valA = a.label ?? '';
          valB = b.label ?? '';
          break;
        case 'aSide':
          valA = a.aSide.deviceName;
          valB = b.aSide.deviceName;
          break;
        case 'bSide':
          valA = a.bSide.deviceName;
          valB = b.bSide.deviceName;
          break;
        case 'status':
          valA = a.status;
          valB = b.status;
          break;
        case 'type':
          valA = CABLE_TYPE_LABEL[a.type];
          valB = CABLE_TYPE_LABEL[b.type];
          break;
        default:
          return 0;
      }
      const cmp = valA.localeCompare(valB);
      return dir === 'asc' ? cmp : -cmp;
    });
  });

  readonly statusCounts = computed(() => {
    const counts: Record<string, number> = { all: this.cables().length };
    this.cables().forEach((c) => {
      counts[c.status] = (counts[c.status] ?? 0) + 1;
    });
    return counts;
  });

  readonly typeCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.cables().forEach((c) => {
      counts[c.type] = (counts[c.type] ?? 0) + 1;
    });
    return counts;
  });

  readonly colorCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.cables().forEach((c) => {
      if (c.color) counts[c.color] = (counts[c.color] ?? 0) + 1;
    });
    return counts;
  });

  readonly hasActiveFilters = computed(
    () =>
      !!(
        this.filterDeviceId() ||
        this.filterStatus() ||
        this.filterType() ||
        this.filterColor() ||
        this.searchText()
      ),
  );

  readonly DATACENTER_INFO = DATACENTER_INFO;

  readonly dcStatusDotClass = (status: DatacenterStatus): string => {
    const map: Record<DatacenterStatus, string> = {
      operational: 'bg-teal-500',
      degraded: 'bg-amber-500',
      maintenance: 'bg-slate-400',
    };
    return map[status] ?? '';
  };

  readonly statusDotClass = (status: CableStatus): string => {
    const map: Record<CableStatus, string> = {
      planned: 'bg-amber-400',
      connected: 'bg-teal-500',
      decommissioned: 'bg-slate-400',
    };
    return map[status] ?? 'bg-slate-300';
  };

  clearFilters(): void {
    this.filterDeviceId.set('');
    this.filterStatus.set('');
    this.filterType.set('');
    this.filterColor.set('');
    this.searchText.set('');
  }

  toggleSort(field: SortField): void {
    if (this.sortField() === field) {
      this.sortDir.update((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      this.sortField.set(field);
      this.sortDir.set('asc');
    }
  }

  exportCsv(): void {
    const cables = this.sortedFilteredCables();
    const dcId = this.dcId();
    const headers = [
      'ID',
      'Label',
      'A Device',
      'A Port',
      'A Port Type',
      'B Device',
      'B Port',
      'B Port Type',
      'Status',
      'Type',
      'Color',
      'Length (m)',
      'Description',
    ];
    const rows = cables.map((c) => [
      c.id,
      c.label ?? '',
      c.aSide.deviceName,
      c.aSide.portName,
      c.aSide.portType,
      c.bSide.deviceName,
      c.bSide.portName,
      c.bSide.portType,
      c.status,
      c.type,
      c.color ?? '',
      c.length != null ? String(c.length) : '',
      c.description ?? '',
    ]);
    const csvContent = [headers, ...rows]
      .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','))
      .join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `cables-${dcId}-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  readonly CABLE_STATUS_COLORS = CABLE_STATUS_COLORS;

  readonly CABLE_STATUS_LABEL = CABLE_STATUS_LABEL;

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  readonly PORT_TYPE_LABEL = PORT_TYPE_LABEL;

  readonly CABLE_STATUSES: { value: CableStatus; label: string }[] = [
    { value: 'planned', label: 'Planned' },
    { value: 'connected', label: 'Connected' },
    { value: 'decommissioned', label: 'Decommissioned' },
  ];

  readonly CABLE_TYPES: CableType[] = [
    'cat5e',
    'cat6',
    'cat6a',
    'cat7',
    'cat8',
    'dac',
    'aoc',
    'mmf',
    'smf',
    'power',
    'console',
    'usb',
    'other',
  ];

  readonly CABLE_COLORS: CableColor[] = [
    'dark-grey',
    'light-grey',
    'red',
    'green',
    'blue',
    'yellow',
    'purple',
    'orange',
    'teal',
    'white',
  ];

  readonly colorHex = (color: CableColor | undefined): string | null =>
    color ? CABLE_COLOR_HEX[color] : null;

  onDeviceFilterChange(event: Event): void {
    this.filterDeviceId.set((event.target as HTMLSelectElement).value);
  }

  onStatusFilterChange(event: Event): void {
    this.filterStatus.set((event.target as HTMLSelectElement).value as CableStatus | '');
  }

  onTypeFilterChange(event: Event): void {
    this.filterType.set((event.target as HTMLSelectElement).value as CableType | '');
  }
}
