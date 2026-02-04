import { Component, Input, Output, EventEmitter, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX } from '@ng-icons/tabler-icons';

@Component({
  selector: 'app-modal',
  imports: [CommonModule, NgIconComponent],
  viewProviders: [
    provideIcons({
      tablerX,
    }),
  ],
  host: {
    '(document:keydown)': 'handleEscapeKey($event)',
  },
  templateUrl: './modal.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ModalComponent {
  @Input() show = false;
  @Input() title = '';
  @Input() maxWidth = 'max-w-md';
  @Output() modalClose = new EventEmitter<void>();

  handleEscapeKey(event: KeyboardEvent): void {
    if (this.show && event.key === 'Escape') {
      event.preventDefault();
      this.onClose();
    }
  }

  onClose(): void {
    this.modalClose.emit();
  }

  onBackdropClick(event: Event): void {
    // Close modal when clicking on the backdrop
    if (event.target === event.currentTarget) {
      this.onClose();
    }
  }
}
