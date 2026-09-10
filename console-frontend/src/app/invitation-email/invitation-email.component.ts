import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  DestroyRef,
  inject,
  input,
  signal,
} from '@angular/core';
import { buildInvitationEmail } from '../utils/invitation-email';
import AutofocusDirective from '../autofocus.directive';

import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/form-field';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/spacer';

/** How long the banner stands as a confirmation before going back to what it
 *  said, so copying a second time has something to change again. */
const COPIED_FOR = 5000;

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

  readonly copied = signal(false);

  readonly message = computed(() =>
    buildInvitationEmail({
      email: this.email(),
      permission: this.permission(),
      organization: this.organization(),
      consoleUrl: window.location.origin,
    }),
  );

  private copyTimer?: ReturnType<typeof setTimeout>;

  constructor() {
    // The sheet can be closed while the confirmation is still standing, and the
    // dialog's content is destroyed the moment its member is cleared.
    inject(DestroyRef).onDestroy(() => clearTimeout(this.copyTimer));
  }

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
      this.confirmCopied();
    } catch {
      this.copied.set(false);
    }
  }

  /** The banner is a live region, so changing what it says announces the copy.
   *  A notification cannot do it: this sits inside a sheet and a modal dialog,
   *  which are in the browser's top layer and paint over the notification
   *  region entirely. */
  private confirmCopied() {
    this.copied.set(true);
    clearTimeout(this.copyTimer);
    this.copyTimer = setTimeout(() => this.copied.set(false), COPIED_FOR);
  }
}
