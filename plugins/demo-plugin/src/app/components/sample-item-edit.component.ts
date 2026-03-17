import { Component, ChangeDetectionStrategy, input, signal, computed, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, ActivatedRoute } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { SampleItemStoreService } from '../sample-item-store.service';

/**
 * Demo: custom edit view for SampleItem resources.
 *
 * Demonstrates the `edit` extension point. Reads the item from the shared
 * in-memory SampleItemStoreService and saves changes back on submit.
 *
 * In a real plugin, inject PluginResourceStoreService (from the Fundament SDK)
 * to fetch and patch the real resource in the cluster.
 */

@Component({
  selector: 'app-demo-sample-item-edit',
  imports: [FormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-6 p-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold dark:text-white">Edit sample item</h2>
        <button type="button" class="btn-secondary" (click)="navigateBack()">Cancel</button>
      </div>

      <!-- Demo notice -->
      <div
        class="rounded-md border border-blue-200 bg-blue-50 px-4 py-2 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
        role="note"
      >
        Demo plugin edit view. Changes are stored in memory and reset on page refresh.
      </div>

      @if (item()) {
        <form class="card space-y-4 p-4" (ngSubmit)="onSubmit()">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <!-- Name (read-only) -->
            <div class="flex flex-col gap-1">
              <label class="text-sm font-medium dark:text-white">Name</label>
              <p class="input bg-gray-50 text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ item()!.name }}
              </p>
            </div>

            <!-- Namespace (read-only) -->
            <div class="flex flex-col gap-1">
              <label class="text-sm font-medium dark:text-white">Namespace</label>
              <p class="input bg-gray-50 text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                {{ item()!.namespace }}
              </p>
            </div>

            <!-- Image -->
            <div class="flex flex-col gap-1">
              <label for="edit-image" class="text-sm font-medium dark:text-white">Image</label>
              <input
                id="edit-image"
                type="text"
                class="input"
                required
                placeholder="nginx:latest"
                [(ngModel)]="form().image"
                name="image"
              />
            </div>

            <!-- Replicas -->
            <div class="flex flex-col gap-1">
              <label for="edit-replicas" class="text-sm font-medium dark:text-white">Replicas</label>
              <input
                id="edit-replicas"
                type="number"
                class="input"
                min="1"
                max="100"
                [(ngModel)]="form().replicas"
                name="replicas"
              />
            </div>
          </div>

          @if (error()) {
            <p class="text-sm text-rose-600 dark:text-rose-400" role="alert">{{ error() }}</p>
          }

          <div class="flex justify-end gap-2">
            <button type="button" class="btn-secondary" (click)="navigateBack()">Cancel</button>
            <button type="submit" class="btn-primary">Save changes</button>
          </div>
        </form>
      } @else {
        <div class="card">
          <div class="card-body text-center">
            <p class="text-gray-600 dark:text-gray-300">Resource not found</p>
            <button type="button" class="btn-primary mt-4" (click)="navigateBack()">Go back</button>
          </div>
        </div>
      }
    </div>
  `,
})
export default class SampleItemEditComponent {
  /** Passed by the dispatcher. */
  canWrite = input<boolean>(false);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private store = inject(SampleItemStoreService);

  private params = toSignal(this.route.paramMap, { initialValue: this.route.snapshot.paramMap });

  private resourceId = computed(() => this.params().get('resourceId') ?? '');

  item = computed(() => this.store.items().find((i) => i.name === this.resourceId()));

  form = signal({ image: '', replicas: 1 });

  error = signal('');

  constructor() {
    const current = this.item();
    if (current) {
      this.form.set({ image: current.image, replicas: current.replicas });
    }
  }

  onSubmit(): void {
    const f = this.form();
    if (!f.image.trim()) {
      this.error.set('Image is required.');
      return;
    }
    this.store.update(this.resourceId(), { image: f.image.trim(), replicas: f.replicas });
    void this.router.navigate(['..'], { relativeTo: this.route });
  }

  navigateBack(): void {
    void this.router.navigate(['..'], { relativeTo: this.route });
  }
}
