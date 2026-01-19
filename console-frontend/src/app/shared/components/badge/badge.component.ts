import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

export type BadgeVariant =
  | 'default'
  | 'success'
  | 'warning'
  | 'danger'
  | 'info'
  | 'purple'
  | 'blue'
  | 'green';
export type BadgeSize = 'sm' | 'md' | 'lg';

@Component({
  selector: 'app-badge',
  standalone: true,
  imports: [CommonModule],
  template: `
    <span
      class="inline-flex items-center rounded-full font-medium"
      [class]="getClasses()"
      [attr.aria-label]="ariaLabel"
    >
      @if (dot) {
        <span class="mr-1.5 h-2 w-2 rounded-full" [class]="getDotColor()"></span>
      }
      <ng-content></ng-content>
    </span>
  `,
  styles: [
    `
      :host {
        display: inline-block;
      }
    `,
  ],
})
export class BadgeComponent {
  @Input() variant: BadgeVariant = 'default';
  @Input() size: BadgeSize = 'md';
  @Input() dot = false;
  @Input() ariaLabel = '';

  getClasses(): string {
    const sizeClasses = {
      sm: 'px-2 py-0.5 text-xs',
      md: 'px-2.5 py-0.5 text-xs',
      lg: 'px-3 py-1 text-sm',
    };

    const variantClasses = {
      default: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
      success: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
      warning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
      danger: 'bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200',
      info: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-950 dark:text-cyan-200',
      purple: 'bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-200',
      blue: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
      green: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200',
    };

    return `${sizeClasses[this.size]} ${variantClasses[this.variant]}`;
  }

  getDotColor(): string {
    const dotColors = {
      default: 'bg-gray-400 dark:bg-gray-500',
      success: 'bg-green-500',
      warning: 'bg-yellow-500',
      danger: 'bg-rose-500',
      info: 'bg-cyan-500',
      purple: 'bg-purple-500',
      blue: 'bg-blue-500',
      green: 'bg-emerald-500',
    };
    return dotColors[this.variant];
  }
}
