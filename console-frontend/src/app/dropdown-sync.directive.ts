import { AfterViewInit, Directive, ElementRef, OnDestroy, inject } from '@angular/core';

type LitDropdown = HTMLElement & { updateComplete?: Promise<unknown> };

/**
 * Keeps the visible label of `<nldd-dropdown>` in sync with its slotted
 * `<select>`.
 *
 * The design-system component computes its display label once, on the
 * `slotchange` event. When the `<option>`s are produced by Angular control
 * flow (`@for`) or arrive from an async source, they are not yet present at
 * that moment, so the label renders empty even though the native `<select>`
 * already has a value selected. We re-run the component's own slot handler
 * after the options have rendered, and again whenever they change. Using the
 * supported `slotchange` path avoids touching the component's internals or
 * dispatching a `change` event (which would trigger app `(change)` handlers).
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

  ngAfterViewInit(): void {
    const dropdown = this.el.nativeElement;
    const select = dropdown.querySelector('select');
    if (!select) return;

    const resync = () =>
      dropdown.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));

    // Sync once the component has rendered its <slot> and the `@for` options
    // are in place. `updateComplete` covers the component's first render; the
    // rAF is a fallback in case the property is unavailable.
    dropdown.updateComplete?.then(resync);
    this.rafId = requestAnimationFrame(resync);

    // Keep in sync for async-loaded options and when re-opening a form to edit.
    this.observer = new MutationObserver(resync);
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
}
