import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
} from '@angular/core';
import { Cable, CableColor, CableType, CABLE_COLOR_HEX, CABLE_TYPE_LABEL } from '../cable.model';

function groupKey(group: ShoppingGroup): string {
  return `${group.type}|${group.color ?? 'none'}|${group.length ?? '?'}`;
}

function cableLabel(cable: Cable): string {
  if (cable.label) return cable.label;
  return `${cable.aSide.deviceName} → ${cable.bSide.deviceName}`;
}

interface ShoppingGroup {
  type: CableType;
  color: CableColor | undefined;
  length: number | undefined;
  count: number;
  cables: Cable[];
}

@Component({
  selector: 'app-shopping-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './shopping-list.html',
})
export default class ShoppingListComponent {
  readonly cables = input.required<Cable[]>();

  readonly dcLabel = input.required<string>();

  readonly groups = computed<ShoppingGroup[]>(() => {
    const map = this.cables().reduce((acc, cable) => {
      const key = `${cable.type}|${cable.color ?? 'none'}|${cable.length ?? '?'}`;
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
      return CABLE_TYPE_LABEL[a.type].localeCompare(CABLE_TYPE_LABEL[b.type]);
    });
  });

  readonly totalCount = computed(() => this.cables().length);

  readonly groupKey = groupKey;

  readonly cableLabel = cableLabel;

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;

  exportCsv(): void {
    const headers = ['Type', 'Color', 'Length (m)', 'Count', 'Cables'];
    const rows = this.groups().map((g) => [
      CABLE_TYPE_LABEL[g.type],
      g.color ?? '',
      g.length != null ? String(g.length) : '',
      String(g.count),
      g.cables.map((c) => cableLabel(c)).join('; '),
    ]);
    const csvContent = [headers, ...rows]
      .map((row) => row.map((cell) => `"${cell.replace(/"/g, '""')}"`).join(','))
      .join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const slug = this.dcLabel()
      .toLowerCase()
      .replace(/\s+/g, '-')
      .replace(/[^a-z0-9-]/g, '');
    a.download = `shopping-list-${slug}-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }
}
