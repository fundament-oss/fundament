import { Component, Input, Output, EventEmitter, HostListener } from '@angular/core';
import { CommonModule } from '@angular/common';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX } from '@ng-icons/tabler-icons';

@Component({
  selector: 'app-modal',
  standalone: true,
  imports: [CommonModule, NgIconComponent],
  viewProviders: [
    provideIcons({
      tablerX,
    }),
  ],
  templateUrl: './modal.component.html',
})
export class ModalComponent {
  @Input() show = false;
  @Input() title = '';
  @Input() maxWidth = 'max-w-md';
  @Output() close = new EventEmitter<void>();

  @HostListener('document:keydown', ['$event'])
  handleEscapeKey(event: KeyboardEvent): void {
    if (this.show && event.key === 'Escape') {
      event.preventDefault();
      this.onClose();
    }
  }

  onClose(): void {
    this.close.emit();
  }

  onBackdropClick(event: Event): void {
    // Close modal when clicking on the backdrop
    if (event.target === event.currentTarget) {
      this.onClose();
    }
  }
}
