import {
  Component,
  inject,
  signal,
  OnInit,
  effect,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  isDevMode,
} from '@angular/core';
import { ActivatedRoute, Router, RouterLink, RouterOutlet } from '@angular/router';
import { ReactiveFormsModule, FormBuilder } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import PageNavService from '../page-nav.service';
import { OverlayService } from '../overlay.service';
import { PROJECT, MEMBER, NAMESPACE } from '../../connect/tokens';
import { OrganizationDataService } from '../organization-data.service';
import DialogSyncDirective from '../dialog-sync.directive';
import { NotificationService } from '../notification.service';
import { formatTimeAgo } from '../utils/date-format';
import { mockBindingsFor, setMockBindings } from '../utils/mock-role-bindings';
import { ALL_NAMESPACES } from '../utils/namespace-grants';
import type { ProjectMember } from '../../generated/v1/project_pb';
import { ProjectMemberRole } from '../../generated/v1/project_pb';

interface ProjectMemberView {
  member: ProjectMember;
  source: 'org' | 'project';
  orgPermission: ProjectMemberRole | null;
}

const roleToString = (role: ProjectMemberRole): string => {
  switch (role) {
    case ProjectMemberRole.ADMIN:
      return 'admin';
    case ProjectMemberRole.VIEWER:
      return 'viewer';
    default:
      return 'unknown';
  }
};

const stringToRole = (s: string): ProjectMemberRole => {
  switch (s) {
    case 'admin':
      return ProjectMemberRole.ADMIN;
    default:
      return ProjectMemberRole.VIEWER;
  }
};

const formatMemberDate = (member: ProjectMember): string =>
  formatTimeAgo(member.created ? timestampDate(member.created) : undefined);

/** 'admin' reads as a value, 'Admin' as a label. The tag shows the label. */
const roleLabel = (role: ProjectMemberRole): string => {
  const value = roleToString(role);
  return value ? value[0].toUpperCase() + value.slice(1) : value;
};

/** TEMPORARY, dev only. Delete together with the branch in loadMembers(). */
const SAMPLE_USERS = [
  { id: 'sample-user-1', name: 'Nadia el Amrani' },
  { id: 'sample-user-2', name: 'Li Wei' },
  { id: 'sample-user-3', name: 'Jean-Pierre van der Meer-Bakhuizen' },
  { id: 'sample-user-4', name: 'Tom Jansen' },
  { id: 'sample-user-5', name: 'Fatima Ouali' },
];

/** Which namespaces this member has a role in. One of them is worth naming: the
 *  name says more than the number does. Beyond that it is a count, and the sheet
 *  shows which. */
const namespaceSummary = (memberId: string, namespaces: string[]): string => {
  const bindings = mockBindingsFor(memberId, namespaces);
  if (bindings.some((binding) => binding.namespace === ALL_NAMESPACES)) return 'All namespaces';
  if (bindings.length === 1) return bindings[0].namespace;
  return `${bindings.length} namespaces`;
};

/** The roles under that summary, but only while it names one thing: across
 *  several namespaces the roles differ per namespace and the sheet shows them. */
const namespaceRoles = (memberId: string, namespaces: string[]): string | null => {
  const bindings = mockBindingsFor(memberId, namespaces);
  return bindings.length === 1 ? bindings[0].roles.join(', ') : null;
};

/** A locked row: the project role equals the organization role and the member is
 *  an admin there, so this page cannot change it. */
const isLocked = (view: ProjectMemberView): boolean =>
  view.source === 'org' && view.member.role === ProjectMemberRole.ADMIN;

/** How long a retry shows as running before its result lands. */
const MIN_RETRY_FEEDBACK_MS = 2000;

const memberTime = (member: ProjectMember): number =>
  member.created ? timestampDate(member.created).getTime() : 0;

const addedDescending = (a: ProjectMemberView, b: ProjectMemberView): number =>
  memberTime(b.member) - memberTime(a.member);

const byPermissionThenAdded = (a: ProjectMemberView, b: ProjectMemberView): number => {
  if (a.member.role !== b.member.role) {
    return a.member.role === ProjectMemberRole.ADMIN ? -1 : 1;
  }
  return addedDescending(a, b);
};

