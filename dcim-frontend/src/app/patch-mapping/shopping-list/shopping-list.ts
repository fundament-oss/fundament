import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  signal,
} from '@angular/core';
import { Cable, CableColor, CableType, CABLE_COLOR_HEX, cableTypeLabel } from '../cable.model';
import type { SiteOption } from '../patch-mapping';

/** Above this many locations a row of buttons stops being readable. */
const TOGGLE_LIMIT = 5;

/** What "everywhere" is called, and the value the toggle carries for it. */
const ALL = 'all';

function groupKey(group: ShoppingGroup): string {
  return `${group.type ?? 'none'}|${group.color ?? 'none'}|${group.length ?? '?'}`;
}

/** What a group says under its name: colour and length, or that it has none. */
function groupDetail(group: ShoppingGroup): string {
  const parts: string[] = [];
  if (group.color) parts.push(group.color.replace('-', ' '));
  parts.push(group.length !== undefined ? `${group.length} m` : 'length unknown');
  return parts.join(' · ');
}

function cableLabel(cable: Cable): string {
  if (cable.label) return cable.label;
  return `${cable.aSide.deviceName} → ${cable.bSide.deviceName}`;
}

interface ShoppingGroup {
  type: CableType | undefined;
  color: CableColor | undefined;
  length: number | undefined;
  count: number;
  cables: Cable[];
}

/**
 * What has to be ordered, over every location or in one of them.
 *
 * An order is not a building: you buy the cables for the estate and carry them
 * where they belong. So the scope is a control in here rather than something
 * decided by the page behind it — you can see which it is and change it in the
 * same place.
 */
@Component({
  selector: 'app-shopping-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './shopping-list.html',
})
export default class ShoppingListComponent {
  /**
   * Everything still to be bought, wherever it lies. Narrowed by location here,
   * not by the caller.
   *
   * Only what is left to do: a line you have ordered has been dealt with, and
   * keeping it on the list is the same mistake as leaving milk on a shopping
   * list you have already bought milk for. Where it went is the Ordered view.
   */
  readonly cables = input.required<Cable[]>();

  readonly sites = input.required<SiteOption[]>();

  /** The location on screen, or ALL. */
  readonly scope = signal<string>(ALL);

  /**
   * Points the list at where the page behind it was standing, so it continues
   * that view rather than jumping. Called on every open and not bound to an
   * input: opening twice from the same place has nothing new to say, and an
   * input would then leave the scope wherever you last dragged it.
   */
  openAt(siteId: string): void {
    this.scope.set(siteId || ALL);
  }

  readonly ALL = ALL;

  /** A row of buttons past five reads as a wall; a combo box does not. */
  readonly asCombo = computed(() => this.sites().length > TOGGLE_LIMIT);

  readonly scopeLabel = computed(() => {
    if (this.scope() === ALL) return 'All locations';
    return this.sites().find((site) => site.id === this.scope())?.name ?? '';
  });

  /** The cables the scope leaves. */
  readonly scoped = computed(() => {
    const scope = this.scope();
    if (scope === ALL) return this.cables();
    return this.cables().filter((cable) => cable.dcId === scope);
  });

  /** The site a cable lies in, named. Only worth saying when more than one is
   *  on screen, which is what `showSite` decides. */
  siteName(cable: Cable): string {
    return this.sites().find((site) => site.id === cable.dcId)?.name ?? '';
  }

  readonly showSite = computed(() => this.scope() === ALL && this.sites().length > 1);

  /**
   * One run, on one line.
   *
   * Where it lies reads as part of the sentence rather than as a property under
   * it: "AMS1: NL-00003 → NL-00013" is one fact about one cable, where a second
   * line would make the building look like something else you could act on. Left
   * off entirely when one location is on screen, since then it is the same
   * answer on every row.
   */
  runLabel(cable: Cable): string {
    const label = cableLabel(cable);
    return this.showSite() ? `${this.siteName(cable)}: ${label}` : label;
  }

  /**
   * The kinds of cable to order.
   *
   * Grouped on what you buy — type, colour, length — and not on where it goes,
   * because five of one cable is one line on an order however many buildings
   * they end up in. Which building each run is in is said on the run itself.
   */
  readonly groups = computed<ShoppingGroup[]>(() => {
    const map = this.scoped().reduce((acc, cable) => {
      const key = `${cable.type ?? 'none'}|${cable.color ?? 'none'}|${cable.length ?? '?'}`;
      const existing = acc.get(key);
      if (existing) {
        existing.count += 1;
        existing.cables.push(cable);
      } else {
        acc.set(key, {
          type: cable.type,
          color: cable.color,
          length: cable.length,
          count: 1,
          cables: [cable],
        });
      }
      return acc;
    }, new Map<string, ShoppingGroup>());

    return [...map.values()].sort((a, b) => {
      if (b.count !== a.count) return b.count - a.count;
      return cableTypeLabel(a.type).localeCompare(cableTypeLabel(b.type));
    });
  });

  readonly totalCount = computed(() => this.scoped().length);

  /**
   * How many each scope holds, said on the scope itself.
   *
   * The number belongs on the button you press to get it: with it there, a line
   * under the toggle repeating what the selected one already says is one more
   * thing to read and one more thing to keep in step.
   */
  countFor(siteId: string): number {
    if (siteId === ALL) return this.cables().length;
    return this.cables().filter((cable) => cable.dcId === siteId).length;
  }

  scopeOption(siteId: string, name: string): string {
    return `${name} (${this.countFor(siteId)})`;
  }

  /** How many of a location's cables are planned, for its own empty sentence. */
  readonly emptyLine = computed(() =>
    this.scope() === ALL
      ? 'Nothing is planned anywhere, so there is nothing to order.'
      : `Nothing is planned in ${this.scopeLabel()}, so there is nothing to order.`,
  );

  readonly groupKey = groupKey;

  readonly groupDetail = groupDetail;

  readonly cableLabel = cableLabel;

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  readonly cableTypeLabel = cableTypeLabel;

  onScope(value: string, selected: boolean): void {
    if (selected) this.scope.set(value);
  }

  /** Exports what is on screen, which is why the button says which that is. */
  readonly exportLabel = computed(() => `Download CSV (${this.scopeLabel()})`);

  exportCsv(): void {
    const headers = ['Type', 'Color', 'Length (m)', 'Count', 'Cables'];
    const rows = this.groups().map((g) => [
      cableTypeLabel(g.type),
      g.color ?? '',
      g.length != null ? String(g.length) : '',
      String(g.count),
      g.cables.map((c) => this.runLabel(c)).join('; '),
    ]);
    const csvContent = [headers, ...rows]
      .map((row) => row.map((cell) => `"${cell.replace(/"/g, '""')}"`).join(','))
      .join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const slug = this.scopeLabel()
      .toLowerCase()
      .replace(/\s+/g, '-')
      .replace(/[^a-z0-9-]/g, '');
    a.download = `shopping-list-${slug}-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }
}
