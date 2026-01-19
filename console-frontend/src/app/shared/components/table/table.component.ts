import { Component, Input, Output, EventEmitter, TemplateRef } from '@angular/core';
import { CommonModule } from '@angular/common';

export interface TableColumn<T = unknown> {
  key: string;
  label: string;
  sortable?: boolean;
  width?: string;
  align?: 'left' | 'center' | 'right';
  template?: TemplateRef<{ $implicit: T }>;
}

export interface TableSortEvent {
  column: string;
  direction: 'asc' | 'desc' | null;
}

@Component({
  selector: 'app-table',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-800">
        @if (showHeader) {
          <thead class="bg-gray-50 dark:bg-gray-900">
            <tr>
              @for (column of columns; track column.key) {
                <th
                  scope="col"
                  class="px-6 py-3 text-xs font-medium tracking-wider text-gray-500 uppercase dark:text-gray-400"
                  [class.text-left]="column.align === 'left' || !column.align"
                  [class.text-center]="column.align === 'center'"
                  [class.text-right]="column.align === 'right'"
                  [class.cursor-pointer]="column.sortable"
                  [class.hover:bg-gray-100]="column.sortable"
                  [class.dark:hover:bg-gray-800]="column.sortable"
                  [style.width]="column.width"
                  (click)="column.sortable && handleSort(column.key)"
                >
                  <div
                    class="flex items-center gap-1"
                    [class.justify-end]="column.align === 'right'"
                  >
                    <span>{{ column.label }}</span>
                    @if (column.sortable) {
                      <svg
                        class="h-4 w-4"
                        [class.text-indigo-600]="sortColumn === column.key"
                        [class.dark:text-indigo-400]="sortColumn === column.key"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        @if (sortColumn === column.key && sortDirection === 'asc') {
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M5 15l7-7 7 7"
                          />
                        } @else if (sortColumn === column.key && sortDirection === 'desc') {
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M19 9l-7 7-7-7"
                          />
                        } @else {
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"
                          />
                        }
                      </svg>
                    }
                  </div>
                </th>
              }
              @if (hasActions) {
                <th
                  scope="col"
                  class="relative px-6 py-3 text-right text-xs font-medium tracking-wider text-gray-500 uppercase dark:text-gray-400"
                >
                  Actions
                </th>
              }
            </tr>
          </thead>
        }
        <tbody class="divide-y divide-gray-200 bg-white dark:divide-gray-800 dark:bg-gray-950">
          @if (loading) {
            <tr>
              <td [attr.colspan]="columns.length + (hasActions ? 1 : 0)" class="px-6 py-12">
                <div class="flex items-center justify-center">
                  <div
                    class="h-8 w-8 animate-spin rounded-full border-b-2 border-gray-900 dark:border-white"
                  ></div>
                </div>
              </td>
            </tr>
          } @else if (data.length === 0) {
            <tr>
              <td
                [attr.colspan]="columns.length + (hasActions ? 1 : 0)"
                class="px-6 py-12 text-center"
              >
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ emptyMessage }}
                </p>
              </td>
            </tr>
          } @else {
            @for (row of data; track trackBy ? trackBy(row) : $index) {
              <tr
                class="transition-colors"
                [class.hover:bg-gray-50]="hoverable"
                [class.dark:hover:bg-gray-900]="hoverable"
                [class.cursor-pointer]="clickable"
                (click)="handleRowClick(row)"
              >
                @for (column of columns; track column.key) {
                  <td
                    class="px-6 py-4 text-sm whitespace-nowrap dark:text-white"
                    [class.text-left]="column.align === 'left' || !column.align"
                    [class.text-center]="column.align === 'center'"
                    [class.text-right]="column.align === 'right'"
                  >
                    @if (column.template) {
                      <ng-container
                        *ngTemplateOutlet="column.template; context: { $implicit: row }"
                      ></ng-container>
                    } @else {
                      {{ getCellValue(row, column.key) }}
                    }
                  </td>
                }
                @if (hasActions) {
                  <td
                    class="px-6 py-4 text-right text-sm font-medium whitespace-nowrap"
                    (click)="$event.stopPropagation()"
                  >
                    <ng-content select="[slot=actions]"></ng-content>
                  </td>
                }
              </tr>
            }
          }
        </tbody>
      </table>
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
export class TableComponent<T = unknown> {
  @Input() columns: TableColumn<T>[] = [];
  @Input() data: T[] = [];
  @Input() loading = false;
  @Input() hoverable = true;
  @Input() clickable = false;
  @Input() showHeader = true;
  @Input() hasActions = false;
  @Input() emptyMessage = 'No data available';
  @Input() sortColumn: string | null = null;
  @Input() sortDirection: 'asc' | 'desc' | null = null;
  @Input() trackBy?: (item: T) => string | number;

  @Output() rowClick = new EventEmitter<T>();
  @Output() sort = new EventEmitter<TableSortEvent>();

  handleRowClick(row: T): void {
    if (this.clickable) {
      this.rowClick.emit(row);
    }
  }

  handleSort(columnKey: string): void {
    let direction: 'asc' | 'desc' | null = 'asc';

    if (this.sortColumn === columnKey) {
      if (this.sortDirection === 'asc') {
        direction = 'desc';
      } else if (this.sortDirection === 'desc') {
        direction = null;
      }
    }

    this.sortColumn = direction ? columnKey : null;
    this.sortDirection = direction;

    this.sort.emit({
      column: columnKey,
      direction: direction,
    });
  }

  getCellValue(row: T, key: string): unknown {
    return (row as Record<string, unknown>)[key];
  }
}
