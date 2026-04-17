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
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';

@Component({
  selector: 'app-modal',
  imports: [],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
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
    // Exclude the close button (ndd-icon-button with text="Close") from initial focus
    const elements = this.getFocusableElements().filter(
      (el) => el.getAttribute('text') !== 'Close',
    );
    const element = elements[0] || this.modalDialog?.nativeElement;
    if (!element) {
      return false;
    }

    ModalComponent.focusElement(element);

    const shadowRoot = (element as HTMLElement & { shadowRoot?: ShadowRoot }).shadowRoot;
    return document.activeElement === element || !!shadowRoot?.activeElement;
  }

  // Focus an element, piercing into shadow DOM when the host doesn't delegate focus natively.
  // Pass focusVisible: true so the browser shows the focus ring even for programmatic focus.
  private static focusElement(element: HTMLElement): void {
    const shadowRoot = (element as HTMLElement & { shadowRoot?: ShadowRoot }).shadowRoot;
    const focusTarget =
      shadowRoot?.querySelector<HTMLElement>('input:not([disabled]), button:not([disabled])') ??
      element;
    focusTarget.focus({ focusVisible: true } as FocusOptions & { focusVisible?: boolean });
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
      'ndd-text-field:not([disabled])',
      'ndd-search-field:not([disabled])',
      'ndd-button:not([disabled])',
      'ndd-icon-button:not([disabled])',
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
