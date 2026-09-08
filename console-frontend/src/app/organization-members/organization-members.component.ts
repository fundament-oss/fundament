import {
  Component,
  inject,
  OnInit,
  signal,
  computed,
  effect,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { ActivatedRoute, Router, RouterOutlet } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import PageNavService from '../page-nav.service';
import { OverlayService } from '../overlay.service';
import { OrganizationDataService } from '../organization-data.service';
import AuthnApiService from '../authn-api.service';
import { MEMBER, INVITE } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { formatTimeAgo } from '../utils/date-format';
import '@nldd/design-system/search-field';
import opensElsewhere from '../opens-elsewhere';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/avatar';
import '@nldd/design-system/badge';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/tag';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';

interface OrganizationMember {
  id: string;
  name: string;
  email?: string;
  externalRef?: string;
  permission: string;
  status: string;
  isCurrentUser?: boolean;
  created?: Date;
}

/** 'admin' reads as a value, 'Admin' as a label. The tag shows the label. */
const permissionLabel = (permission: string): string =>
  permission ? permission[0].toUpperCase() + permission.slice(1) : permission;

/** Name or email; a member without a name is only findable by their address. */
const filterByQuery = (members: OrganizationMember[], query: string): OrganizationMember[] => {
  const needle = query.trim().toLowerCase();
  if (!needle) return members;
  return members.filter((member) =>
    `${member.name} ${member.email}`.toLowerCase().includes(needle),
  );
};

/** How long a retry shows as running before its result lands. */
const MIN_RETRY_FEEDBACK_MS = 2000;

type MemberSort = 'status' | 'permission' | 'joined' | 'joined-oldest';

/** Only Joined needs its direction spelled out: with status and role you can
 *  see which way the list runs, with dates you cannot. */
/** What the filter button says while its menu is closed. */
const FILTER_LABELS: Record<string, string> = {
  all: 'All',
  invitations: 'Invitations',
  members: 'Members',
};

// The key is the field, the label is the word the app uses for it everywhere
// else: the invite form and both sort menus say Role, so the button does too.
const SORT_LABELS: Record<MemberSort, string> = {
  status: 'Status',
  permission: 'Role',
  joined: 'Joined (newest first)',
  'joined-oldest': 'Joined (oldest first)',
};

/** Invitations are the rows you still have to act on, your own row explains
 *  itself, the rest are members. */
const statusRank = (member: OrganizationMember): number => {
  if (member.status === 'pending') return 0;
  if (member.isCurrentUser) return 1;
  return 2;
};

const joinedDescending = (a: OrganizationMember, b: OrganizationMember): number =>
  (b.created?.getTime() ?? 0) - (a.created?.getTime() ?? 0);

/**
 * Sorting by role keeps the status order inside each role rather than
 * interleaving the two: admins first, and within them the invitation you still
 * have to act on. One axis at a time stays predictable.
 */
const comparatorFor =
  (sort: MemberSort) =>
  (a: OrganizationMember, b: OrganizationMember): number => {
    if (sort === 'joined') return joinedDescending(a, b);
    if (sort === 'joined-oldest') return -joinedDescending(a, b);
    if (sort === 'permission' && a.permission !== b.permission) {
      return a.permission === 'admin' ? -1 : 1;
    }
    return statusRank(a) - statusRank(b) || joinedDescending(a, b);
  };

@Component({
  selector: 'app-organization-members',
  imports: [RouterOutlet, DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './organization-members.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class OrganizationMembersComponent implements OnInit {
  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private routeQuery = toSignal(this.route.queryParamMap, {
    initialValue: this.route.snapshot.queryParamMap,
  });

  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  protected overlays = inject(OverlayService);

  private organizationDataService = inject(OrganizationDataService);

  private notificationService = inject(NotificationService);

  private memberClient = inject(MEMBER);

  private inviteClient = inject(INVITE);

  private idempotency = createIdempotencyRef();

  private authnService = inject(AuthnApiService);

  // Loading and error state
  isLoading = signal(true);

  error = signal<string | null>(null);

  isSubmitting = signal(false);

  // Delete modal state
  showDeleteModal = signal(false);

  deletingMember = signal<OrganizationMember | null>(null);

  /** The dialog sits in the DOM before anyone is picked, so this has to read as
   *  a sentence with the blank still open. It said "Remove undefined". */
  removeMemberTitle = computed(() => {
    const member = this.deletingMember();
    const name = member?.name || member?.email;
    return name
      ? `Remove ${name} from this organization?`
      : 'Remove this member from this organization?';
  });

  /** A failed action, reported over the list instead of in place of it: the
   *  list is still valid, only the action was not. */
  actionError = signal<{
    title: string;
    message: string;
    attempts: number;
    retry: () => Promise<void>;
  } | null>(null);

  /** The attempt count is what makes a second failure legible: without it the
   *  dialog comes back identical and the retry looks like it never ran. */
  actionErrorText = computed(() => {
    const failed = this.actionError();
    if (!failed) return null;
    return failed.attempts > 1
      ? `Tried ${failed.attempts} times. ${failed.message}`
      : failed.message;
  });

  // All members loaded from API (includes pending, active, declined and revoked)
  allMembers = signal<OrganizationMember[]>([]);

  activeMembers = computed(() => this.allMembers().filter((m) => m.status === 'accepted'));

  pendingInvitations = computed(() => this.allMembers().filter((m) => m.status === 'pending'));

  /** Everyone with access or on their way to it, invitations first: an
   *  invitation is the row you still have to do something about. */
  allAccess = computed(() => [...this.pendingInvitations(), ...this.activeMembers()]);

  memberQuery = signal('');

  /** Everything the query leaves standing; the filter and the counts both work
   *  on this, so the tabs describe what you would actually see. */
  matching = computed(() => filterByQuery(this.allAccess(), this.memberQuery()));

  matchingInvitations = computed(() =>
    filterByQuery(this.pendingInvitations(), this.memberQuery()),
  );

  matchingMembers = computed(() => filterByQuery(this.activeMembers(), this.memberQuery()));

  emptyStateText = computed(() => {
    switch (this.memberFilter()) {
      case 'invitations':
        return 'No pending invitations';
      case 'members':
        return 'No members found';
      default:
        return 'Nobody has access yet';
    }
  });

  memberFilter = signal<'all' | 'invitations' | 'members'>('all');

  memberSort = signal<MemberSort>('status');

  sortLabel = computed(() => SORT_LABELS[this.memberSort()]);

  filterLabel = computed(() => FILTER_LABELS[this.memberFilter()]);

  visibleMembers = computed(() => {
    const base = this.filtered();
    return [...base].sort(comparatorFor(this.memberSort()));
  });

  private filtered = computed(() => {
    switch (this.memberFilter()) {
      case 'invitations':
        return this.matchingInvitations();
      case 'members':
        return this.matchingMembers();
      default:
        return this.matching();
    }
  });

  constructor() {
    this.titleService.setTitle('Organization members');
  }

  ngOnInit() {
    this.loadMembers();
  }

  /** What membersChanged stood at when this list was built. An effect runs once
   *  on creation, and ngOnInit already loads, so only a later bump is news. */
  private seenMembersChanged = this.organizationDataService.membersChanged();

  /** The sheet that invites one lives in the shell and may well be standing over
   *  this very list, so the list hears about the invitation from there. */
  private readonly reloadOnInvite = effect(() => {
    const changed = this.organizationDataService.membersChanged();
    if (changed === this.seenMembersChanged) return;
    this.seenMembersChanged = changed;
    this.loadMembers();
  });

  async loadMembers() {
    this.isLoading.set(true);
    this.error.set(null);

    try {
      const currentUser = await firstValueFrom(this.authnService.currentUser$);
      const response = await firstValueFrom(this.memberClient.listMembers({}));

      const members: OrganizationMember[] = response.members.map((member) => ({
        id: member.id,
        name: member.name,
        email: member.email,
        externalRef: member.externalRef,
        permission: member.permission,
        status: member.status,
        isCurrentUser: currentUser?.id === member.userId,
        created: member.created ? timestampDate(member.created) : undefined,
      }));

      this.allMembers.set(members);
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load members: ${err.message}` : 'Failed to load members',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  async cancelInvitation(id: string) {
    const invitation = this.pendingInvitations().find((m) => m.id === id);
    const invitee = invitation?.email || invitation?.name;

    try {
      await firstValueFrom(this.memberClient.deleteMember({ id }));
      this.notificationService.success(
        invitee ? `Invitation for '${invitee}' cancelled` : 'Invitation cancelled',
      );
      await this.loadMembers();
    } catch (err) {
      this.actionError.set({
        title: 'Invitation not cancelled',
        message: err instanceof Error ? err.message : 'The request failed.',
        attempts: 1,
        retry: () => this.cancelInvitation(id),
      });
    }
  }

  openDeleteModal(member: OrganizationMember) {
    this.deletingMember.set(member);
    this.showDeleteModal.set(true);
  }

  async confirmDeleteMember() {
    const member = this.deletingMember();
    if (!member) return;

    try {
      await firstValueFrom(this.memberClient.deleteMember({ id: member.id }));
      this.showDeleteModal.set(false);
      this.deletingMember.set(null);
      this.notificationService.success(`'${member.name}' removed from the organization`);
      await this.loadMembers();
    } catch (err) {
      this.showDeleteModal.set(false);
      this.actionError.set({
        title: 'Member not removed',
        message: err instanceof Error ? err.message : 'The request failed.',
        attempts: 1,
        retry: () => this.confirmDeleteMember(),
      });
    }
  }

  /** Permission is the only thing that can be edited, and it has two values, so
   *  the menu flips it directly instead of opening a sheet to hold one radio
   *  group. Reversible in one click, so no confirmation. */
  async setPermission(member: OrganizationMember) {
    const permission = member.permission === 'admin' ? 'viewer' : 'admin';
    // An invitation is a member record without a name yet, and nobody is
    // anything until they accept: "will be admin", not "is admin".
    const who = member.status === 'pending' ? member.email || member.name : member.name;
    const becomes = member.status === 'pending' ? 'is invited as' : 'is now';
    const stays = member.status === 'pending' ? 'is still invited as' : 'is still';
    this.isSubmitting.set(true);

    try {
      await firstValueFrom(this.memberClient.updateMemberPermission({ id: member.id, permission }));
      this.notificationService.success(`${who} ${becomes} ${permission}`);
      await this.loadMembers();
    } catch (err) {
      this.actionError.set({
        title: `${who} ${stays} ${member.permission}`,
        message: err instanceof Error ? err.message : 'The request failed.',
        attempts: 1,
        retry: () => this.setPermission(member),
      });
    } finally {
      this.isSubmitting.set(false);
    }
  }

  retrying = signal(false);

  /**
   * The button reports the retry, and for long enough to be seen: a failure that
   * comes back instantly would otherwise leave the dialog looking untouched, as
   * if the click had missed. The wait is a floor, not a delay on top — a request
   * slower than this is not held back.
   */
  async retryAction() {
    const failed = this.actionError();
    if (!failed) return;

    this.retrying.set(true);
    try {
      await Promise.all([
        failed.retry(),
        new Promise((resolve) => {
          setTimeout(resolve, MIN_RETRY_FEEDBACK_MS);
        }),
      ]);
    } finally {
      this.retrying.set(false);
    }

    // The action writes a fresh object when it fails again, so an untouched one
    // means it went through this time.
    const next = this.actionError();
    if (next === failed) {
      this.actionError.set(null);
      return;
    }
    if (next) this.actionError.set({ ...next, attempts: failed.attempts + 1 });
  }

  /** Routes client-side while leaving the control a real link, so middle-click
   *  and "open in new tab" keep working. */
  openPermissions(event: Event): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.pageNav.goTo('/members/permissions');
  }

  permissionLabel = permissionLabel;

  formatTimeAgo = formatTimeAgo;

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
