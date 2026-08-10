import {
  Component,
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

/**
 * Inviting someone into the organization, from wherever you were. The shell owns
 * this sheet rather than the member list, so the toolbar can open it over any
 * page; the list hears about a new invitation through the organization data.
 */
@Component({
  selector: 'app-invite-member-sheet',
  imports: [SheetSyncDirective, AutofocusDirective],
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

  inviteEmailDirty = signal(false);

  invitePermission = signal('viewer');

  inviteError = signal<string | null>(null);

  permissionOptions = [
    {
      value: 'viewer',
      label: 'Viewer',
      description: 'Can look at the organization, its clusters and its members.',
    },
    {
      value: 'admin',
      label: 'Admin',
      description: 'Can also create clusters, invite members and reach every project.',
    },
  ];

  constructor() {
    // The sheet outlives the page it was opened over, so opening is the moment
    // to start from nothing rather than from whoever you invited last time.
    effect(() => {
      if (!this.show()) return;
      this.inviteEmail.set('');
      this.inviteEmailDirty.set(false);
      this.invitePermission.set('viewer');
      this.inviteError.set(null);
      this.isSubmitting.set(false);
    });
  }

  onClose() {
    this.closed.emit();
  }

  async submitInvitation(event?: Event) {
    event?.preventDefault();

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
      this.closed.emit();
      this.notificationService.success(`'${email}' invited as ${permission}`);
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
