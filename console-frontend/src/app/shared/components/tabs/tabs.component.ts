import { Component, Input, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule } from '@angular/router';

export interface Tab {
  id: string;
  label: string;
  icon?: string;
  badge?: string | number;
  disabled?: boolean;
  routerLink?: string | unknown[];
}

@Component({
  selector: 'app-tabs',
  standalone: true,
  imports: [CommonModule, RouterModule],
  template: `
    <div>
      <div class="border-b border-gray-200 dark:border-gray-800">
        <nav class="-mb-px flex space-x-8" [class.flex-col]="orientation === 'vertical'">
          @for (tab of tabs; track tab.id) {
            @if (tab.routerLink) {
              <a
                [routerLink]="tab.routerLink"
                routerLinkActive="border-indigo-500 text-indigo-600 dark:border-indigo-400 dark:text-indigo-400"
                [routerLinkActiveOptions]="{ exact: false }"
                class="group inline-flex items-center border-b-2 px-1 py-4 text-sm font-medium transition-colors"
                [class.border-transparent]="activeTab !== tab.id"
                [class.text-gray-500]="activeTab !== tab.id && !tab.disabled"
                [class.hover:border-gray-300]="activeTab !== tab.id && !tab.disabled"
                [class.hover:text-gray-700]="activeTab !== tab.id && !tab.disabled"
                [class.dark:text-gray-400]="activeTab !== tab.id && !tab.disabled"
                [class.dark:hover:text-gray-300]="activeTab !== tab.id && !tab.disabled"
                [class.cursor-not-allowed]="tab.disabled"
                [class.text-gray-400]="tab.disabled"
                [class.dark:text-gray-600]="tab.disabled"
                [attr.aria-current]="activeTab === tab.id ? 'page' : null"
                [attr.aria-disabled]="tab.disabled"
              >
                @if (tab.icon) {
                  <span class="mr-2">{{ tab.icon }}</span>
                }
                {{ tab.label }}
                @if (tab.badge !== undefined && tab.badge !== null) {
                  <span
                    class="ml-2 rounded-full bg-gray-200 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-gray-700 dark:text-gray-200"
                  >
                    {{ tab.badge }}
                  </span>
                }
              </a>
            } @else {
              <button
                type="button"
                (click)="selectTab(tab)"
                [disabled]="tab.disabled"
                class="group inline-flex items-center border-b-2 px-1 py-4 text-sm font-medium transition-colors"
                [class.border-indigo-500]="activeTab === tab.id"
                [class.text-indigo-600]="activeTab === tab.id"
                [class.dark:border-indigo-400]="activeTab === tab.id"
                [class.dark:text-indigo-400]="activeTab === tab.id"
                [class.border-transparent]="activeTab !== tab.id"
                [class.text-gray-500]="activeTab !== tab.id && !tab.disabled"
                [class.hover:border-gray-300]="activeTab !== tab.id && !tab.disabled"
                [class.hover:text-gray-700]="activeTab !== tab.id && !tab.disabled"
                [class.dark:text-gray-400]="activeTab !== tab.id && !tab.disabled"
                [class.dark:hover:text-gray-300]="activeTab !== tab.id && !tab.disabled"
                [class.cursor-not-allowed]="tab.disabled"
                [class.text-gray-400]="tab.disabled"
                [class.dark:text-gray-600]="tab.disabled"
                [attr.aria-current]="activeTab === tab.id ? 'page' : null"
              >
                @if (tab.icon) {
                  <span class="mr-2">{{ tab.icon }}</span>
                }
                {{ tab.label }}
                @if (tab.badge !== undefined && tab.badge !== null) {
                  <span
                    class="ml-2 rounded-full bg-gray-200 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-gray-700 dark:text-gray-200"
                  >
                    {{ tab.badge }}
                  </span>
                }
              </button>
            }
          }
        </nav>
      </div>
      @if (!useRouter) {
        <div class="mt-4">
          <ng-content></ng-content>
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
export class TabsComponent {
  @Input() tabs: Tab[] = [];
  @Input() activeTab = '';
  @Input() orientation: 'horizontal' | 'vertical' = 'horizontal';
  @Input() useRouter = false;

  @Output() tabChange = new EventEmitter<Tab>();

  selectTab(tab: Tab): void {
    if (tab.disabled) {
      return;
    }
    this.activeTab = tab.id;
    this.tabChange.emit(tab);
  }
}
