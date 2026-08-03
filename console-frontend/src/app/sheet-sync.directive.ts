import { Directive, ElementRef, OnDestroy, OnInit, effect, inject, input } from '@angular/core';

type SheetElement = HTMLElement & { show(): void; hide(): void };

/**
 * Re-arms the error-text wiring of every `nldd-form-field` in a subtree that was just moved.
 *
 * `nldd-form-field` watches the `invalid` attribute of its slotted input with a MutationObserver
 * and toggles the matching `nldd-form-field-error-text`. Moving the field disconnects it, which
 * tears that observer down, and the field only rebuilds it from its own child-list observer — so
 * without a nudge the error text stays hidden forever. A throwaway child node produces exactly
 * that child-list mutation.
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
      this.el.nativeElement.show();
    } else if (!show && this.prev) {
      this.el.nativeElement.hide();
    }
    this.prev = show;
  });
}
