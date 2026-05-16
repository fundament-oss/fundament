import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  signal,
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
  expanded: boolean;
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

  readonly expandedKeys = signal(new Set<string>());

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
          expanded: false,
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

  isExpanded(group: ShoppingGroup): boolean {
    return this.expandedKeys().has(groupKey(group));
  }

  toggleExpanded(group: ShoppingGroup): void {
    const key = groupKey(group);
    this.expandedKeys.update((set) => {
      const next = new Set(set);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  readonly cableLabel = cableLabel;

  readonly CABLE_COLOR_HEX = CABLE_COLOR_HEX;

  readonly CABLE_TYPE_LABEL = CABLE_TYPE_LABEL;
}
