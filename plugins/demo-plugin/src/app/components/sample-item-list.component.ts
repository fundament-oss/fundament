import {
  Component,
  ChangeDetectionStrategy,
  input,
  computed,
  inject,
} from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';
import { SampleItemStoreService } from '../sample-item-store.service';
import type { SampleItem } from '../sample-item-store.service';

/**
 * Demo: custom list view for SampleItem resources.
 *
 * Demonstrates replacing the default table layout with a responsive card grid.
 * In a real plugin, inject PluginResourceStoreService from the Fundament SDK to
 * fetch live resources instead of using the static demo data below.
 */

@Component({
  selector: 'app-demo-sample-item-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-4 p-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold dark:text-white">
          Sample items
          <span class="badge badge-gray ml-2">{{ store.items().length }}</span>
        </h2>
        <button type="button" class="btn-primary" (click)="navigateCreate()">
          New item
        </button>
      </div>

      <!-- Demo notice -->
      <div
        class="rounded-md border border-blue-200 bg-blue-50 px-4 py-2 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
        role="note"
      >
        Demo plugin loaded via Native Federation. This card layout replaces the default table.
      </div>

      <!-- Card grid -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        @for (item of store.items(); track item.name) {
          <div class="card overflow-hidden">
            <div class="card-header flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span
                  class="h-2.5 w-2.5 rounded-full"
                  [class]="statusDot(item.status)"
                  [attr.aria-label]="item.status"
                ></span>
                <span class="font-semibold dark:text-white">{{ item.name }}</span>
              </div>
              <span class="badge" [class]="statusBadge(item.status)">{{ item.status }}</span>
            </div>
            <div class="card-body space-y-1 text-sm text-gray-600 dark:text-gray-400">
              <div class="flex justify-between">
                <span>Replicas</span>
                <span class="font-medium dark:text-gray-200">{{ item.replicas }}</span>
              </div>
              <div class="flex justify-between">
                <span>Namespace</span>
                <span class="font-medium dark:text-gray-200">{{ item.namespace }}</span>
              </div>
              <div class="flex justify-between">
                <span>Image</span>
                <span class="truncate font-mono text-xs dark:text-gray-200">{{ item.image }}</span>
              </div>
            </div>
            <div class="flex gap-2 border-t border-gray-100 px-4 py-2 dark:border-gray-700">
              <button
                type="button"
                class="btn-light text-sm"
                (click)="navigateDetail(item.name, item.namespace)"
              >
                View
              </button>
              @if (canWrite()) {
                <button
                  type="button"
                  class="btn-light text-sm"
                  (click)="navigateEdit(item.name, item.namespace)"
                >
                  Edit
                </button>
              }
            </div>
          </div>
        }
      </div>
    </div>
  `,
})
export default class SampleItemListComponent {
  /** Passed by the dispatcher; controls create/edit affordances. */
  canWrite = input<boolean>(false);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  store = inject(SampleItemStoreService);

  pluginName = computed(() => this.route.snapshot.params['pluginName'] ?? 'demo');

  resourceKind = computed(() => this.route.snapshot.params['resourceKind'] ?? 'sampleitems');

  statusDot(status: SampleItem['status']): string {
    switch (status) {
      case 'Ready':
        return 'bg-green-500';
      case 'Pending':
        return 'bg-yellow-400';
      case 'Error':
        return 'bg-rose-500';
    }
  }

  statusBadge(status: SampleItem['status']): string {
    switch (status) {
      case 'Ready':
        return 'badge-green';
      case 'Pending':
        return 'badge-yellow';
      case 'Error':
        return 'badge-rose';
    }
  }

  navigateCreate(): void {
    void this.router.navigate(['create'], { relativeTo: this.route });
  }

  navigateDetail(name: string, _namespace: string): void {
    void this.router.navigate([name], { relativeTo: this.route });
  }

  navigateEdit(name: string, _namespace: string): void {
    void this.router.navigate([name, 'edit'], { relativeTo: this.route });
  }
}
