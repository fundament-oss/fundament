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
      class="pointer-events-none fixed inset-x-0 top-auto bottom-6 z-50 mx-auto w-fit border-0 bg-transparent p-0 transition-opacity duration-300"
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
    effect(() => {
      const msg = this.toast.message();
      const el = this.toastEl()?.nativeElement;
      if (!el) return;
      try {
        if (msg) el.showPopover();
        else el.hidePopover();
      } catch {
        // Popover already in the requested state — safe to ignore.
      }
    });
  }
}
