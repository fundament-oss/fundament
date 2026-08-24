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
  CABLE_STATUSES,
  CableStatus,
  CABLE_TYPE_LABEL,
  CableType,
  cableStatusTagColor,
  cableStatusLabel,
  cableTypeLabel,
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

  readonly editCable = output<Cable>();

  readonly deleteCable = output<Cable>();

  readonly cableStatusChanged = output<{ cableId: string; status: CableStatus | undefined }>();

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

  /** A cable can have no status at all, so that is a view of its own: without
   *  it those cables sit behind none of the three. */
  readonly filterUnspecified = computed(() => this.statusView() === 'unspecified');


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
        if (value === 'unspecified') return 'Unspecified';
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

  /**
   * Which building you are standing in, said under the title of the view.
   *
   * The data center is picked in the menu, and on a narrow screen that menu is
   * a pane you have left behind: the rows would then be from somewhere, with
   * nothing on screen saying where. Worth having on a wide screen too, so the
   * answer is next to what it is about instead of two panes to the left.
   */
  readonly dcLabel = computed(() => {
    if (!this.dcId()) return 'All locations';
    return this.sites().find((site) => site.id === this.dcId())?.name ?? '';
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

  /**
   * How many rows are left, and out of how many.
   *
   * "of" is only true of the search, because that is the only thing narrowing
   * what the view already picked. Measured against this view and not against
   * every cable in the data center: on Planned there are not eight to be three
   * of, and saying so reads as a filter that failed.
   */
  readonly listSummary = computed(() => {
    const shown = this.filteredCables().length;
    const inView = this.viewCables().length;
    // The noun agrees with the number it follows, which in "1 of 3 cables" is
    // the three.
    const noun = inView === 1 ? 'cable' : 'cables';
    return shown === inView ? `${shown} ${noun}` : `${shown} of ${inView} ${noun}`;
  });

  /**
   * What an empty list means, which is not the same thing every time.
   *
   * With something typed in the search box there is an adjustment to offer.
   * With nothing typed there is not: the view is the filter, and no control on
   * screen would make cables appear. Telling somebody to adjust their filters
   * there sends them looking for a knob that is not on the page.
   */
  readonly emptyState = computed<{ text: string; supporting: string }>(() => {
    if (this.searchText().trim()) {
      return { text: 'No cables found', supporting: 'Nothing in this view matches your search.' };
    }

    const { kind, value } = this.menuSelection();
    if (kind !== 'status') {
      if (kind === 'all') {
        return { text: 'No cables yet', supporting: 'Add the first one to this data center.' };
      }
      return { text: 'No cables here', supporting: 'This data center has none of these.' };
    }

    switch (value) {
      case 'unspecified':
        // An empty list here is the good outcome, so it does not read as a
        // dead end: there is nothing left for anybody to classify.
        return { text: 'Every cable has a status', supporting: 'Nothing is left to classify.' };
      case 'to-order':
        return { text: 'Nothing to order', supporting: 'Everything here has been bought.' };
      case 'ordered':
        return {
          text: 'Nothing on order',
          supporting: 'Tick a line in the shopping list once you have ordered it.',
        };
      case 'ready-to-install':
        return {
          text: 'Nothing waiting to be fitted',
          supporting: 'Cables show up here once you mark them as arrived.',
        };
      default:
        return { text: 'No cables here', supporting: 'This data center has none of these.' };
    }
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

  /** What the view picked, before the search box says anything about it. */
  readonly viewCables = computed(() => {
    const devId = this.filterDeviceId();
    const status = this.filterStatus();
    const type = this.filterType();
    const color = this.filterColor();
    const unspecified = this.filterUnspecified();

    return this.cables().filter((c) => {
      if (status && c.status !== status) return false;
      if (unspecified && c.status) return false;
      if (type && c.type !== type) return false;
      if (color && c.color !== color) return false;
      if (devId && c.aSide.deviceId !== devId && c.bSide.deviceId !== devId) return false;
      return true;
    });
  });

  readonly filteredCables = computed(() => {
    const q = this.searchText().toLowerCase();
    if (!q) return this.viewCables();

    return this.viewCables().filter((c) => {
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
      return haystack.includes(q);
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

  /**
   * The run a cable makes, as one sentence.
   *
   * The port is named and its type is not: in a rack every port is a network
   * interface until one is not, and the type column says so on the run where it
   * matters. "NIC 1" and "Port 4" are what is printed on the device anyway.
   */
  readonly runLabel = (cable: Cable): string =>
    `${cable.aSide.deviceName} (${cable.aSide.portName}) → ${cable.bSide.deviceName} (${cable.bSide.portName})`;


  readonly CABLE_STATUSES = CABLE_STATUSES;

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
