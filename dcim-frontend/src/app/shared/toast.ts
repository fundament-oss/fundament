import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  viewChild,
} from '@angular/core';
import ToastService from './toast.service';

type PopoverElement = HTMLElement & { showPopover(): void; hidePopover(): void };

// Must match the `duration-300` on the toast's opacity transition below.
const FADE_MS = 300;

// Rendered once at the app root. Uses the Popover API (rather than plain
// `position: fixed`) so the toast is promoted to the browser's top layer —
// the same layer native <dialog>-based nldd-sheet/nldd-modal-dialog use —
// meaning it stays visible above an open sheet instead of being painted
// behind it.
@Component({
  selector: 'app-toast',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <div
      #toastEl
      popover="manual"
      class="pointer-events-none fixed top-auto bottom-6 z-50 w-fit -translate-x-1/2 border-0 bg-transparent p-0 transition-opacity duration-300"
      [style.left]="'calc(50% + ' + toast.offsetPx() + 'px)'"
      [class.opacity-0]="!toast.message()"
      [class.opacity-100]="!!toast.message()"
      aria-live="polite"
      aria-atomic="true"
    >
      <div
        class="flex items-center gap-2 rounded-full bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-xl ring-1 ring-slate-700"
      >
        <nldd-icon
          name="info-circle"
          style="width: 14px; height: 14px"
          aria-hidden="true"
        ></nldd-icon>
        <span>{{ toast.message() }}</span>
      </div>
    </div>
  `,
})
export default class AppToast {
  protected readonly toast = inject(ToastService);

  private readonly toastEl = viewChild<ElementRef<PopoverElement>>('toastEl');

  constructor() {
    effect((onCleanup) => {
      const msg = this.toast.message();
      const el = this.toastEl()?.nativeElement;
      if (!el) return;

      if (msg) {
        AppToast.setPopover(el, true);
        return;
      }

      // Hiding is deferred by the length of the opacity transition: a popover
      // is display:none while closed, so calling hidePopover() as the element
      // fades would cut straight to invisible and the fade-out would never
      // render. The class binding has already flipped to opacity-0 by now.
      const timer = setTimeout(() => AppToast.setPopover(el, false), FADE_MS);
      onCleanup(() => clearTimeout(timer));
    });
  }

  private static setPopover(el: PopoverElement, open: boolean): void {
    try {
      if (open) el.showPopover();
      else el.hidePopover();
    } catch {
      // Popover already in the requested state, or the element has since been
      // detached — safe to ignore.
    }
  }
}
