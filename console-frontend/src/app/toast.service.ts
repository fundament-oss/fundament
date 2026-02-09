import { Injectable, signal, inject } from '@angular/core';
import { Router, NavigationStart } from '@angular/router';

export interface Toast {
  message: string;
  type: 'success' | 'info' | 'warning' | 'error';
  id: number;
}

@Injectable({
  providedIn: 'root',
})
export class ToastService {
  private toastIdCounter = 0;

  private currentToast = signal<Toast | null>(null);

  private router = inject(Router);

  // When true, preserve the current toast through the next navigation
  private preserveThroughNextNavigation = false;

  // Expose the toast as a readonly signal
  toast = this.currentToast.asReadonly();

  constructor() {
    // Dismiss any toast when navigating to a new page
    this.router.events.subscribe((event) => {
      if (event instanceof NavigationStart) {
        if (this.preserveThroughNextNavigation) {
          // Clear the flag and keep the toast visible through the first navigation
          this.preserveThroughNextNavigation = false;
          return;
        }

        this.dismiss();
      }
    });
  }

  show(message: string, type: Toast['type'] = 'info') {
    const toast: Toast = {
      message,
      type,
      id: this.toastIdCounter,
    };
    this.toastIdCounter += 1;
    this.currentToast.set(toast);

    // Mark that we should preserve this toast across one navigation cycle
    // This covers the common pattern where code shows a toast and then immediately navigates to a details page (set-then-navigate)
    this.preserveThroughNextNavigation = true;
  }

  dismiss() {
    this.currentToast.set(null);
  }

  success(message: string) {
    this.show(message, 'success');
  }

  info(message: string) {
    this.show(message, 'info');
  }

  warning(message: string) {
    this.show(message, 'warning');
  }

  error(message: string) {
    this.show(message, 'error');
  }
}
