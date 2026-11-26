import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { ConnectError } from '@connectrpc/connect';
import { MEMBER } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import OrganizationChangedError from '../organization-changed-error';
import AuthnApiService from '../authn-api.service';
import InvitationEmailComponent from '../invitation-email/invitation-email.component';
import { formatTimeAgo } from '../utils/date-format';
import { organizationPermissionLabel } from '../utils/role-label';
import copyWithNotice from '../utils/copy-with-notice';
import type { Member } from '../../generated/v1/member_pb';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/list-item-segment';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

/** The memberships the list shows. ListMembers also returns a declined
 *  invitation (declining sets the status, it does not soft-delete the row), and
 *  a link to one must not open as a member with Remove on it. */
const SHOWN_STATUSES = new Set(['pending', 'accepted']);

/**
 * Everything about one organization member or invitation, as a route of its own
 * so a row can be a real link: shareable, openable in a new tab, closed by the
 * back button. `:memberId` is the membership ID (Member.id), not the user ID.
 */
@Component({
  selector: 'app-organization-member-sheet',
  imports: [DialogSyncDirective, SheetSyncDirective, InvitationEmailComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './organization-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class OrganizationMemberSheetComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private memberClient = inject(MEMBER);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  private currentUser = toSignal(inject(AuthnApiService).currentUser$);

  memberId = signal('');

  isLoading = signal(true);

  /** Why the member list could not be fetched. Shown only when the cache has
   *  no copy of this member either, so a failed load never reads as "not
   *  found". */
  loadError = signal<string | null>(null);

  saving = signal(false);

  showRemoveModal = signal(false);

  showInvitationEmail = signal(false);

  /** Read from the shared cache, so an action here shows in the list behind
   *  the sheet as soon as the cache has reloaded. */
  member = computed<Member | null>(
    () =>
      this.organizationData
        .members()
        .find((m) => m.id === this.memberId() && SHOWN_STATUSES.has(m.status)) ?? null,
  );

  isInvitation = computed(() => this.member()?.status === 'pending');

  isCurrentUser = computed(() => {
    const userId = this.currentUser()?.id;
    return !!userId && this.member()?.userId === userId;
  });

  /** An invitation has no name of its own yet: the address is what it is called. */
  displayName = computed(() => {
    const member = this.member();
    if (!member) return 'Member';
    return (
      (this.isInvitation() ? member.email || member.name : member.name || member.email) || 'Member'
    );
  });

  dateLabel = computed(() => (this.isInvitation() ? 'Invited' : 'Joined'));

  dateText = computed(() => {
    const created = this.member()?.created;
    return formatTimeAgo(created ? timestampDate(created) : undefined);
  });

  organizationName = computed(() => this.organizationData.currentOrganizationDisplayName());

  /** The dialog is in the DOM before the member is loaded, so this has to read
   *  as a sentence with the blank still open. */
  removeMemberTitle = computed(() => {
    const name = this.member() ? this.displayName() : '';
    return name
      ? `Remove ${name} from this organization?`
      : 'Remove this member from this organization?';
  });

  permissionLabel = organizationPermissionLabel;

  ngOnInit() {
    this.memberId.set(this.route.snapshot.params['memberId'] ?? '');
    this.load();
  }

  async load() {
    this.isLoading.set(true);
    this.loadError.set(null);
    try {
      // Through the shared cache, so the sidebar's count is of this same list.
      await this.organizationData.reloadMembers();
    } catch (err) {
      // The cache keeps what it had, so a member it holds is still shown.
      // rawMessage leaves out the "[code]" prefix ConnectError's message has.
      this.loadError.set(ConnectError.from(err).rawMessage);
    } finally {
      this.isLoading.set(false);
    }
  }

  copyUserId(member: Member): Promise<void> {
    return copyWithNotice(this.notificationService, member.userId, 'User ID');
  }

  copyEmail(member: Member): Promise<void> {
    if (!member.email) return Promise.resolve();
    return copyWithNotice(this.notificationService, member.email, 'Email address');
  }

  /** The same flip as the row's menu. Reversible in one click, so no
   *  confirmation. */
  async setPermission(member: Member) {
    // The organization viewer role is temporarily disabled, so an admin stays
    // an admin. Remove this guard to bring the viewer role back.
    if (member.permission === 'admin') return;
    const permission = member.permission === 'admin' ? 'viewer' : 'admin';
    const who = this.displayName();
    const becomes = this.isInvitation() ? 'is invited as' : 'is now';

    this.saving.set(true);
    try {
      await firstValueFrom(this.memberClient.updateMemberPermission({ id: member.id, permission }));
      this.notificationService.success(`${who} ${becomes} ${permission}`);
      await this.load();
    } catch (err) {
      this.notificationService.error(
        err instanceof Error ? `Role not changed: ${err.message}` : 'Role not changed',
      );
    } finally {
      this.saving.set(false);
    }
  }

  /** Removing a member and cancelling an invitation are the same call: both
   *  end the membership. */
  async confirmRemove() {
    const member = this.member();
    if (!member) return;
    const invitation = this.isInvitation();
    const who = this.displayName();

    this.saving.set(true);
    try {
      await firstValueFrom(this.memberClient.deleteMember({ id: member.id }));
      this.showRemoveModal.set(false);
      this.notificationService.success(
        invitation ? `Invitation for '${who}' cancelled` : `'${who}' removed from the organization`,
      );
      try {
        await this.organizationData.reloadMembers();
      } catch (err) {
        // The removal went through; only the refresh failed. Without this the
        // list behind the sheet would show them again as it closes. Unless the
        // organization changed meanwhile: the cache is no longer the one they
        // were in, and the new organization's list is on its way.
        if (!(err instanceof OrganizationChangedError)) {
          this.organizationData.forgetMember(member.id);
        }
      }
      this.onClose();
    } catch (err) {
      this.showRemoveModal.set(false);
      const failed = invitation ? 'Invitation not cancelled' : 'Member not removed';
      this.notificationService.error(err instanceof Error ? `${failed}: ${err.message}` : failed);
    } finally {
      this.saving.set(false);
    }
  }

  /** Back to the list, which shows whatever the cache now holds. */
  onClose(): void {
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
