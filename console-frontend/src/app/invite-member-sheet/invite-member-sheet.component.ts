import {
  Component,
  computed,
  inject,
  input,
  signal,
  effect,
  Output,
  EventEmitter,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ConnectError, Code } from '@connectrpc/connect';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { INVITE } from '../../connect/tokens';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import InvitationEmailComponent from '../invitation-email/invitation-email.component';
import { organizationPermissionLabel } from '../utils/role-label';

import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/radio-button';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';
/**
 * Inviting someone into the organization, from wherever you were. The shell owns
 * this sheet rather than the member list, so the toolbar can open it over any
 * page; the list hears about a new invitation through the organization data.
 */
@Component({
  selector: 'app-invite-member-sheet',
  imports: [SheetSyncDirective, AutofocusDirective, InvitationEmailComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './invite-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InviteMemberSheetComponent {
  readonly show = input(false);

  @Output() closed = new EventEmitter<void>();

  private inviteClient = inject(INVITE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  isSubmitting = signal(false);

  inviteEmail = signal('');

  inviteSubmitted = signal(false);

  invitePermission = signal('viewer');

  inviteError = signal<string | null>(null);

  /** The sheet does not close on a successful invitation any more: nothing is
   *  sent, so the admin still has a message to deliver, and this is the one
   *  moment we know everything needed to write it. */
  step = signal<'form' | 'created'>('form');

  /** Snapshotted at submit, not read back off the form: "Invite another person"
   *  clears the fields while the created step may still be showing them. */
  createdEmail = signal('');

  createdPermission = signal('viewer');

  /** How many were created without leaving the sheet, so the notification on the
   *  way out can account for all of them rather than only the last. */
  createdCount = signal(0);

  organizationName = computed(() => this.organizationDataService.currentOrganizationDisplayName());

  /** Named with their scope, the way the tags on the members list are: this is
   *  standing in the organization, not in one of its projects. The label comes
   *  from role-label so this list, the tag on the row, the menu item that
   *  changes it and now the invitation email all say the same words. */
  permissionOptions = [
    {
      value: 'viewer',
      label: organizationPermissionLabel('viewer'),
      description: 'Can look at the organization, its clusters and its members.',
    },
    {
      value: 'admin',
      label: organizationPermissionLabel('admin'),
      description: 'Can also create clusters, invite members and reach every project.',
    },
  ];

  constructor() {
    // The sheet outlives the page it was opened over, so opening is the moment
    // to start from nothing rather than from whoever you invited last time.
    effect(() => {
      if (!this.show()) return;
      this.resetForm();
      this.step.set('form');
      this.createdCount.set(0);
    });
  }

  private resetForm() {
    this.inviteEmail.set('');
    this.inviteSubmitted.set(false);
    this.invitePermission.set('viewer');
    this.inviteError.set(null);
    this.isSubmitting.set(false);
  }

  /** Back to an empty form with the sheet still standing, for onboarding
   *  several people in one go. */
  inviteAnother() {
    this.resetForm();
    this.step.set('form');
  }

  onClose() {
    // Raised here rather than at the moment of success: the sheet is a modal
    // <dialog>, so it is in the browser's top layer and paints over the
    // notification region entirely. On the way out it is seen.
    const created = this.createdCount();
    if (created === 1) {
      this.notificationService.success(
        `'${this.createdEmail()}' invited as ${this.createdPermission()}`,
      );
    } else if (created > 1) {
      this.notificationService.success(`${created} invitations created`);
    }
    this.closed.emit();
  }

  async submitInvitation(event?: Event) {
    event?.preventDefault();
    this.inviteSubmitted.set(true);

    const email = this.inviteEmail().trim();
    const permission = this.invitePermission();

    if (!email) {
      return;
    }

    this.isSubmitting.set(true);
    this.inviteError.set(null);

    try {
      await withIdempotency((opts) => this.inviteClient.inviteMember({ email, permission }, opts), {
        signal: this.idempotency.reset(),
      });
      this.createdEmail.set(email);
      this.createdPermission.set(permission);
      this.createdCount.update((count) => count + 1);
      this.step.set('created');
      // The list of members is a page of its own, and it may well be the page
      // behind this sheet, so it hears about the invitation from here.
      this.organizationDataService.membersChanged.update((count) => count + 1);
    } catch (err: unknown) {
      if (err instanceof ConnectError) {
        if (err.code === Code.AlreadyExists) {
          this.inviteError.set('This email address is already in use.');
        } else if (err.code === Code.InvalidArgument) {
          this.inviteError.set('Please enter a valid email address.');
        } else {
          this.inviteError.set('Failed to invite member. Please try again.');
        }
      } else {
        this.inviteError.set('Failed to invite member. Please try again.');
      }
    } finally {
      this.isSubmitting.set(false);
    }
  }
}
