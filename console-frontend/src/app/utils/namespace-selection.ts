import { computed, signal } from '@angular/core';

// Reusable multi-select state for the namespace list + bulk-delete UI, shared by
// the cluster and project namespace pages. Construct it with an accessor that
// returns the ids currently shown, so "select all" and the all/indeterminate
// header checkbox stay in sync with the list.
export default class NamespaceSelection {
  private selected = signal<Set<string>>(new Set());

  readonly count = computed(() => this.selected().size);

  readonly allSelected = computed(() => {
    const ids = this.visibleIds();
    const selected = this.selected();
    return ids.length > 0 && ids.every((id) => selected.has(id));
  });

  readonly someSelected = computed(() => this.count() > 0 && !this.allSelected());

  constructor(private visibleIds: () => string[]) {}

  has(id: string): boolean {
    return this.selected().has(id);
  }

  ids(): string[] {
    return [...this.selected()];
  }

  set(id: string, checked: boolean): void {
    this.selected.update((set) => {
      const next = new Set(set);
      if (checked) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  }

  toggleAll(checked: boolean): void {
    this.selected.set(checked ? new Set(this.visibleIds()) : new Set());
  }

  clear(): void {
    this.selected.set(new Set());
  }

  // Drops any selected ids that are no longer present (e.g. after a delete).
  retainVisible(): void {
    const existing = new Set(this.visibleIds());
    this.selected.update((set) => new Set([...set].filter((id) => existing.has(id))));
  }
}
