import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

export type SkeletonVariant = 'text' | 'circular' | 'rectangular';

@Component({
  selector: 'app-skeleton',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div
      class="animate-pulse bg-gray-200 dark:bg-gray-700"
      [class.rounded-full]="variant === 'circular'"
      [class.rounded]="variant === 'rectangular'"
      [class.rounded-md]="variant === 'text'"
      [style.width]="width"
      [style.height]="height"
      role="status"
      aria-label="Loading"
    >
      <span class="sr-only">Loading...</span>
    </div>
  `,
  styles: [
    `
      :host {
        display: block;
      }

      .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border-width: 0;
      }
    `,
  ],
})
export class SkeletonComponent {
  @Input() variant: SkeletonVariant = 'text';
  @Input() width = '100%';
  @Input() height = '1rem';
}
