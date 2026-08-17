import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
  signal,
  inject,
  viewChild,
  TemplateRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import {
  Cable,
  CableColor,
  CABLE_COLOR_HEX,
  CableStatus,
  CABLE_TYPE_LABEL,
  CableType,
  cableStatusTagColor,
  cableStatusLabel,
  cableTypeLabel,
  PORT_TYPE_LABEL,
} from '../cable.model';
import { PATCH_MAPPING_PATH } from '../patch-mapping-views';
import SecondaryNavService from '../../shell/secondary-nav.service';

interface DeviceOption {
  id: string;
  name: string;
}

interface SiteOption {
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
export default class CableListComponent implements AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  readonly cables = input.required<Cable[]>();

  readonly dcId = input.required<string>();

  readonly sites = input<SiteOption[]>([]);

  readonly plannedCount = input(0);

  readonly editCable = output<Cable>();

  readonly deleteCable = output<Cable>();

  readonly dcSelected = output<string>();

  readonly addCable = output<void>();

  readonly showShoppingList = output<void>();

  readonly showTopology = output<void>();

  readonly searchText = signal('');

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly viewParams = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  /**
   * Whether the address names a view. The section's own path (/patch-mapping) means you
   * have opened the section and picked nothing yet, and then the pane beside
   * the menu says so rather than showing a list you did not ask for.
   */
  readonly hasSelection = computed(() => this.viewParams().get('view') !== null);

  /**
   * Which cables the list is showing, read from the address. The menu is
   * navigation, so a view can be linked to, opened in a second tab and reached
   * with the browser's back button. Which data center you are in is not part of
   * it: that is where you stand, not what you are looking at.
   */
  readonly menuSelection = computed<{ kind: string; value: string }>(() => {
    const params = this.viewParams();
    return { kind: params.get('view') ?? 'all', value: params.get('value') ?? '' };
  });

  private readonly selectionOf = (kind: string) =>
    computed(() => (this.menuSelection().kind === kind ? this.menuSelection().value : ''));

  private readonly statusView = this.selectionOf('status');

  private readonly typeView = this.selectionOf('type');

  private readonly colorView = this.selectionOf('color');

  private readonly deviceView = this.selectionOf('device');

  readonly filterStatus = computed<CableStatus | ''>(
    () => this.CABLE_STATUSES.find((s) => s.value === this.statusView())?.value ?? '',
  );

  readonly filterType = computed<CableType | ''>(
    () => this.CABLE_TYPES.find((t) => t === this.typeView()) ?? '',
  );

  readonly filterColor = computed<CableColor | ''>(
    () => this.CABLE_COLORS.find((c) => c === this.colorView()) ?? '',
  );

  readonly filterDeviceId = computed(() => this.deviceView());

  /**
   * The title of the page is the row you picked in the menu. The section name
   * is already in the menu's own heading and in the way back, so repeating it
   * above the list would say "Patch mapping" three times and never say which
   * cables you are looking at.
   */
  readonly viewTitle = computed(() => {
    const { kind, value } = this.menuSelection();
    switch (kind) {
      case 'status':
        return this.CABLE_STATUSES.find((s) => s.value === value)?.label ?? 'All cables';
      case 'type':
        return this.CABLE_TYPE_LABEL[value as CableType] ?? 'All cables';
      case 'color':
        return this.CABLE_COLORS.includes(value as CableColor)
          ? this.colorLabel(value)
          : 'All cables';
      case 'device':
        return this.dcDevices().find((d) => d.id === value)?.name ?? 'All cables';
      default:
        return 'All cables';
    }
  });

  /** The address of a view, so every row in the menu is a real link. */
  readonly viewPath = (kind: string, value?: string): string =>
    kind === 'all' ? `${PATCH_MAPPING_PATH}/all` : `${PATCH_MAPPING_PATH}/${kind}/${value}`;

  /** A real link, routed in-app unless the click asks for a new tab or window. */
  selectView(event: Event, path: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(path);
  }

  /** Back from the list is back to the menu, so the address says so too. */
  goToMenu(): void {
    this.router.navigateByUrl(PATCH_MAPPING_PATH);
  }

  readonly listSummary = computed(() => {
    const shown = this.filteredCables().length;
    const total = this.cables().length;
    const noun = total === 1 ? 'cable' : 'cables';
    return shown === total ? `${total} ${noun}` : `${shown} of ${total} ${noun}`;
  });

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

  readonly statusCounts = computed(() => {
    const counts: Record<string, number> = { all: this.cables().length };
    this.cables().forEach((c) => {
      const key = c.status ?? 'unspecified';
      counts[key] = (counts[key] ?? 0) + 1;
    });
    return counts;
  });

  readonly typeCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.cables().forEach((c) => {
      const key = c.type ?? 'unspecified';
      counts[key] = (counts[key] ?? 0) + 1;
    });
    return counts;
  });

  /**
   * The types and colors that occur in this data center. A heading with an
   * empty list under it is a group that says nothing, so the menu leaves both
   * out until there is something to pick.
   */
  readonly menuTypes = computed(() => this.CABLE_TYPES.filter((t) => this.typeCounts()[t]));

  readonly menuColors = computed(() => this.CABLE_COLORS.filter((c) => this.colorCounts()[c]));

  readonly colorCounts = computed(() => {
    const counts: Record<string, number> = {};
    this.cables().forEach((c) => {
      if (c.color) counts[c.color] = (counts[c.color] ?? 0) + 1;
    });
    return counts;
  });

  exportCsv(): void {
    const cables = this.filteredCables();
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

  readonly cableStatusTagColor = cableStatusTagColor;

  readonly cableStatusLabel = cableStatusLabel;

  readonly cableTypeLabel = cableTypeLabel;

  /** The kind of cable, or nothing: an unknown kind is not worth a column of
   *  "Unspecified" on every row. */
  readonly knownCableType = (type: CableType | undefined): string =>
    type ? cableTypeLabel(type) : '';

  /** The data center above the list: a radio group, so only the button that
   *  becomes selected has anything to say. */
  onDcToggle(id: string, selected: boolean): void {
    if (selected && id !== this.dcId()) this.dcSelected.emit(id);
  }

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  /** "light-blue" is how the color is stored; "Light blue" is how you read it. */
  readonly colorLabel = (color: string): string => {
    const words = color.replace(/-/g, ' ');
    return words.charAt(0).toUpperCase() + words.slice(1);
  };

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
}
