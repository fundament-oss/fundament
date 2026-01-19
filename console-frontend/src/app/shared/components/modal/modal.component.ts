import { Component, Input, Output, EventEmitter, HostListener } from '@angular/core';
import { CommonModule } from '@angular/common';

export type ModalSize = 'sm' | 'md' | 'lg' | 'xl' | 'full';

@Component({
  selector: 'app-modal',
  standalone: true,
  imports: [CommonModule],
  template: `
    @if (isOpen) {
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
        (click)="handleBackdropClick()"
        (keydown)="handleBackdropKeydown($event)"
        tabindex="0"
      >
        <div
          class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-xl dark:border-gray-800 dark:bg-gray-950"
          [class]="getModalSizeClass()"
          role="dialog"
          [attr.aria-labelledby]="titleId"
          [attr.aria-modal]="true"
          (click)="$event.stopPropagation()"
          (keydown)="$event.stopPropagation()"
        >
          @if (hasHeader) {
            <div class="border-b border-gray-200 px-6 py-4 dark:border-gray-800">
              <div class="flex items-center justify-between">
                <h2 [id]="titleId" class="text-xl font-semibold dark:text-white">
                  <ng-content select="[slot=header]"></ng-content>
                </h2>
                @if (showCloseButton) {
                  <button
                    type="button"
                    (click)="close()"
                    class="cursor-pointer text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                    aria-label="Close"
                  >
                    <svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
            </div>
          }
          <div
            class="px-6 py-4"
            [class.max-h-[70vh]]="scrollable"
            [class.overflow-y-auto]="scrollable"
          >
            <ng-content></ng-content>
          </div>
          @if (hasFooter) {
            <div class="border-t border-gray-200 px-6 py-4 dark:border-gray-800">
              <ng-content select="[slot=footer]"></ng-content>
            </div>
          }
        </div>
      </div>
    }
  `,
  styles: [
    `
      :host {
        display: contents;
      }
    `,
  ],
})
export class ModalComponent {
  @Input() isOpen = false;
  @Input() size: ModalSize = 'md';
  @Input() hasHeader = true;
  @Input() hasFooter = false;
  @Input() showCloseButton = true;
  @Input() closeOnBackdrop = true;
  @Input() closeOnEscape = true;
  @Input() scrollable = true;
  @Input() titleId = 'modal-title';

  @Output() closed = new EventEmitter<void>();

  @HostListener('document:keydown', ['$event'])
  handleEscape(event: KeyboardEvent): void {
    if (this.isOpen && this.closeOnEscape && event.key === 'Escape') {
      this.close();
    }
  }

  handleBackdropClick(): void {
    if (this.closeOnBackdrop) {
      this.close();
    }
  }

  handleBackdropKeydown(event: KeyboardEvent): void {
    if (this.closeOnBackdrop && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault();
      this.close();
    }
  }

  close(): void {
    this.isOpen = false;
    this.closed.emit();
  }

  getModalSizeClass(): string {
    const sizeClasses = {
      sm: 'w-full max-w-sm',
      md: 'w-full max-w-md',
      lg: 'w-full max-w-lg',
      xl: 'w-full max-w-xl',
      full: 'w-full max-w-7xl',
    };
    return sizeClasses[this.size];
  }
}
