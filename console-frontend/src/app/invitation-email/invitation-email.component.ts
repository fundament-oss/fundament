import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  input,
} from '@angular/core';
import { buildInvitationEmail } from '../utils/invitation-email';
import AutofocusDirective from '../autofocus.directive';
import { NotificationService } from '../notification.service';

import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/form-field';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/spacer';

/**
 * The message an admin sends by hand after creating an invitation, with the two
 * ways out: the mail client, or the clipboard.
 *
 * Both ways, because a mailto: is not reliable. A browser with no registered
 * mail handler — Outlook Web on a managed desktop is the common one — does
 * nothing at all when the link is followed, and gives no sign that it did
 * nothing. Copy is what makes that recoverable.
 *
 * Presentational: it is shown inside the invite sheet just after creating an
 * invitation, and inside a dialog on the members page for one made long ago.
 */
@Component({
  selector: 'app-invitation-email',
  // The panel carries its own focus: it is rendered into a sheet that is already
  // standing open and into a dialog that is about to open, and the directive
  // handles both. Without it the sheet's focus lock strands the user on the
  // input that was just removed.
  imports: [AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './invitation-email.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InvitationEmailComponent {
  /** The invited address. */
  readonly email = input.required<string>();

  /** The organization permission they were invited with. */
  readonly permission = input.required<string>();

  /** What the organization is called on screen. */
  readonly organization = input.required<string>();

  /** What the banner says when nothing has just happened. The two callers
   *  arrive from different directions: one just created this invitation, the
   *  other is looking up one that already existed. */
  readonly idleText = input('Invitation created');

  private notificationService = inject(NotificationService);

  readonly message = computed(() =>
    buildInvitationEmail({
      email: this.email(),
      permission: this.permission(),
      organization: this.organization(),
      consoleUrl: window.location.origin,
    }),
  );

  async copy() {
    const { body } = this.message();

    try {
      // The clipboard API needs a secure context. The console is served over
      // HTTPS everywhere it is deployed, but a plain-HTTP port-forward for
      // debugging is not, and losing copy there loses the only way through.
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(body);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = body;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
      this.notificationService.success('Message copied to clipboard');
    } catch {
      this.notificationService.error('Failed to copy the message. Please copy it manually.');
    }
  }
}
