import {
  AfterViewInit,
  Directive,
  ElementRef,
  OnDestroy,
  afterEveryRender,
  inject,
} from '@angular/core';

type LitDropdown = HTMLElement & { updateComplete?: Promise<unknown> };

/**
 * Keeps the visible label of `<nldd-dropdown>` in sync with its slotted
 * `<select>`.
 *
 * The design-system component computes its display label only on the
 * `slotchange` event and on the select's `change` event. When the `<option>`s
 * are produced by Angular control flow (`@for`) or arrive from an async source,
 * they are not yet present at slot time, so the label renders empty even though
 * the native `<select>` already has a value selected. And when the value is set
 * programmatically — e.g. by `[ngModel]` on opening a form to edit — neither
 * event fires, so the label keeps the previously-shown text.
 *
 * We re-run the component's own slot handler after the options have rendered,
 * whenever the options change, and whenever the select's value changes (which
 * covers `[ngModel]` writes). Using the supported `slotchange` path avoids
 * touching the component's internals or dispatching a `change` event (which
 * would trigger app `(change)` handlers).
 */
@Directive({
  // Element selector by design: this label-sync fix must auto-apply to every
  // design-system `<nldd-dropdown>` without an opt-in attribute on each usage.
  // eslint-disable-next-line @angular-eslint/directive-selector
  selector: 'nldd-dropdown',
})
export default class DropdownSyncDirective implements AfterViewInit, OnDestroy {
  private readonly el = inject<ElementRef<LitDropdown>>(ElementRef);

  private observer?: MutationObserver;

  private rafId?: number;

  private lastValue?: string;

  constructor() {
    // The component recomputes its label on `slotchange`/`change` only, not when
    // the slotted <select>'s value is set programmatically (e.g. [ngModel] when
    // opening a form to edit a record). Re-sync whenever the value moves.
    afterEveryRender(() => {
      const select = this.el.nativeElement.querySelector('select');
      if (!select || select.value === this.lastValue) return;
      this.lastValue = select.value;
      this.resync();
    });
  }

  ngAfterViewInit(): void {
    const select = this.el.nativeElement.querySelector('select');
    if (!select) return;

    // Sync once the component has rendered its <slot> and the `@for` options
    // are in place. `updateComplete` covers the component's first render; the
    // rAF is a fallback in case the property is unavailable.
    this.el.nativeElement.updateComplete?.then(() => this.resync());
    this.rafId = requestAnimationFrame(() => this.resync());

    // Keep in sync for async-loaded options and any plain `selected`-attribute
    // dropdowns that don't go through [ngModel].
    this.observer = new MutationObserver(() => this.resync());
    this.observer.observe(select, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['selected'],
    });
  }

  ngOnDestroy(): void {
    this.observer?.disconnect();
    if (this.rafId !== undefined) cancelAnimationFrame(this.rafId);
  }

  private resync(): void {
    this.el.nativeElement.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  }
}
