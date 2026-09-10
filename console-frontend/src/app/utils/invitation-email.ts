import { organizationPermissionLabel } from './role-label';

/**
 * The message an admin sends by hand after creating an invitation.
 *
 * Nothing sends mail here. There is no mailer in the platform, and
 * InviteMemberResponse carries only the membership row id: no token, no link,
 * no accept route. The invitee joins by signing in with the invited address,
 * so the message is instructions rather than a link, and the admin is the one
 * who delivers it.
 *
 * One module because two places show the same message — the invite sheet just
 * after creating one, and the row menu of a pending member weeks later — and a
 * message that differs between them is a message the admin has to read twice.
 */

/** Windows mail clients treat a bare \n as no break at all, so the body is
 *  built with CRLF and stays that way through encodeURIComponent. */
const BREAK = '\r\n';

const APP_NAME = 'Fundament Console';

export interface InvitationEmailOptions {
  /** The invited address, which is also the address they must sign in with. */
  email: string;
  /** The organization permission, "viewer" or "admin" per the proto. */
  permission: string;
  /** What the organization is called on screen: its alias, or its name. */
  organization: string;
  /** Where the console lives, normally window.location.origin. */
  consoleUrl: string;
}

export interface InvitationEmail {
  to: string;
  subject: string;
  body: string;
  mailto: string;
}

/**
 * The role as it reads inside a sentence.
 *
 * organizationPermissionLabel is the one place these labels agree — the tag on
 * a row, the radio list in the sheet, the menu item that changes it — and the
 * email is a fourth reader of the same thing. It hands back a label ("Organization
 * viewer"), and mid-sentence that wants a lower-case first letter. Only the first
 * letter, so a permission the function did not recognise and handed straight back
 * is not mangled beyond recognition.
 */
function roleInSentence(permission: string): string {
  const label = organizationPermissionLabel(permission);
  return label.charAt(0).toLowerCase() + label.slice(1);
}

export function buildInvitationEmail(opts: InvitationEmailOptions): InvitationEmail {
  const { email, permission, organization, consoleUrl } = opts;

  const subject = `Invitation to ${organization} on ${APP_NAME}`;

  // No deep link: there is no per-invitation URL. The console root is enough,
  // because a pending invitation forces the organization picker on load, which
  // is where the Accept button is.
  const body = [
    'Hi,',
    '',
    `You have been invited to join ${organization} on ${APP_NAME} as an ${roleInSentence(permission)}.`,
    '',
    'To join:',
    `1. Go to ${consoleUrl}`,
    `2. Sign in with this email address (${email})`,
    `3. Accept the pending invitation for ${organization}`,
    '',
    'If you do not have an account yet, signing in with this address creates one.',
  ].join(BREAK);

  const mailto = `mailto:${encodeURIComponent(email)}?subject=${encodeURIComponent(
    subject,
  )}&body=${encodeURIComponent(body)}`;

  return { to: email, subject, body, mailto };
}
