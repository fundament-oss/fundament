import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule } from '@angular/router';

export interface Breadcrumb {
  label: string;
  url?: string | unknown[];
  icon?: string;
}

@Component({
  selector: 'app-breadcrumbs',
  standalone: true,
  imports: [CommonModule, RouterModule],
  template: `
    <nav class="flex" aria-label="Breadcrumb">
      <ol class="flex items-center space-x-2">
        @for (crumb of breadcrumbs; track $index; let isLast = $last) {
          <li class="flex items-center">
            @if (!isLast && crumb.url) {
              <a
                [routerLink]="crumb.url"
                class="flex items-center text-sm font-medium text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              >
                @if (crumb.icon) {
                  <span class="mr-1.5">{{ crumb.icon }}</span>
                }
                {{ crumb.label }}
              </a>
            } @else {
              <span
                class="flex items-center text-sm font-medium"
                [class.text-gray-900]="isLast"
                [class.dark:text-white]="isLast"
                [class.text-gray-500]="!isLast"
                [class.dark:text-gray-400]="!isLast"
                [attr.aria-current]="isLast ? 'page' : null"
              >
                @if (crumb.icon) {
                  <span class="mr-1.5">{{ crumb.icon }}</span>
                }
                {{ crumb.label }}
              </span>
            }
            @if (!isLast) {
              <svg
                class="mx-2 h-5 w-5 shrink-0 text-gray-400 dark:text-gray-600"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            }
          </li>
        }
      </ol>
    </nav>
  `,
  styles: [
    `
      :host {
        display: block;
      }
    `,
  ],
})
export class BreadcrumbsComponent {
  @Input() breadcrumbs: Breadcrumb[] = [];
}
