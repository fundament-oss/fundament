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

  ngOnInit(): void {
    document.body.appendChild(this.el.nativeElement);
    rewireFormFields(this.el.nativeElement);
  }

  ngOnDestroy(): void {
    this.el.nativeElement.remove();
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
