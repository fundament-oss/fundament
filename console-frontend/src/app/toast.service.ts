import { Injectable, signal, inject } from '@angular/core';
import { Router, NavigationEnd } from '@angular/router';

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
  // When true, the next navigation will NOT auto-dismiss the current toast
  private skipNextDismiss = false;

  // Expose the toast as a readonly signal
  toast = this.currentToast.asReadonly();

  constructor() {
    // Dismiss any toast after successful navigation to a new page
    this.router.events.subscribe((event) => {
      if (event instanceof NavigationEnd) {
        if (this.skipNextDismiss) {
          // Clear the flag and keep the toast visible on the first navigation
          this.skipNextDismiss = false;
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
      id: this.toastIdCounter++,
    };
    this.currentToast.set(toast);

    // Mark that we should preserve this toast across one navigation cycle
    // This covers the common pattern where code shows a toast and then immediately navigates to a details page (set-then-navigate)
    this.skipNextDismiss = true;
    // Ensure the toast is visible to the user by scrolling to top
    window.scrollTo({ top: 0, behavior: 'smooth' });
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
