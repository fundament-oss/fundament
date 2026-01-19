import { Component, Input, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule } from '@angular/router';

export type ButtonVariant = 'primary' | 'secondary' | 'light' | 'danger' | 'ghost';
export type ButtonSize = 'sm' | 'md' | 'lg';

@Component({
  selector: 'app-button',
  standalone: true,
  imports: [CommonModule, RouterModule],
  template: `
    @if (href && !routerLink) {
      <a
        [href]="href"
        [class]="getClasses()"
        [attr.disabled]="disabled || loading ? true : null"
        [attr.aria-disabled]="disabled || loading"
        (click)="handleClick($event)"
      >
        <ng-container *ngTemplateOutlet="buttonContent"></ng-container>
      </a>
    } @else if (routerLink) {
      <a
        [routerLink]="routerLink"
        [class]="getClasses()"
        [attr.disabled]="disabled || loading ? true : null"
        [attr.aria-disabled]="disabled || loading"
        (click)="handleClick($event)"
      >
        <ng-container *ngTemplateOutlet="buttonContent"></ng-container>
      </a>
    } @else {
      <button
        [type]="type"
        [class]="getClasses()"
        [disabled]="disabled || loading"
        (click)="handleClick($event)"
      >
        <ng-container *ngTemplateOutlet="buttonContent"></ng-container>
      </button>
    }

    <ng-template #buttonContent>
      @if (loading) {
        <div class="mr-2 h-4 w-4 animate-spin rounded-full border-b-2 border-current"></div>
      }
      @if (iconLeading) {
        <span class="icon-leading" [class.mr-2]="hasContent()">
          <ng-content select="[slot=icon-leading]"></ng-content>
        </span>
      }
      <ng-content></ng-content>
      @if (iconTrailing) {
        <span class="icon-trailing" [class.ml-2]="hasContent()">
          <ng-content select="[slot=icon-trailing]"></ng-content>
        </span>
      }
    </ng-template>
  `,
  styles: [
    `
      :host {
        display: inline-block;
      }

      :host([block]) {
        display: block;
      }

      :host([block]) button,
      :host([block]) a {
        width: 100%;
        justify-content: center;
      }

      button,
      a {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        font-weight: 500;
        transition-property: color, background-color, border-color, transform;
        transition-duration: 200ms;
        transition-timing-function: ease-in-out;
      }

      button:disabled,
      a[disabled] {
        cursor: not-allowed;
        opacity: 0.6;
      }

      a[disabled] {
        pointer-events: none;
      }
    `,
  ],
})
export class ButtonComponent {
  @Input() variant: ButtonVariant = 'primary';
  @Input() size: ButtonSize = 'md';
  @Input() type: 'button' | 'submit' | 'reset' = 'button';
  @Input() disabled = false;
  @Input() loading = false;
  @Input() iconLeading = false;
  @Input() iconTrailing = false;
  @Input() href?: string;
  @Input() routerLink?: string | unknown[];
  @Input() block = false;

  @Output() clicked = new EventEmitter<Event>();

  handleClick(event: Event): void {
    if (this.disabled || this.loading) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    this.clicked.emit(event);
  }

  hasContent(): boolean {
    return true;
  }

  getClasses(): string {
    const baseClasses =
      'cursor-pointer rounded-md border focus:ring-2 focus:ring-offset-2 focus:outline-none dark:ring-offset-gray-950';

    const sizeClasses = {
      sm: 'px-3 py-1.5 text-xs',
      md: 'px-4 py-2 text-sm',
      lg: 'px-6 py-3 text-base',
    };

    const variantClasses = {
      primary:
        'border-transparent bg-indigo-600 text-white shadow-sm ring-indigo-500 hover:bg-indigo-700',
      secondary:
        'border-gray-300 bg-white text-gray-700 shadow-sm ring-indigo-500 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-900 dark:text-white dark:hover:bg-gray-700',
      light:
        'border-indigo-300 bg-white text-indigo-700 shadow-sm ring-indigo-500 hover:bg-indigo-50 dark:border-indigo-600 dark:bg-gray-900 dark:text-indigo-300 dark:hover:bg-indigo-900 dark:hover:text-white',
      danger:
        'border-rose-300 bg-white text-rose-700 shadow-sm ring-rose-500 hover:bg-rose-50 dark:border-rose-600 dark:bg-gray-900 dark:text-rose-300 dark:hover:bg-rose-900 dark:hover:text-white',
      ghost:
        'border-transparent bg-transparent text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800',
    };

    return `${baseClasses} ${sizeClasses[this.size]} ${variantClasses[this.variant]}`;
  }
}
