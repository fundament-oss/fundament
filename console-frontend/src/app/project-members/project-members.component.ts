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
import { toSignal } from '@angular/core/rxjs-interop';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import PageNavService from '../page-nav.service';
import { PROJECT, MEMBER, NAMESPACE } from '../../connect/tokens';
import { OrganizationDataService } from '../organization-data.service';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { NotificationService } from '../notification.service';
import { formatTimeAgo } from '../utils/date-format';
import { mockBindingsFor, setMockBindings, ALL_ROLES } from '../utils/mock-role-bindings';
import {
  NEW_NAMESPACE,
  ALL_NAMESPACES,
  NAMESPACE_NAME,
  namespaceLabel,
  toggleNamespace,
} from '../utils/namespace-grants';
import type { RoleBinding } from '../utils/mock-role-bindings';
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
    SheetSyncDirective,
    RouterLink,
    AutofocusDirective,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './project-members.component.html',
})
export default class ProjectMembersComponent implements OnInit {
  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private routeQuery = toSignal(this.route.queryParamMap, {
    initialValue: this.route.snapshot.queryParamMap,
  });

  /** Opened from the URL as well as from the button, so a control elsewhere in
   *  the app can send you straight here. Closing takes the parameter off again,
   *  or the back button would land on something you just finished. */
  private readonly openFromUrl = effect(() => {
    if (this.routeQuery().get('add') !== null && !this.showAddMemberModal()) this.openAddMemberModal();
  });

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

  showAddMemberModal = signal<boolean>(false);

