import {
  Directive,
  ElementRef,
  Injector,
  OnDestroy,
  OnInit,
  afterNextRender,
  effect,
  inject,
  input,
} from '@angular/core';
import isPresenting from './presentation/presenting';

type SheetElement = HTMLElement & { show(): void; hide(): void };

/** Whether the last input was a pointer, by the keys the design system counts
 *  as keyboard use. Its own tracker is not exported. */
let pointerInput = true;
const NAVIGATION_KEYS = [
  'Tab',
  'ArrowUp',
  'ArrowDown',
  'ArrowLeft',
  'ArrowRight',
  'Enter',
  ' ',
  'Escape',
];
if (typeof document !== 'undefined') {
  document.addEventListener(
    'pointerdown',
    () => {
      pointerInput = true;
    },
    true,
  );
  document.addEventListener(
    'keydown',
    (event) => {
      if (NAVIGATION_KEYS.includes(event.key)) pointerInput = false;
    },
    true,
  );
}

/**
 * Holds back the tooltip of the icon button a closing sheet hands focus back to.
 *
 * A modal <dialog> returns focus on close to what had it before: for a sheet
 * opened from the Create menu that is the + button, and nldd-tooltip shows on any
 * focus that is not a touch. After a mouse click on Create project, the button's
 * "Create" tooltip hung under the notification on the next page. Focus stays
 * where the dialog put it, so the keyboard and a screen reader keep their place;
 * only the visual tooltip waits until focus leaves, and only after a pointer.
 */
export function quietRestoredTooltip(event: FocusEvent, sheet: HTMLElement): void {
  if (!pointerInput) return;
  const button = event
    .composedPath()
    .find(
      (node): node is HTMLElement =>
        node instanceof HTMLElement && node.localName === 'nldd-icon-button',
    );
  if (!button || sheet.contains(button) || button.hasAttribute('tooltip-timing')) return;
  button.setAttribute('tooltip-timing', 'never');
  button.addEventListener('focusout', () => button.removeAttribute('tooltip-timing'), {
    once: true,
  });
}

/**
 * Re-arms the wiring of every `nldd-form-field` in a subtree that was just moved.
 *
 * A field builds its wiring, the label and the link to its `nldd-validation-list`, from its own
 * child-list observer. Moving the element runs `disconnectedCallback`, and what comes back does
 * not run that wiring again by itself. A throwaway child node produces exactly the mutation that
 * does.
 */
export function rewireFormFields(root: HTMLElement): void {
  root.querySelectorAll('nldd-form-field').forEach((field) => {
    const probe = document.createComment('nldd-form-field-rewire');
    field.appendChild(probe);
    probe.remove();
  });
}

/**
 * Syncs an `nldd-sheet` with a boolean signal, and portals the sheet to `document.body`.
 *
 * The portal is required: the router outlet lives inside an `nldd-split-view-pane`, and a sheet
 * left in that flow steals pane height. Moving the element does not disturb its Angular bindings,
 * since the view context is unchanged.
 */
@Directive({
  selector: 'nldd-sheet[appSheetSync]',
})
export default class SheetSyncDirective implements OnInit, OnDestroy {
  private el = inject<ElementRef<SheetElement>>(ElementRef);

  private injector = inject(Injector);

  show = input(false);

  private prev = false;

  // While the sheet is open, focus outside it is only ever the dialog handing it
  // back on close. Capture, so the button hears the setting before its tooltip
  // hears the focus.
  private onFocusIn = (event: FocusEvent) => quietRestoredTooltip(event, this.el.nativeElement);

  // Only the sheet's own: a popover inside it sends open and close up through it.
  private onOpen = (event: Event) => {
    if (event.target === this.el.nativeElement) {
      document.addEventListener('focusin', this.onFocusIn, true);
    }
  };

  // The sheet emits close after the dialog has given focus back.
  private onClose = (event?: Event) => {
    if (!event || event.target === this.el.nativeElement) {
      document.removeEventListener('focusin', this.onFocusIn, true);
    }
  };

  ngOnInit(): void {
    const el = this.el.nativeElement;
    document.body.appendChild(el);
    rewireFormFields(el);
    el.addEventListener('open', this.onOpen);
    el.addEventListener('close', this.onClose);
  }

  ngOnDestroy(): void {
    const el = this.el.nativeElement;
    el.removeEventListener('open', this.onOpen);
    el.removeEventListener('close', this.onClose);
    this.onClose();
    el.remove();
  }

  private sync = effect(() => {
    const show = this.show();
    if (show && !this.prev) {
      this.open();
    } else if (!show && this.prev) {
      this.el.nativeElement.hide();
    }
    this.prev = show;
  });

  /**
   * Opens the sheet, leaving the keyboard to the walkthrough when it is driving.
   *
   * `nldd-sheet.show()` focuses its `[autofocus]` element synchronously, before it
   * even emits `open`, so hiding the mark for the length of that one call is the
   * last point where the decision can still be made — and it has to be made per
   * open, since the walkthrough can start long after the field first rendered.
   * Focus inside a field there swallows the overlay's arrow keys.
   */
  private open(): void {
    const el = this.el.nativeElement;
    if (!isPresenting()) {
      el.show();
      this.focusMarkedField();
      return;
    }
    const marked = Array.from(el.querySelectorAll('[autofocus]'));
    marked.forEach((target) => target.removeAttribute('autofocus'));
    el.show();
    marked.forEach((target) => target.setAttribute('autofocus', ''));
  }

  /**
   * Focuses what `show()` would have focused, for the one open where the mark is
   * not there yet.
   *
   * The sheet reads `[autofocus]` synchronously inside `show()`, and
   * AutofocusDirective sets that attribute a render later — so the first open of
   * a sheet, the open that renders its fields, finds nothing to focus. Every
   * open after it finds the mark in place, which is why only the first one
   * landed on nothing.
   */
  private focusMarkedField(): void {
    afterNextRender(
      () => {
        // A task rather than a microtask: the field keeps its real input in
        // shadow DOM, which Lit renders on a microtask, and the mark itself is
        // set from a render hook of its own that may run after this one. By the
        // time this runs both are settled, and an open that focused the field
        // itself has nothing left to do.
        setTimeout(() => {
          const target = this.el.nativeElement.querySelector<HTMLElement>('[autofocus]');
          if (target && document.activeElement !== target) target.focus();
        });
      },
      { injector: this.injector },
    );
  }
}