@Component({
  selector: 'app-project-members',
  imports: [
    RouterOutlet,
    ReactiveFormsModule,
    DialogSyncDirective,
    RouterLink,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './project-members.component.html',
})
export default class ProjectMembersComponent implements OnInit {
  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  protected overlays = inject(OverlayService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private organizationData = inject(OrganizationDataService);

  private fb = inject(FormBuilder);

  private projectClient = inject(PROJECT);

  private idempotency = createIdempotencyRef();

  private memberClient = inject(MEMBER);

  private namespaceClient = inject(NAMESPACE);

  private notificationService = inject(NotificationService);

  projectId = signal<string>('');

  memberViews = signal<ProjectMemberView[]>([]);

  availableUsers = signal<{ id: string; name: string }[]>([]);

  isLoading = signal(true);

  error = signal<string | null>(null);

  /** A fixed order, now that there is nothing to sort by: admins first, and the
   *  most recently added first within a permission. */
  visibleMembers = computed(() => [...this.memberViews()].sort(byPermissionThenAdded));

  /** A failed action, reported over the list instead of in place of it. */
  actionError = signal<{
    title: string;
    message: string;
    attempts: number;
    retry: () => Promise<void>;
  } | null>(null);

  actionErrorText = computed(() => {
    const failed = this.actionError();
    if (!failed) return null;
    return failed.attempts > 1
      ? `Tried ${failed.attempts} times. ${failed.message}`
      : failed.message;
  });

  retrying = signal(false);

  isLocked = isLocked;

  ProjectMemberRole = ProjectMemberRole;

  /** Falls back to the neutral word while the organization data is still
   *  loading, so the sheet title never reads "Add member to undefined". */
  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  constructor() {
    this.titleService.setTitle('Project members');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    this.loadMembers();
    this.loadNamespaces();
  }

  /** The sheet that adds one lives in the shell and may well be standing over
   *  this very list, so the list hears about a new member from there. */
  private readonly reloadOnAdd = effect(() => {
    this.organizationData.membersChanged();
    if (this.projectId()) this.loadMembers();
  });

  /** Only for the namespace count on a row; the sheet has the detail. */
  async loadNamespaces() {
    try {
      const response = await firstValueFrom(
        this.namespaceClient.listProjectNamespaces({ projectId: this.projectId() }),
      );
      this.projectNamespaces.set(response.namespaces.map((namespace) => namespace.name));
    } catch {
      this.projectNamespaces.set([]);
    }
  }

  async loadMembers() {
    this.isLoading.set(true);
    this.error.set(null);

    try {
      const [projectResponse, orgResponse] = await Promise.all([
        firstValueFrom(this.projectClient.listProjectMembers({ projectId: this.projectId() })),
        firstValueFrom(this.memberClient.listMembers({})),
      ]);

      // Build a map of org member id → org role string
      const orgRoleByUserId = new Map<string, string>();
      orgResponse.members
        .filter((m) => m.externalRef)
        .forEach((m) => orgRoleByUserId.set(m.userId, m.permission));

      // Enrich project members with source info
      const views: ProjectMemberView[] = projectResponse.members.map((member) => {
        const orgRole = orgRoleByUserId.get(member.userId);
        if (orgRole !== undefined) {
          const orgRoleEnum = stringToRole(orgRole);
          const sameRole = member.role === orgRoleEnum;
          return {
            member,
            source: sameRole ? 'org' : 'project',
            orgPermission: orgRoleEnum,
          };
        }
        return { member, source: 'project', orgPermission: null };
      });
      this.memberViews.set(views);

      const pending = this.organizationData.pendingProjectGrant();
      const granted = pending && views.find((view) => view.member.userId === pending.userId);
      if (pending && granted) {
        setMockBindings(granted.member.id, pending.bindings);
        this.organizationData.pendingProjectGrant.set(null);
      }

      // Available users for "add member": org members not yet in project
      const projectUserIds = new Set(projectResponse.members.map((m) => m.userId));
      const available = orgResponse.members
        .filter((m) => m.externalRef && !projectUserIds.has(m.userId))
        .map((m) => ({ id: m.userId, name: m.name }));

      // TEMPORARY, dev only: this environment has nobody left to add, so the
      // add form can never be seen. Delete before merging.
      this.availableUsers.set(available.length === 0 && isDevMode() ? SAMPLE_USERS : available);
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load members: ${err.message}` : 'Failed to load members',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  /**
   * The button reports the retry, and for long enough to be seen: a failure that
   * comes back instantly would otherwise leave the dialog looking untouched.
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

    const next = this.actionError();
    if (next === failed) {
      this.actionError.set(null);
      return;
    }
    if (next) this.actionError.set({ ...next, attempts: failed.attempts + 1 });
  }

  projectNamespaces = signal<string[]>([]);

  namespaceSummary = (memberId: string): string =>
    namespaceSummary(memberId, this.projectNamespaces());

  namespaceRoles = (memberId: string): string | null =>
    namespaceRoles(memberId, this.projectNamespaces());

  /** Routes client-side while leaving the row a real link, so middle-click and
   *  "open in new tab" keep working. */
  openMember(event: Event, memberId: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.pageNav.goTo(`/projects/${this.projectId()}/members/${memberId}`);
  }

  /** Routes client-side while leaving the control a real link. */
  openPermissions(event: Event): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.pageNav.goTo(`/projects/${this.projectId()}/members/permissions`);
  }

  roleToString = roleToString;

  roleLabel = roleLabel;

  formatMemberDate = formatMemberDate;
}
