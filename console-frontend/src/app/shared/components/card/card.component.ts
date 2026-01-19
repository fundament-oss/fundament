import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-card',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div
      class="rounded-md border bg-white dark:bg-gray-950"
      [class.border-gray-200]="!outlined"
      [class.dark:border-gray-800]="!outlined"
      [class.border-2]="outlined"
      [class.shadow-lg]="elevated"
    >
      @if (hasHeader) {
        <div class="border-b border-gray-200 px-6 py-4 dark:border-gray-800">
          <div class="flex items-center justify-between">
            <div>
              <ng-content select="[slot=header]"></ng-content>
            </div>
            @if (hasHeaderActions) {
              <div class="flex items-center gap-2">
                <ng-content select="[slot=header-actions]"></ng-content>
              </div>
            }
          </div>
        </div>
      }
      <div [class.p-6]="padding">
        @if (loading) {
          <div class="flex items-center justify-center py-12">
            <div
              class="h-8 w-8 animate-spin rounded-full border-b-2 border-gray-900 dark:border-white"
            ></div>
          </div>
        } @else {
          <ng-content></ng-content>
        }
      </div>
      @if (hasFooter) {
        <div class="border-t border-gray-200 px-6 py-4 dark:border-gray-800">
          <ng-content select="[slot=footer]"></ng-content>
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
export class CardComponent {
  @Input() hasHeader = false;
  @Input() hasHeaderActions = false;
  @Input() hasFooter = false;
  @Input() padding = true;
  @Input() outlined = false;
  @Input() elevated = false;
  @Input() loading = false;
}
