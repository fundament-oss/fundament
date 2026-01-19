import { Component, Input, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';

export type AlertVariant = 'success' | 'warning' | 'danger' | 'info';

@Component({
  selector: 'app-alert',
  standalone: true,
  imports: [CommonModule],
  template: `
    @if (isVisible) {
      <div class="flex items-start rounded-md p-4" [class]="getClasses()" role="alert">
        @if (hasIcon) {
          <div class="shrink-0" [class]="getIconColorClass()">
            <ng-content select="[slot=icon]"></ng-content>
          </div>
        }
        <div class="flex-1" [class.ml-3]="hasIcon">
          @if (title) {
            <h3 class="mb-1 text-sm font-medium" [class]="getTitleColorClass()">{{ title }}</h3>
          }
          <div class="text-sm" [class]="getTextColorClass()">
            <ng-content></ng-content>
          </div>
          @if (hasActions) {
            <div class="mt-3">
              <ng-content select="[slot=actions]"></ng-content>
            </div>
          }
        </div>
        @if (dismissible) {
          <button
            type="button"
            (click)="dismiss()"
            class="ml-3 inline-flex shrink-0 rounded-md p-1.5 focus:ring-2 focus:ring-offset-2 focus:outline-none"
            [class]="getDismissButtonClass()"
            aria-label="Dismiss"
          >
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        }
      </div>
    }
  `,
  styles: [
    `
      :host {
        display: block;
      }
    `,
  ],
})
export class AlertComponent {
  @Input() variant: AlertVariant = 'info';
  @Input() title = '';
  @Input() dismissible = false;
  @Input() hasIcon = false;
  @Input() hasActions = false;

  @Output() dismissed = new EventEmitter<void>();

  isVisible = true;

  dismiss(): void {
    this.isVisible = false;
    this.dismissed.emit();
  }

  getClasses(): string {
    const variantClasses = {
      success: 'bg-green-50 dark:bg-green-900/20',
      warning: 'bg-yellow-50 dark:bg-yellow-900/20',
      danger: 'bg-rose-50 dark:bg-rose-900/20',
      info: 'bg-blue-50 dark:bg-blue-900/20',
    };
    return variantClasses[this.variant];
  }

  getIconColorClass(): string {
    const iconColorClasses = {
      success: 'text-green-400 dark:text-green-500',
      warning: 'text-yellow-400 dark:text-yellow-500',
      danger: 'text-rose-400 dark:text-rose-500',
      info: 'text-blue-400 dark:text-blue-500',
    };
    return iconColorClasses[this.variant];
  }

  getTitleColorClass(): string {
    const titleColorClasses = {
      success: 'text-green-800 dark:text-green-200',
      warning: 'text-yellow-800 dark:text-yellow-200',
      danger: 'text-rose-800 dark:text-rose-200',
      info: 'text-blue-800 dark:text-blue-200',
    };
    return titleColorClasses[this.variant];
  }

  getTextColorClass(): string {
    const textColorClasses = {
      success: 'text-green-700 dark:text-green-300',
      warning: 'text-yellow-700 dark:text-yellow-300',
      danger: 'text-rose-700 dark:text-rose-300',
      info: 'text-blue-700 dark:text-blue-300',
    };
    return textColorClasses[this.variant];
  }

  getDismissButtonClass(): string {
    const dismissButtonClasses = {
      success:
        'text-green-500 hover:bg-green-100 focus:ring-green-600 focus:ring-offset-green-50 dark:hover:bg-green-800',
      warning:
        'text-yellow-500 hover:bg-yellow-100 focus:ring-yellow-600 focus:ring-offset-yellow-50 dark:hover:bg-yellow-800',
      danger:
        'text-rose-500 hover:bg-rose-100 focus:ring-rose-600 focus:ring-offset-rose-50 dark:hover:bg-rose-800',
      info: 'text-blue-500 hover:bg-blue-100 focus:ring-blue-600 focus:ring-offset-blue-50 dark:hover:bg-blue-800',
    };
    return dismissButtonClasses[this.variant];
  }
}
