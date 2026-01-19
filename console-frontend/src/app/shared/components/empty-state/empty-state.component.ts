import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-empty-state',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="text-center" [class.py-12]="padding">
      @if (hasIcon) {
        <div
          class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800"
        >
          <ng-content select="[slot=icon]"></ng-content>
        </div>
      }
      @if (title) {
        <h3 class="mb-2 text-lg font-medium text-gray-900 dark:text-white">{{ title }}</h3>
      }
      @if (description) {
        <p class="mb-6 text-sm text-gray-500 dark:text-gray-400">{{ description }}</p>
      }
      @if (hasPrimaryAction || hasSecondaryAction) {
        <div class="flex items-center justify-center gap-3">
          @if (hasPrimaryAction) {
            <ng-content select="[slot=primary-action]"></ng-content>
          }
          @if (hasSecondaryAction) {
            <ng-content select="[slot=secondary-action]"></ng-content>
          }
        </div>
      }
    </div>
  `,
  styles: [
    `
      :host {
        display: block;
      }
    `,
  ],
})
export class EmptyStateComponent {
  @Input() title = '';
  @Input() description = '';
  @Input() hasIcon = false;
  @Input() hasPrimaryAction = false;
  @Input() hasSecondaryAction = false;
  @Input() padding = true;
}
