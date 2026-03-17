import { Component, ChangeDetectionStrategy, input, signal, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, ActivatedRoute } from '@angular/router';
import { SampleItemStoreService } from '../sample-item-store.service';
import type { SampleItem } from '../sample-item-store.service';

/**
 * Demo: custom create wizard for SampleItem resources.
 *
 * Demonstrates the `createWizard` extension point. Adds the new item to the
 * shared in-memory SampleItemStoreService so the list reflects it immediately.
 *
 * In a real plugin, inject PluginResourceStoreService (from the Fundament SDK)
 * to create the resource in the cluster.
 */

@Component({
  selector: 'app-demo-sample-item-create',
  imports: [FormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-6 p-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold dark:text-white">New sample item</h2>
        <button type="button" class="btn-secondary" (click)="navigateBack()">Cancel</button>
      </div>

      <!-- Demo notice -->
      <div
        class="rounded-md border border-blue-200 bg-blue-50 px-4 py-2 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
        role="note"
      >
        Demo plugin create wizard. Items are stored in memory and reset on page refresh.
      </div>

      <form class="card space-y-4 p-4" (ngSubmit)="onSubmit()">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <!-- Name -->
          <div class="flex flex-col gap-1">
            <label for="item-name" class="text-sm font-medium dark:text-white">Name</label>
            <input
              id="item-name"
              type="text"
              class="input"
              required
              placeholder="my-item"
              [(ngModel)]="form().name"
              name="name"
            />
          </div>

          <!-- Namespace -->
          <div class="flex flex-col gap-1">
            <label for="item-namespace" class="text-sm font-medium dark:text-white">Namespace</label>
            <input
              id="item-namespace"
              type="text"
              class="input"
              required
              placeholder="default"
              [(ngModel)]="form().namespace"
              name="namespace"
            />
          </div>

          <!-- Image -->
          <div class="flex flex-col gap-1">
            <label for="item-image" class="text-sm font-medium dark:text-white">Image</label>
            <input
              id="item-image"
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
            <label for="item-replicas" class="text-sm font-medium dark:text-white">Replicas</label>
            <input
              id="item-replicas"
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
          <button type="submit" class="btn-primary">Create item</button>
        </div>
      </form>
    </div>
  `,
})
export default class SampleItemCreateComponent {
  /** Passed by the dispatcher. */
  canWrite = input<boolean>(false);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private store = inject(SampleItemStoreService);

  form = signal<{ name: string; namespace: string; image: string; replicas: number }>({
    name: '',
    namespace: 'default',
    image: '',
    replicas: 1,
  });

  error = signal('');

  onSubmit(): void {
    const f = this.form();

    if (!f.name.trim()) {
      this.error.set('Name is required.');
      return;
    }
    if (!f.namespace.trim()) {
      this.error.set('Namespace is required.');
      return;
    }
    if (!f.image.trim()) {
      this.error.set('Image is required.');
      return;
    }

    const item: SampleItem = {
      name: f.name.trim(),
      namespace: f.namespace.trim(),
      image: f.image.trim(),
      replicas: f.replicas,
      status: 'Pending',
    };

    this.store.add(item);
    void this.router.navigate(['..'], { relativeTo: this.route });
  }

  navigateBack(): void {
    void this.router.navigate(['..'], { relativeTo: this.route });
  }
}
