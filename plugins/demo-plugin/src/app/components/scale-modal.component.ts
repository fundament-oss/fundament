import { Component, ChangeDetectionStrategy, input, signal, computed } from '@angular/core';
import { FormsModule } from '@angular/forms';

/**
 * Demo: modal action component for scaling a SampleItem.
 *
 * Demonstrates the `actions` extension point on a custom detail component.
 * The modal receives a `context` input from PluginModalService containing data
 * provided by the opener (e.g. the current replica count).
 *
 * To close this modal from code, inject PluginModalService and call:
 *   this.modalService.notifyClose();
 *
 * PluginModalService is provided in the host's root injector. Because @angular/core
 * is shared as a singleton in the NF configuration, remote components can inject
 * host services transparently without a separate SDK dependency.
 *
 * Import path (type reference — resolves from the host at runtime):
 *   import PluginModalService from 'fundament-plugin-sdk'; // Phase 2 SDK
 */

interface ScaleContext {
  currentReplicas?: number;
  resourceName?: string;
  namespace?: string;
}

@Component({
  selector: 'app-demo-scale-modal',
  imports: [FormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-4 p-4">
      <h3 class="text-base font-semibold dark:text-white">
        Scale {{ ctx().resourceName ?? 'app' }}
      </h3>

      <p class="text-sm text-gray-500 dark:text-gray-400">
        Current replicas:
        <strong class="text-gray-800 dark:text-gray-100">{{ ctx().currentReplicas ?? '—' }}</strong>
      </p>

      <div class="space-y-1">
        <label
          for="replica-count"
          class="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          New replica count
          <span class="text-rose-500" aria-hidden="true">*</span>
        </label>
        <input
          id="replica-count"
          type="number"
          min="0"
          max="50"
          class="w-full"
          [ngModel]="newReplicas()"
          (ngModelChange)="newReplicas.set($event)"
          aria-describedby="replica-hint"
        />
        <p id="replica-hint" class="text-xs text-gray-400 dark:text-gray-500">
          Set to 0 to suspend all instances.
        </p>
      </div>

      @if (saving()) {
        <p class="text-sm text-gray-500 dark:text-gray-400">Applying…</p>
      }

      <div class="flex justify-end gap-2 pt-2">
        <button type="button" class="btn-secondary" (click)="cancel()" [disabled]="saving()">
          Cancel
        </button>
        <button
          type="button"
          class="btn-primary"
          (click)="apply()"
          [disabled]="saving() || !isValid()"
        >
          Apply
        </button>
      </div>
    </div>
  `,
})
export default class ScaleModalComponent {
  /**
   * Context passed by the opener via PluginModalService.open('demo-ScaleModal', context).
   * The type is `unknown` to match the host's ComponentRef.setInput() contract.
   */
  context = input<unknown>();

  saving = signal(false);

  newReplicas = signal<number>(1);

  ctx = computed<ScaleContext>(() => {
    const raw = this.context();
    if (raw && typeof raw === 'object') return raw as ScaleContext;
    return {};
  });

  isValid = computed(() => {
    const v = this.newReplicas();
    return Number.isInteger(v) && v >= 0 && v <= 50;
  });

  apply(): void {
    if (!this.isValid()) return;
    this.saving.set(true);

    // In a real plugin, PATCH the resource here via PluginResourceStoreService before closing.
    // Simulate async patch for demo purposes
    setTimeout(() => {
      this.saving.set(false);
      window.dispatchEvent(new CustomEvent('plugin:close-modal'));
    }, 800);
  }

  cancel(): void {
    window.dispatchEvent(new CustomEvent('plugin:close-modal'));
  }
}
