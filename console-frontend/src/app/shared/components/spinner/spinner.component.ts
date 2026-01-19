import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

export type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl';
export type SpinnerVariant = 'border' | 'dots';

@Component({
  selector: 'app-spinner',
  standalone: true,
  imports: [CommonModule],
  template: `
    @if (variant === 'border') {
      <div
        class="animate-spin rounded-full border-b-2"
        [class]="getSizeClass()"
        [class.border-gray-900]="!color"
        [class.dark:border-white]="!color"
        [style.border-color]="color"
        role="status"
        [attr.aria-label]="ariaLabel"
      >
        <span class="sr-only">Loading...</span>
      </div>
    } @else {
      <div
        class="flex items-center justify-center space-x-2"
        role="status"
        [attr.aria-label]="ariaLabel"
      >
        <div
          class="animate-bounce rounded-full"
          [class]="getDotSizeClass()"
          [class.bg-gray-900]="!color"
          [class.dark:bg-white]="!color"
          [style.background-color]="color"
          [style.animation-delay]="'0ms'"
        ></div>
        <div
          class="animate-bounce rounded-full"
          [class]="getDotSizeClass()"
          [class.bg-gray-900]="!color"
          [class.dark:bg-white]="!color"
          [style.background-color]="color"
          [style.animation-delay]="'150ms'"
        ></div>
        <div
          class="animate-bounce rounded-full"
          [class]="getDotSizeClass()"
          [class.bg-gray-900]="!color"
          [class.dark:bg-white]="!color"
          [style.background-color]="color"
          [style.animation-delay]="'300ms'"
        ></div>
        <span class="sr-only">Loading...</span>
      </div>
    }
  `,
  styles: [
    `
      :host {
        display: inline-block;
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
export class SpinnerComponent {
  @Input() size: SpinnerSize = 'md';
  @Input() variant: SpinnerVariant = 'border';
  @Input() color = '';
  @Input() ariaLabel = 'Loading';

  getSizeClass(): string {
    const sizeClasses = {
      sm: 'h-4 w-4',
      md: 'h-8 w-8',
      lg: 'h-12 w-12',
      xl: 'h-16 w-16',
    };
    return sizeClasses[this.size];
  }

  getDotSizeClass(): string {
    const dotSizeClasses = {
      sm: 'h-1.5 w-1.5',
      md: 'h-2.5 w-2.5',
      lg: 'h-3.5 w-3.5',
      xl: 'h-5 w-5',
    };
    return dotSizeClasses[this.size];
  }
}
