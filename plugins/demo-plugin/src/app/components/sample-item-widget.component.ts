import { Component, ChangeDetectionStrategy, computed, signal } from '@angular/core';

/**
 * Demo: dashboard widget for the cluster dashboard.
 *
 * Demonstrates the `dashboardWidgets` extension point. This widget summarises
 * the SampleItem resource health at a glance.
 *
 * In a real plugin, inject PluginResourceStoreService (from the Fundament SDK) and
 * replace the static DEMO_STATS with live data derived from the cluster's resources.
 */

interface AppStats {
  total: number;
  ready: number;
  pending: number;
  error: number;
}

@Component({
  selector: 'app-demo-sample-item-widget',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="card-body space-y-3">
      <!-- Demo badge -->
      <div class="flex items-center justify-between">
        <span class="text-sm font-semibold dark:text-white">Sample items</span>
        <span class="badge badge-blue">demo</span>
      </div>

      <!-- Total count -->
      <div class="text-3xl font-bold dark:text-white" aria-label="{{ stats().total }} total items">
        {{ stats().total }}
      </div>

      <!-- Breakdown -->
      <div class="flex flex-wrap gap-2 text-xs">
        <span class="inline-flex items-center gap-1 text-green-700 dark:text-green-400">
          <span class="h-2 w-2 rounded-full bg-green-500"></span>
          {{ stats().ready }} ready
        </span>
        <span class="inline-flex items-center gap-1 text-yellow-700 dark:text-yellow-400">
          <span class="h-2 w-2 rounded-full bg-yellow-400"></span>
          {{ stats().pending }} pending
        </span>
        <span class="inline-flex items-center gap-1 text-rose-700 dark:text-rose-400">
          <span class="h-2 w-2 rounded-full bg-rose-500"></span>
          {{ stats().error }} error
        </span>
      </div>

      <!-- Health bar -->
      <div
        class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700"
        role="progressbar"
        [attr.aria-valuenow]="readyPercent()"
        aria-valuemin="0"
        aria-valuemax="100"
        [attr.aria-label]="readyPercent() + '% ready'"
      >
        <div
          class="h-full rounded-full bg-green-500 transition-all"
          [style.width.%]="readyPercent()"
        ></div>
      </div>
    </div>
  `,
})
export default class SampleItemWidgetComponent {
  stats = signal<AppStats>({ total: 4, ready: 2, pending: 1, error: 1 });

  readyPercent = computed(() => {
    const s = this.stats();
    return s.total === 0 ? 0 : Math.round((s.ready / s.total) * 100);
  });
}
