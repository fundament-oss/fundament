import { Injectable, signal } from '@angular/core';

// Drives the single app-wide <app-toast/> overlay (see ./toast.ts). Centralized
// so every route shares one instance rendered at the app root, in the browser's
// top layer — avoiding the stacking issues of a per-page fixed-position toast
// getting hidden behind native <dialog>-based sheets/modals.
@Injectable({ providedIn: 'root' })
export default class ToastService {
  readonly message = signal<string | null>(null);

  private timeout: ReturnType<typeof setTimeout> | undefined;

  show(msg: string): void {
    this.message.set(msg);
    clearTimeout(this.timeout);
    this.timeout = setTimeout(() => {
      this.message.set(null);
    }, 2000);
  }
}
