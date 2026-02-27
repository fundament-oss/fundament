import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerAlertTriangle,
  tablerArrowLeft,
  tablerPencil,
  tablerTrash,
  tablerServer,
  tablerCopy,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../../modal/modal.component';
import PluginRegistryService from '../../plugin-resources/plugin-registry.service';
import PluginResourceStoreService from '../../plugin-resources/plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import { TitleService } from '../../title.service';
import { resolveStatusBadge, formatDate } from '../../plugin-resources/crd-schema.utils';

/**
 * Custom detail view for a single SamplePlugin resource.
 *
 * Demonstrates a more visually rich layout compared to the auto-generated detail view.
 */
@Component({
  selector: 'app-sample-plugin-detail',
  standalone: true,
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerAlertTriangle,
      tablerArrowLeft,
      tablerPencil,
      tablerTrash,
      tablerServer,
      tablerCopy,
    }),
  ],
  template: `
    @if (resource(); as app) {
      <!-- Breadcrumb back link -->
      <div class="mb-4">
        <a
          [routerLink]="['..']"
          class="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-indigo-600 dark:text-gray-400 dark:hover:text-indigo-400"
        >
          <ng-icon name="tablerArrowLeft" />
          Back to list
        </a>
      </div>

      <!-- Hero card -->
      <div class="card mb-4">
        <div class="card-header flex flex-wrap items-start justify-between gap-4">
          <div class="flex items-center gap-3">
            <div
              class="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-100 dark:bg-indigo-900/40"
            >
              <ng-icon
                name="tablerServer"
                size="1.25rem"
                class="text-indigo-600! dark:text-indigo-400!"
              />
            </div>
            <div>
              <h1 class="text-xl font-bold dark:text-white">{{ app.metadata.name }}</h1>
              @if (app.metadata.namespace) {
                <p class="font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ app.metadata.namespace }}
                </p>
              }
            </div>
          </div>
          <div class="flex items-center gap-2">
            @if (statusBadge(); as badge) {
              <span [class]="badge.badge">{{ badge.label }}</span>
            }
            <a [routerLink]="['edit']" class="btn-light inline-flex items-center gap-1.5">
              <ng-icon name="tablerPencil" />
              Edit
            </a>
            <button
              type="button"
              (click)="showDeleteModal.set(true)"
              class="btn-remove inline-flex items-center gap-1.5"
            >
              <ng-icon name="tablerTrash" />
              Delete
            </button>
          </div>
        </div>

        <!-- Quick stats row -->
        <div
          class="grid grid-cols-2 divide-x divide-gray-200 border-t border-gray-200 sm:grid-cols-3 dark:divide-gray-800 dark:border-gray-800"
        >
          <div class="px-6 py-4">
            <p class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400">
              Replicas
            </p>
            <p class="mt-1 text-2xl font-bold dark:text-white">{{ app.spec['replicas'] ?? 1 }}</p>
          </div>
          <div class="px-6 py-4">
            <p class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400">
              Port
            </p>
            <p class="mt-1 text-2xl font-bold dark:text-white">
              {{ app.spec['port'] ?? '—' }}
            </p>
          </div>
          <div class="col-span-2 px-6 py-4 sm:col-span-1">
            <p class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400">
              Created
            </p>
            <p class="mt-1 text-sm font-medium dark:text-white">
              {{ formatDate(app.metadata.creationTimestamp) }}
            </p>
          </div>
        </div>
      </div>

      <!-- Configuration card -->
      <div class="card mb-4">
        <div class="card-header">
          <h2 class="font-semibold dark:text-white">Configuration</h2>
        </div>
        <div class="card-body divide-y divide-gray-100 dark:divide-gray-900">
          <div class="flex items-start gap-4 py-3">
            <span class="w-28 shrink-0 text-sm text-gray-500 dark:text-gray-400">Image</span>
            <div class="flex min-w-0 flex-1 items-center gap-2">
              <code
                class="min-w-0 truncate rounded bg-gray-100 px-2 py-0.5 font-mono text-sm dark:bg-gray-800 dark:text-gray-200"
              >
                {{ app.spec['image'] ?? '—' }}
              </code>
              <button
                type="button"
                title="Copy image"
                (click)="copyToClipboard(app.spec['image'])"
                class="shrink-0 cursor-pointer text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              >
                <ng-icon name="tablerCopy" />
              </button>
            </div>
          </div>
          <div class="flex items-center gap-4 py-3">
            <span class="w-28 shrink-0 text-sm text-gray-500 dark:text-gray-400">Replicas</span>
            <span class="text-sm dark:text-white">{{ app.spec['replicas'] ?? 1 }}</span>
          </div>
          <div class="flex items-center gap-4 py-3">
            <span class="w-28 shrink-0 text-sm text-gray-500 dark:text-gray-400">Port</span>
            <span class="text-sm dark:text-white">{{ app.spec['port'] ?? '—' }}</span>
          </div>
        </div>
      </div>

      <!-- Status card -->
      @if (app.status && statusPhase()) {
        <div class="card">
          <div class="card-header">
            <h2 class="font-semibold dark:text-white">Status</h2>
          </div>
          <div class="card-body">
            <div class="flex items-center gap-3">
              @if (statusBadge(); as badge) {
                <span [class]="badge.badge">{{ badge.label }}</span>
              }
              <span class="text-sm text-gray-600 dark:text-gray-400">{{ statusPhase() }}</span>
            </div>
          </div>
        </div>
      }
    } @else {
      <div class="card p-6 text-center text-gray-500 dark:text-gray-400">Resource not found.</div>
    }

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
                resource()?.metadata?.name
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
export default class SamplePluginDetailComponent {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private toastService = inject(ToastService);

  private titleService = inject(TitleService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  private crdDef = computed(() => this.registry.getCrdByPlural(this.pluginName(), 'sampleitems'));

  resource = computed(() => {
    const crd = this.crdDef();
    if (!crd) return undefined;
    return this.store.getResource(this.pluginName(), crd.kind, this.resourceId());
  });

  statusBadge = computed(() => {
    const r = this.resource();
    const p = this.plugin();
    const crd = this.crdDef();
    if (!r || !p?.uiHints || !crd) return undefined;
    return resolveStatusBadge(r, p.uiHints[crd.kind]?.statusMapping);
  });

  statusPhase = computed(() => {
    const r = this.resource();
    return r?.status?.['phase'] as string | undefined;
  });

  showDeleteModal = signal(false);

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(r?.metadata.name);
    });
  }

  formatDate = formatDate;

  copyToClipboard(value: unknown): void {
    if (typeof value === 'string') {
      navigator.clipboard.writeText(value).then(() => {
        this.toastService.show('Copied to clipboard', 'success');
      });
    }
  }

  confirmDelete(): void {
    const crd = this.crdDef();
    if (!crd) return;
    this.store.deleteResource(this.pluginName(), crd.kind, this.resourceId());
    this.showDeleteModal.set(false);
    this.toastService.show('Sample plugin deleted', 'success');
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
