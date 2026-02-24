import {
  Component,
  Input,
  Output,
  EventEmitter,
  ChangeDetectionStrategy,
  ElementRef,
  ViewChild,
  OnChanges,
  SimpleChanges,
  AfterViewChecked,
  OnDestroy,
} from '@angular/core';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX } from '@ng-icons/tabler-icons';

@Component({
  selector: 'app-modal',
  imports: [NgIconComponent],
  viewProviders: [
    provideIcons({
      tablerX,
    }),
  ],
  host: {
    '(document:keydown)': 'handleKeydown($event)',
    '[attr.title]': 'null',
  },
  templateUrl: './modal.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ModalComponent implements OnChanges, AfterViewChecked, OnDestroy {
  @Input() show = false;

  @Input() title = '';

  @Input() maxWidth = 'max-w-md';

  @Output() modalClose = new EventEmitter<void>();

  @ViewChild('modalDialog', { read: ElementRef }) modalDialog?: ElementRef<HTMLDivElement>;

  private previouslyFocusedElement: HTMLElement | null = null;

  private shouldSetFocus = false;

  ngOnChanges(changes: SimpleChanges): void {
    // Detect when modal opens and flag that we should set focus
    if (
      changes['show'] &&
      changes['show'].currentValue === true &&
      !changes['show'].previousValue
    ) {
      this.previouslyFocusedElement = document.activeElement as HTMLElement;
      this.shouldSetFocus = true;
    }
  }

  ngAfterViewChecked(): void {
    // Once the view is checked and we have the flag set, attempt to set focus
    if (this.shouldSetFocus && this.show && this.modalDialog) {
      this.shouldSetFocus = false;
      // Use a retry mechanism to handle various rendering timing scenarios
      this.trySetFocus(0);
    }
  }

  ngOnDestroy(): void {
    // Ensure focus is restored if component is destroyed while modal is open
    if (this.show && this.previouslyFocusedElement) {
      this.previouslyFocusedElement.focus();
    }
  }

  private trySetFocus(attempt = 0): void {
    if (attempt > 3 || !this.show) return;

    setTimeout(() => {
      if (this.setInitialFocus()) return;
      this.trySetFocus(attempt + 1);
    }, attempt * 100);
  }

  private setInitialFocus(): boolean {
    // Get focusable elements excluding the close button for initial focus
    const elements = this.getFocusableElements().filter(
      (el) => !el.hasAttribute('aria-label') || el.getAttribute('aria-label') !== 'Close',
    );
    const element = elements[0] || this.modalDialog?.nativeElement;
    if (!element) {
      return false;
    }

    element.focus();
    return document.activeElement === element;
  }

  private getFocusableElements(): HTMLElement[] {
    if (!this.modalDialog) return [];

    const selectors = [
      'input:not([disabled])',
      'select:not([disabled])',
      'textarea:not([disabled])',
      'button:not([disabled])',
      'a[href]',
      '[tabindex]:not([tabindex="-1"])',
    ].join(', ');

    // Get all focusable elements in the modal (includes close button for tab cycle)
    return Array.from<HTMLElement>(this.modalDialog.nativeElement.querySelectorAll(selectors));
  }

  handleKeydown(event: KeyboardEvent): void {
    if (!this.show) {
      return;
    }

    if (event.key === 'Escape') {
      event.preventDefault();
      this.onClose();
      return;
    }

    // Handle Tab key for focus trapping
    if (event.key === 'Tab') {
      this.handleTabKey(event);
    }
  }

  // Focus trapping logic to keep focus within the modal when it's open, WCAG 2.1 compliant
  private handleTabKey(event: KeyboardEvent): void {
    const focusableElements = this.getFocusableElements();

    if (focusableElements.length === 0) {
      // No focusable elements, prevent tabbing
      event.preventDefault();
      return;
    }

    const firstElement = focusableElements[0];
    const lastElement = focusableElements[focusableElements.length - 1];

    if (event.shiftKey) {
      // Shift+Tab: moving backwards
      if (document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      }
    } else if (document.activeElement === lastElement) {
      // Tab: moving forwards
      event.preventDefault();
      firstElement.focus();
    }
  }

  onClose(): void {
    // Restore focus to the previously focused element
    if (this.previouslyFocusedElement) {
      this.previouslyFocusedElement.focus();
      this.previouslyFocusedElement = null;
    }
    this.modalClose.emit();
  }

  onBackdropClick(event: Event): void {
    // Close modal when clicking on the backdrop
    if (event.target === event.currentTarget) {
      this.onClose();
    }
  }
}
