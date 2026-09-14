import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, inject } from '@angular/core';
import ToastService from './toast.service';

/**
 * Rendered once at the app root. The notifications place themselves: the design
 * system moves them to one shared region and stacks them into a deck, so there
 * is nothing here about position, and nothing about timing either — a critical
 * one waits to be dismissed, the rest count down on their own and tell us with
 * `dismiss` when they are done.
 */
@Component({
  selector: 'app-toast',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    @for (notification of toast.notifications(); track notification.id) {
      <nldd-notification
        [attr.variant]="notification.variant"
        [attr.text]="notification.text"
        (dismiss)="toast.dismiss(notification.id)"
      ></nldd-notification>
    }
  `,
})
export default class ToastComponent {
  protected readonly toast = inject(ToastService);
}
