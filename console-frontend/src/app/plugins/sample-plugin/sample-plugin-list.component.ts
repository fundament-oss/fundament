import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerEye,
  tablerPencil,
  tablerTrash,
  tablerAlertTriangle,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../../modal/modal.component';
import PluginRegistryService from '../../plugin-resources/plugin-registry.service';
import PluginResourceStoreService from '../../plugin-resources/plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import { TitleService } from '../../title.service';
import type { KubeResource } from '../../plugin-resources/types';
import { resolveStatusBadge, kindToSingularLabel } from '../../plugin-resources/crd-schema.utils';

/**
 * Custom list view for SamplePlugin resources.
 *
 * Shows resources as a responsive card grid instead of the default table layout.
 * This component demonstrates how plugin authors can replace the auto-generated UI
 * by registering a named component in src/app/plugins/index.ts and referencing it
 * in the plugin YAML's `customComponents` section.
 */
@Component({
  selector: 'app-sample-plugin-list',
  standalone: true,
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({ tablerPlus, tablerEye, tablerPencil, tablerTrash, tablerAlertTriangle }),
  ],
  template: `
    <div class="card">
      <div class="card-header">
        <h1 class="text-2xl font-bold dark:text-white">Sample Plugins</h1>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
          Sample plugin with a custom list view and custom detail view.
        </p>
      </div>

      <div class="p-6">
        @if (menuItem()?.create && resources().length > 0) {
          <div class="mb-6">
            <a [routerLink]="['.', 'create']" class="btn-primary inline-flex items-center">
              <ng-icon name="tablerPlus" class="mr-1.5" />
              Create sample item
            </a>
          </div>
        }

        @if (resources().length > 0) {
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            @for (app of resources(); track app.metadata.uid) {
              <div class="card flex flex-col">
                <!-- Card header -->
                <div class="card-header flex items-start justify-between gap-2">
                  <div class="min-w-0 flex-1">
                    @if (menuItem()?.detail) {
                      <a
                        [routerLink]="['.', app.metadata.uid]"
                        class="truncate font-semibold hover:underline dark:text-white"
                      >
                        {{ app.metadata.name }}
                      </a>
                    } @else {
                      <span class="truncate font-semibold dark:text-white">
                        {{ app.metadata.name }}
                      </span>
                    }
                    @if (app.metadata.namespace) {
                      <div class="mt-0.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                        {{ app.metadata.namespace }}
                      </div>
                    }
                  </div>
                  @if (statusBadge(app); as badge) {
                    <span [class]="badge.badge">{{ badge.label }}</span>
                  }
                </div>

                <!-- Card body -->
                <div class="card-body flex-1 space-y-2 text-sm">
                  <div class="flex items-start gap-2">
                    <span class="w-16 shrink-0 text-gray-500 dark:text-gray-400">Image</span>
                    <span class="min-w-0 truncate font-mono text-gray-900 dark:text-gray-100">
                      {{ app.spec['image'] ?? '—' }}
                    </span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="w-16 shrink-0 text-gray-500 dark:text-gray-400">Replicas</span>
                    <span class="text-gray-900 dark:text-gray-100">
                      {{ app.spec['replicas'] ?? 1 }}
                    </span>
                  </div>
                  @if (app.spec['port']) {
                    <div class="flex items-center gap-2">
                      <span class="w-16 shrink-0 text-gray-500 dark:text-gray-400">Port</span>
                      <span class="font-mono text-gray-900 dark:text-gray-100">
                        {{ app.spec['port'] }}
                      </span>
                    </div>
                  }
                </div>

                <!-- Card footer actions -->
                <div
                  class="card-body-sm flex justify-end gap-1 border-t border-gray-200 dark:border-gray-800"
                >
                  @if (menuItem()?.detail) {
                    <a
                      [routerLink]="['.', app.metadata.uid]"
                      class="btn-light inline-flex shrink-0 items-center px-2!"
                      aria-label="View"
                      title="View"
                    >
                      <ng-icon name="tablerEye" />
                    </a>
                  }
                  @if (menuItem()?.create) {
                    <a
                      [routerLink]="['.', app.metadata.uid, 'edit']"
                      class="btn-light inline-flex shrink-0 items-center px-2!"
                      aria-label="Edit"
                      title="Edit"
                    >
                      <ng-icon name="tablerPencil" />
                    </a>
                  }
                  <button
                    type="button"
                    (click)="openDeleteModal(app)"
                    class="btn-remove inline-flex shrink-0 items-center px-2!"
                    aria-label="Delete"
                    title="Delete"
                  >
                    <ng-icon name="tablerTrash" />
                  </button>
                </div>
              </div>
            }
          </div>
        }

        @if (resources().length === 0) {
          <div class="py-12 text-center">
            <p class="text-gray-600 dark:text-gray-300">No sample items found</p>
            @if (menuItem()?.create) {
              <a [routerLink]="['.', 'create']" class="btn-primary mt-4 inline-flex items-center">
                <ng-icon name="tablerPlus" class="mr-1.5" />
                Create sample item
              </a>
            }
          </div>
        }
      </div>
    </div>

    <!-- Delete Modal -->
    <app-modal
      [show]="showDeleteModal()"
      title="Delete sample plugin"
      [maxWidth]="'max-w-lg'"
      (modalClose)="showDeleteModal.set(false)"
    >
      <div class="sm:flex sm:items-start">
        <div
          class="mx-auto flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-rose-100 sm:mx-0 sm:h-10 sm:w-10 dark:bg-rose-900/20"
        >
          <ng-icon
            name="tablerAlertTriangle"
            size="1.5rem"
            class="text-rose-600! dark:text-rose-400!"
          />
        </div>
        <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
          <div class="mt-2">
            <p class="text-sm text-gray-600 dark:text-gray-400">
              Are you sure you want to delete
              <span class="font-semibold text-gray-800 dark:text-white">{{
                pendingDeleteName()
              }}</span
              >?
            </p>
          </div>
        </div>
      </div>
      <div modal-footer class="modal-footer">
        <button type="button" (click)="showDeleteModal.set(false)" class="btn-secondary">
          Cancel
        </button>
        <button
          type="button"
          (click)="confirmDelete()"
          class="cursor-pointer items-center rounded-md border border-rose-700 bg-rose-600 px-4 py-2 text-sm font-medium text-white shadow-sm ring-rose-500 hover:bg-rose-700 focus:ring-2 focus:ring-rose-500 focus:ring-offset-2 focus:outline-none dark:border-rose-800 dark:bg-rose-700 dark:ring-offset-gray-950 dark:hover:bg-rose-800"
        >
          Yes, delete
        </button>
      </div>
    </app-modal>
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class SamplePluginListComponent {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private toastService = inject(ToastService);

  private titleService = inject(TitleService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  private crdDef = computed(() => this.registry.getCrdByPlural(this.pluginName(), 'sampleitems'));

  menuItem = computed(() => {
    const p = this.plugin();
    const crd = this.crdDef();
    if (!p || !crd) return undefined;
    const allItems = [...(p.menu.organization ?? []), ...(p.menu.project ?? [])];
    return allItems.find((item) => item.crd === crd.kind);
  });

  resources = computed<KubeResource[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return this.store.listResources(this.pluginName(), crd.kind);
  });

  showDeleteModal = signal(false);

  pendingDeleteUid = signal('');

  pendingDeleteName = signal('');

  constructor() {
    effect(() => {
      this.titleService.setTitle('Sample Plugins');
    });
  }

  statusBadge(resource: KubeResource): { badge: string; label: string } | undefined {
    const p = this.plugin();
    const crd = this.crdDef();
    if (!p?.uiHints || !crd) return undefined;
    return resolveStatusBadge(resource, p.uiHints[crd.kind]?.statusMapping);
  }

  openDeleteModal(resource: KubeResource): void {
    this.pendingDeleteUid.set(resource.metadata.uid);
    this.pendingDeleteName.set(resource.metadata.name);
    this.showDeleteModal.set(true);
  }

  confirmDelete(): void {
    const crd = this.crdDef();
    if (!crd) return;
    this.store.deleteResource(this.pluginName(), crd.kind, this.pendingDeleteUid());
    this.showDeleteModal.set(false);
    this.toastService.show(`${kindToSingularLabel(crd.kind)} deleted`, 'success');
  }
}
