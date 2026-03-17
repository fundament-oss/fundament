import { Component, ChangeDetectionStrategy, input, signal, inject } from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';

/**
 * Demo: custom detail view for SampleItem resources.
 *
 * Demonstrates:
 * - Tabbed layout (Overview / Raw spec) instead of the default field-renderer grid.
 * - A "Scale" action button that opens a modal via PluginModalService.
 *
 * In a real plugin:
 * - Inject PluginResourceStoreService (from the Fundament SDK) to fetch the real resource.
 * - Inject PluginModalService to open the ScaleModal: `modalService.open('demo-ScaleModal', { currentReplicas })`.
 *   PluginModalService is provided in the host's root injector and resolves via Angular DI
 *   automatically because @angular/core is shared as a singleton in the NF configuration.
 */

type Tab = 'overview' | 'raw';

const DEMO_RESOURCE = {
  apiVersion: 'demo.fundament.io/v1',
  kind: 'SampleItem',
  metadata: {
    name: 'web-frontend',
    namespace: 'default',
    uid: 'abc-123',
    creationTimestamp: '2026-01-15T10:00:00Z',
  },
  spec: {
    replicas: 3,
    image: 'nginx:1.25',
    environment: 'production',
    tags: ['web', 'public'],
    enabled: true,
  },
  status: {
    readyReplicas: 3,
    phase: 'Ready',
    message: 'All replicas are ready.',
  },
};

@Component({
  selector: 'app-demo-sample-item-detail',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-4 p-4">
      <!-- Header -->
      <div class="flex items-start justify-between">
        <div>
          <h2 class="text-lg font-semibold dark:text-white">{{ resource().metadata.name }}</h2>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ resource().metadata.namespace }} &bull; {{ resource().kind }}
          </p>
        </div>
        <div class="flex gap-2">
          @if (canWrite()) {
            <button type="button" class="btn-light" (click)="openScaleModal()">Scale</button>
            <button type="button" class="btn-light" (click)="navigateEdit()">Edit</button>
          }
          <button type="button" class="btn-secondary" (click)="navigateBack()">Back</button>
        </div>
      </div>

      <!-- Demo notice -->
      <div
        class="rounded-md border border-blue-200 bg-blue-50 px-4 py-2 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
        role="note"
      >
        Demo plugin loaded via Native Federation. This tabbed layout replaces the default detail view.
      </div>

      <!-- Tab bar -->
      <div class="flex gap-1 border-b border-gray-200 dark:border-gray-700" role="tablist">
        <button
          type="button"
          role="tab"
          [attr.aria-selected]="activeTab() === 'overview'"
          [class]="tabClass('overview')"
          (click)="setTab('overview')"
        >
          Overview
        </button>
        <button
          type="button"
          role="tab"
          [attr.aria-selected]="activeTab() === 'raw'"
          [class]="tabClass('raw')"
          (click)="setTab('raw')"
        >
          Raw spec
        </button>
      </div>

      <!-- Tab panels -->
      @if (activeTab() === 'overview') {
        <div class="card" role="tabpanel" aria-label="Overview">
          <div class="card-header">
            <h3 class="text-sm font-semibold dark:text-white">Status</h3>
          </div>
          <div class="card-body">
            <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-3">
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Phase</dt>
                <dd>
                  <span class="badge badge-green">{{ resource().status.phase }}</span>
                </dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Ready replicas</dt>
                <dd class="font-semibold dark:text-white">{{ resource().status.readyReplicas }}</dd>
              </div>
              <div class="col-span-2 sm:col-span-1">
                <dt class="text-gray-500 dark:text-gray-400">Message</dt>
                <dd class="dark:text-gray-200">{{ resource().status.message }}</dd>
              </div>
            </dl>
          </div>

          <div class="card-header mt-4">
            <h3 class="text-sm font-semibold dark:text-white">Configuration</h3>
          </div>
          <div class="card-body">
            <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-3">
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Replicas</dt>
                <dd class="font-semibold dark:text-white">{{ resource().spec.replicas }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Image</dt>
                <dd class="font-mono text-xs dark:text-gray-200">{{ resource().spec.image }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Environment</dt>
                <dd class="dark:text-gray-200">{{ resource().spec.environment }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Enabled</dt>
                <dd class="dark:text-gray-200">{{ resource().spec.enabled ? 'Yes' : 'No' }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">Tags</dt>
                <dd class="flex flex-wrap gap-1">
                  @for (tag of resource().spec.tags; track tag) {
                    <span class="badge badge-blue">{{ tag }}</span>
                  }
                </dd>
              </div>
            </dl>
          </div>
        </div>
      }

      @if (activeTab() === 'raw') {
        <div class="card" role="tabpanel" aria-label="Raw spec">
          <div class="card-body">
            <pre
              class="overflow-auto rounded-md bg-gray-50 p-4 text-xs text-gray-800 dark:bg-gray-900 dark:text-gray-200"
            ><code>{{ rawSpec() }}</code></pre>
          </div>
        </div>
      }
    </div>
  `,
})
export default class SampleItemDetailComponent {
  /** Passed by the dispatcher; controls edit and scale affordances. */
  canWrite = input<boolean>(false);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  resource = signal(DEMO_RESOURCE);

  activeTab = signal<Tab>('overview');

  rawSpec = signal(JSON.stringify(DEMO_RESOURCE.spec, null, 2));

  setTab(tab: Tab): void {
    this.activeTab.set(tab);
  }

  tabClass(tab: Tab): string {
    const base =
      'px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500';
    return this.activeTab() === tab
      ? `${base} border-b-2 border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400`
      : `${base} text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200`;
  }

  openScaleModal(): void {
    /**
     * In a real plugin, inject PluginModalService and call:
     *   this.modalService.open('demo-ScaleModal', { currentReplicas: this.resource().spec.replicas });
     *
     * PluginModalService is provided in the host's root injector. Because @angular/core is shared
     * as a singleton in the NF config, remote components can inject host services transparently.
     *
     * Import path (for type reference only — resolves from host at runtime):
     *   import PluginModalService from 'fundament-plugin-sdk'; // Phase 2 SDK
     */
    // eslint-disable-next-line no-console
    console.info('[DemoPlugin] Scale modal would open here — wire PluginModalService from SDK.');
  }

  navigateEdit(): void {
    void this.router.navigate(['edit'], { relativeTo: this.route });
  }

  navigateBack(): void {
    void this.router.navigate(['..'], { relativeTo: this.route });
  }
}