  isAddingMember = signal<boolean>(false);

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

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    permission: ['viewer', Validators.required],
  });

  ProjectMemberRole = ProjectMemberRole;

  /** Namespace access, handed out while adding: giving someone a place to work
   *  is the same intention as putting them in the project, so it does not need a
   *  second trip through the member sheet. Empty is fine, nothing is granted. */
  draftNamespaces = signal<string[]>([]);

  draftRoles = signal<string[]>([]);

  newNamespaceName = signal('');

  /** Set by pressing the button: it is never disabled, so the fields are what
   *  say what is missing. */
  private addSubmitted = signal(false);

  newNamespaceInvalid = computed(() => {
    if (!this.draftNamespaces().includes(NEW_NAMESPACE)) return false;
    const name = this.newNamespaceName();
    if (name.trim() === '') return this.addSubmitted();
    return !NAMESPACE_NAME.test(name);
  });

  newNamespaceError = computed(() =>
    this.newNamespaceName().trim() === ''
      ? 'A namespace name is required.'
      : 'Use lowercase letters, numbers and hyphens, starting with a letter.',
  );

  allRoles = ALL_ROLES;

  NEW_NAMESPACE = NEW_NAMESPACE;

  ALL_NAMESPACES = ALL_NAMESPACES;

  namespaceLabel = namespaceLabel;

  toggleDraftNamespace(namespace: string) {
    this.draftNamespaces.update((current) => toggleNamespace(current, namespace));
  }

  toggleDraftRole(role: string) {
    this.draftRoles.update((roles) =>
      roles.includes(role) ? roles.filter((current) => current !== role) : [...roles, role],
    );
  }

  /** Falls back to the neutral word while the organization data is still
   *  loading, so the sheet title never reads "Add member to undefined". */
  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  selectedUserName = computed(() => {
    const id = this.selectedUserId();
    return this.availableUsers().find((user) => user.id === id)?.name ?? '';
  });

  private selectedUserId = signal('');

  /** Set by pressing Add member without a choice: the button stays enabled, so
   *  the field is what says what is missing. */
  private userFieldTouched = signal(false);

  userFieldInvalid = computed(() => this.userFieldTouched() && !this.selectedUserId());

  permissionOptions = [
    {
      value: 'viewer',
      label: 'Project viewer',
      description: 'Can see the project, its members and its namespaces.',
    },
    {
      value: 'admin',
      label: 'Project admin',
      description: 'Can also edit the project, manage its members and create namespaces.',
    },
  ];

  constructor() {
    this.titleService.setTitle('Project members');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    this.loadMembers();
    this.loadNamespaces();
  }

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

      const pending = this.pendingGrant;
      const granted = pending && views.find((view) => view.member.userId === pending.userId);
      if (pending && granted) {
        setMockBindings(granted.member.id, pending.bindings);
        this.pendingGrant = null;
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

  closeAddMemberModal() {
    this.showAddMemberModal.set(false);
    if (this.route.snapshot.queryParamMap.get('add') !== null) {
      this.router.navigate([], {
        relativeTo: this.route,
        queryParams: { add: null },
        queryParamsHandling: 'merge',
        replaceUrl: true,
      });
    }
  }

  openAddMemberModal() {
    this.selectedUserId.set('');
    this.userFieldTouched.set(false);
    this.addSubmitted.set(false);
    this.draftNamespaces.set([]);
    this.draftRoles.set([]);
    this.newNamespaceName.set('');
    this.memberForm.reset({ userId: '', permission: 'viewer' });
    this.showAddMemberModal.set(true);
  }

  onPermissionChange(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.memberForm.get('permission')?.setValue(value);
  }

  /** The row is the target, the radio is the control. */
  selectPermission(value: string) {
    this.memberForm.get('permission')?.setValue(value);
  }

  onUserChange(event: Event) {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.selectedUserId.set(value);
    this.userFieldTouched.set(false);
    this.memberForm.get('userId')?.setValue(value);
    this.memberForm.get('userId')?.markAsDirty();
  }

  async saveMember(event?: Event) {
    event?.preventDefault();

    this.addSubmitted.set(true);
    if (!this.selectedUserId()) this.userFieldTouched.set(true);
    // Both fields report at once: fixing one and being sent back for the other
    // reads as a second failure.
    if (!this.selectedUserId() || this.newNamespaceInvalid()) return;

    this.isAddingMember.set(true);
    const role = stringToRole(this.memberForm.value.permission!);
    const userId = this.memberForm.value.userId!;

    try {
      await withIdempotency(
        (opts) =>
          this.projectClient.addProjectMember({ projectId: this.projectId(), userId, role }, opts),
        { signal: this.idempotency.reset() },
      );
      this.showAddMemberModal.set(false);
      await this.grantDraftAccess(userId);
      await this.loadMembers();
    } catch (err) {
      this.showAddMemberModal.set(false);
      this.actionError.set({
        title: 'Member not added',
        message: err instanceof Error ? err.message : 'The request failed.',
        attempts: 1,
        retry: () => this.saveMember(),
      });
    } finally {
      this.isAddingMember.set(false);
    }
  }

  /**
   * The access picked in the add form, handed out once the member exists. The
   * member is already in the project by now, so a namespace that cannot be
   * created is reported and the rest still goes through.
   */
  private async grantDraftAccess(userId: string) {
    let namespaces = [...this.draftNamespaces()];
    if (namespaces.length === 0) return;

    if (namespaces.includes(NEW_NAMESPACE)) {
      const name = this.newNamespaceName();
      try {
        await firstValueFrom(
          this.namespaceClient.createNamespace({ projectId: this.projectId(), name }),
        );
        this.projectNamespaces.update((names) => [...names, name]);
        this.notificationService.success(`Namespace '${name}' created`);
        // Under an all-namespaces grant the new one is covered already.
        namespaces = namespaces.includes(ALL_NAMESPACES)
          ? namespaces.filter((entry) => entry !== NEW_NAMESPACE)
          : namespaces.map((entry) => (entry === NEW_NAMESPACE ? name : entry));
      } catch (err) {
        this.notificationService.error(
          err instanceof Error ? `Namespace not created: ${err.message}` : 'Namespace not created',
        );
        namespaces = namespaces.filter((entry) => entry !== NEW_NAMESPACE);
      }
    }

    if (namespaces.length === 0) return;

    // The member id only exists once the list comes back, so the roles are
    // parked against the user and attached in loadMembers().
    const roles = ALL_ROLES.filter((role) => this.draftRoles().includes(role));
    this.pendingGrant = { userId, bindings: namespaces.map((namespace) => ({ namespace, roles })) };
  }

  /** TEMPORARY, dev only: roles have no API, so a fresh grant waits here for the
   *  member id the list assigns. Delete with the mock bindings. */
  private pendingGrant: { userId: string; bindings: RoleBinding[] } | null = null;

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
    this.router.navigateByUrl(`/projects/${this.projectId()}/members/${memberId}`);
  }

  /** Routes client-side while leaving the control a real link. */
  openPermissions(event: Event): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.router.navigateByUrl(`/projects/${this.projectId()}/members/permissions`);
  }

  roleToString = roleToString;

  roleLabel = roleLabel;

  formatMemberDate = formatMemberDate;
}
