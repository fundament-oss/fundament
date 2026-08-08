import {
  Component,
  inject,
  OnInit,
  signal,
  computed,
  ChangeDetectionStrategy,
  isDevMode,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { Router, RouterOutlet } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { ConnectError, Code } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import PageNavService from '../page-nav.service';
import AuthnApiService from '../authn-api.service';
import { MEMBER, INVITE } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import focusFirstModalInput from '../modal-focus';
import { formatTimeAgo } from '../utils/date-format';

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

/** TEMPORARY, dev only. Delete together with the branch in loadMembers(). */
const sampleMembers = (currentUserId?: string): OrganizationMember[] => {
  const daysAgo = (days: number) => new Date(Date.now() - days * 86400000);
  return [
    {
      id: 'sample-1',
      name: '',
      email: 'nieuw.medewerker@example.com',
      externalRef: '',
      permission: 'viewer',
      status: 'pending',
      isCurrentUser: false,
      created: daysAgo(0.2),
    },
    {
      id: 'sample-2',
      name: '',
      email: 'a.de.jong@example.com',
      externalRef: '',
      permission: 'admin',
      status: 'pending',
      isCurrentUser: false,
      created: daysAgo(4),
    },
    {
      id: 'sample-3',
      name: 'Bart van de Biezen',
      email: 'bart@example.com',
      externalRef: 'sso|bart',
      permission: 'admin',
      status: 'accepted',
      isCurrentUser: true,
      created: daysAgo(420),
    },
    {
      id: 'sample-4',
      name: 'Nadia el Amrani',
      email: 'nadia.el.amrani@example.com',
      externalRef: 'sso|nadia',
      permission: 'viewer',
      status: 'accepted',
      isCurrentUser: false,
      created: daysAgo(3),
    },
    {
      id: 'sample-5',
      name: 'Jean-Pierre van der Meer-Bakhuizen',
      email: 'jp.vandermeer.bakhuizen@een-hele-lange-domeinnaam.example.com',
      externalRef: 'sso|jp',
      permission: 'admin',
      status: 'accepted',
      isCurrentUser: false,
      created: daysAgo(96),
    },
    {
      id: 'sample-6',
      name: 'Li Wei',
      email: 'li.wei@example.com',
      externalRef: 'sso|liwei',
      permission: 'viewer',
      status: 'accepted',
      isCurrentUser: false,
      created: daysAgo(1),
    },
  ].map((m) => ({ ...m, isCurrentUser: m.isCurrentUser && !!currentUserId }));
};

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

const SORT_LABELS: Record<MemberSort, string> = {
  status: 'Status',
  permission: 'Permission',
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
  imports: [
    RouterOutlet,
    DialogSyncDirective,
    SheetSyncDirective,
    AutofocusDirective,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './organization-members.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class OrganizationMembersComponent implements OnInit {
  private router = inject(Router);

  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  private toastService = inject(ToastService);

  private memberClient = inject(MEMBER);

  private inviteClient = inject(INVITE);

  private idempotency = createIdempotencyRef();

  private authnService = inject(AuthnApiService);

  // Loading and error state
  isLoading = signal(true);

  error = signal<string | null>(null);

  isSubmitting = signal(false);

  // Invite modal state
  isModalOpen = signal(false);

  inviteEmail = signal('');

  inviteEmailDirty = signal(false);

  invitePermission = signal('viewer');

  inviteError = signal<string | null>(null);

  // Delete modal state
  showDeleteModal = signal(false);

  deletingMember = signal<OrganizationMember | null>(null);

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

      // TEMPORARY, dev only: this environment returns members, but none with a
      // status the page shows, so the list layout can never be seen. Delete
      // before merging.
      const shown = members.filter((m) => m.status === 'accepted' || m.status === 'pending');
      this.allMembers.set(
        shown.length === 0 && isDevMode() ? sampleMembers(currentUser?.id) : members,
      );
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load members: ${err.message}` : 'Failed to load members',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  openModal() {
    this.inviteEmail.set('');
    this.inviteEmailDirty.set(false);
    this.invitePermission.set('viewer');
    this.inviteError.set(null);
    this.isModalOpen.set(true);
  }

  closeModal() {
    this.isModalOpen.set(false);
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
      this.closeModal();
      this.toastService.success(`'${email}' invited as ${permission}`);
      await this.loadMembers();
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

  async cancelInvitation(id: string) {
    const invitation = this.pendingInvitations().find((m) => m.id === id);
    const invitee = invitation?.email || invitation?.name;

    try {
      await firstValueFrom(this.memberClient.deleteMember({ id }));
      this.toastService.success(
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
      this.toastService.success(`'${member.name}' removed from the organization`);
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
    this.isSubmitting.set(true);

    try {
      await firstValueFrom(this.memberClient.updateMemberPermission({ id: member.id, permission }));
      this.toastService.success(`${member.name} is now ${permission}`);
      await this.loadMembers();
    } catch (err) {
      this.actionError.set({
        title: `${member.name} is still ${member.permission}`,
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
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.router.navigateByUrl('/organization/members/permissions');
  }

  permissionLabel = permissionLabel;

  formatTimeAgo = formatTimeAgo;

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
