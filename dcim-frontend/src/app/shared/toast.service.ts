import { Injectable, signal } from '@angular/core';

/** What a message is: a failure stays until you dismiss it, the rest leaves. */
export type NotificationVariant = 'success' | 'warning' | 'critical' | 'neutral';

export interface AppNotification {
  id: number;
  variant: NotificationVariant;
  text: string;
}

/**
 * The app's messages, rendered once at the root by nldd-notification.
 *
 * The design system places a notification itself, in one shared region, and
 * stacks several into a deck. So this holds the list and nothing else: no
 * position, no timer. A `critical` one never leaves on its own, which is the
 * point of it, and the others count down in the component.
 */
@Injectable({ providedIn: 'root' })
export default class ToastService {
  readonly notifications = signal<AppNotification[]>([]);

  private nextId = 0;

  /** A failure. It stays on screen until it is dismissed. */
  error(text: string): void {
    this.push('critical', text);
  }

  /** Something worked, and the result is not visible on its own. */
  success(text: string): void {
    this.push('success', text);
  }

  /** Half worked: what you asked for happened, the rest did not. */
  warning(text: string): void {
    this.push('warning', text);
  }

  /** Plain information. */
  info(text: string): void {
    this.push('neutral', text);
  }

  dismiss(id: number): void {
    this.notifications.update((list) => list.filter((n) => n.id !== id));
  }

  private push(variant: NotificationVariant, text: string): void {
    this.nextId += 1;
    const id = this.nextId;
    this.notifications.update((list) => [...list, { id, variant, text }]);
  }
}
