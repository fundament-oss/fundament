import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

export type ProgressVariant = 'default' | 'success' | 'warning' | 'danger';
export type ProgressSize = 'sm' | 'md' | 'lg';

@Component({
  selector: 'app-progress-bar',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div>
      @if (showLabel) {
        <div class="mb-2 flex items-center justify-between text-sm">
          @if (label) {
            <span class="font-medium text-gray-700 dark:text-white">{{ label }}</span>
          }
          @if (showPercentage) {
            <span class="text-gray-500 dark:text-gray-400">{{ value }}%</span>
          }
        </div>
      }
      <div class="rounded-full bg-gray-200 dark:bg-gray-700" [class]="getHeightClass()">
        <div
          class="rounded-full transition-all duration-300 ease-in-out"
          [class]="getColorClass()"
          [class]="getHeightClass()"
          [style.width.%]="clampValue()"
          role="progressbar"
          [attr.aria-valuenow]="value"
          [attr.aria-valuemin]="0"
          [attr.aria-valuemax]="max"
        ></div>
      </div>
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
export class ProgressBarComponent {
  @Input() value = 0;
  @Input() max = 100;
  @Input() variant: ProgressVariant = 'default';
  @Input() size: ProgressSize = 'md';
  @Input() label = '';
  @Input() showLabel = false;
  @Input() showPercentage = true;

  clampValue(): number {
    const percentage = (this.value / this.max) * 100;
    return Math.min(Math.max(percentage, 0), 100);
  }

  getHeightClass(): string {
    const heightClasses = {
      sm: 'h-1',
      md: 'h-2',
      lg: 'h-3',
    };
    return heightClasses[this.size];
  }

  getColorClass(): string {
    const colorClasses = {
      default: 'bg-indigo-600 dark:bg-indigo-500',
      success: 'bg-green-600 dark:bg-green-500',
      warning: 'bg-yellow-500 dark:bg-yellow-400',
      danger: 'bg-rose-600 dark:bg-rose-500',
    };
    return colorClasses[this.variant];
  }
}
