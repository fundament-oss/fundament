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
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { PROJECT, MEMBER, NAMESPACE } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatTimeAgo } from '../utils/date-format';
import { mockBindingsFor, setMockBindings, ALL_ROLES } from '../utils/mock-role-bindings';
import type { RoleBinding } from '../utils/mock-role-bindings';
import {
  NEW_NAMESPACE,
  ALL_NAMESPACES,
  NAMESPACE_NAME,
  namespaceLabel,
  toggleNamespace,
} from '../utils/namespace-grants';
import type { ProjectMember } from '../../generated/v1/project_pb';
import { ProjectMemberRole } from '../../generated/v1/project_pb';
import PageNavService from '../page-nav.service';

interface ProjectMemberView {
  member: ProjectMember;
  /** 'org' when the project role equals the organization role. */
  source: 'org' | 'project';
}

const roleLabel = (role: ProjectMemberRole): string =>
  role === ProjectMemberRole.ADMIN ? 'Admin' : 'Viewer';

/** Locked: an organization admin is admin here too, and this page cannot change
 *  what the organization decided. */
const isLocked = (view: ProjectMemberView): boolean =>
  view.source === 'org' && view.member.role === ProjectMemberRole.ADMIN;

const formatMemberDate = (member: ProjectMember): string =>
  member.created ? formatTimeAgo(timestampDate(member.created)) : 'unknown';

/**
 * Everything one member has in this project, as a route of its own so a row can
 * be a real link: shareable, openable in a new tab, closed by the back button.
 */
@Component({
  selector: 'app-project-member-sheet',
  imports: [DialogSyncDirective, SheetSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './project-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectMemberSheetComponent implements OnInit {
  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private memberClient = inject(MEMBER);

  private namespaceClient = inject(NAMESPACE);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  projectId = signal('');

  memberId = signal('');

  member = signal<ProjectMemberView | null>(null);

  /** The dialog is in the DOM before the member is loaded, so this has to read
   *  as a sentence with the blank still open. It said "Remove undefined". */
  removeMemberTitle = computed(() => {
    const name = this.member()?.member?.userName;
    return name ? `Remove ${name} from this project?` : 'Remove this member from this project?';
  });

  isLoading = signal(true);

  saving = signal(false);

  showRemoveModal = signal(false);

  /** The namespace waiting for a confirmed removal, or '' when none is. */
  pendingRemoveNamespace = signal('');

  /** Local for now: roles have no API, so an edit lives in the sheet only. */
  bindings = signal<RoleBinding[]>([]);

  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || this.projectId();
  });

  allRoles = ALL_ROLES;

  /** Which step the sheet is on. The list is the way back from either editor. */
  step = signal<'overview' | 'access'>('overview');

  /** The namespace being edited, or '' while adding a new one. */
  editingNamespace = signal('');

  draftRoles = signal<string[]>([]);

  /** The add step grants the same roles to several namespaces at once; the edit
   *  step works on the one namespace it was opened with. */
  draftNamespaces = signal<string[]>([]);

  /** Sentinel for the "New namespace…" option in the picker. */
  NEW_NAMESPACE = NEW_NAMESPACE;

  ALL_NAMESPACES = ALL_NAMESPACES;

  namespaceLabel = namespaceLabel;

  newNamespaceName = signal('');

  creatingNamespace = signal(false);

  /** Set by pressing the submit button: the button is never disabled, so the
   *  fields are what say what is missing. */
  accessSubmitted = signal(false);

  newNamespaceInvalid = computed(() => {
    if (!this.effectiveNamespaces().includes(NEW_NAMESPACE)) return false;
    const name = this.newNamespaceName();
    if (name.trim() === '') return this.accessSubmitted();
    return !NAMESPACE_NAME.test(name);
  });

  newNamespaceError = computed(() =>
    this.newNamespaceName().trim() === ''
      ? 'A namespace name is required.'
      : 'Use lowercase letters, numbers and hyphens, starting with a letter.',
  );

  private canSaveAccess = computed(() => {
    const namespaces = this.effectiveNamespaces();
    if (namespaces.length === 0) return false;
    if (!namespaces.includes(NEW_NAMESPACE)) return true;
    return NAMESPACE_NAME.test(this.newNamespaceName());
  });

  /** An all-namespaces grant covers everything, now and later, so there is
   *  nothing left to hand out per namespace. */
  hasAllGrant = computed(() =>
    this.bindings().some((binding) => binding.namespace === ALL_NAMESPACES),
  );

  /** With the all-grant in place the only choice left is a new namespace: no
   *  list of one, the name field alone says it. Having every existing namespace
   *  separately is not the same thing, "All" still adds the future ones. */
  impliedNew = computed(() => this.hasAllGrant());

  /** Every existing namespace granted one by one. Worth saying, because the
   *  list that follows then holds nothing they do not already have, and the
   *  difference is only in what comes later. */
  coversEveryNamespace = computed(
    () =>
      !this.hasAllGrant() &&
      this.projectNamespaces().length > 0 &&
      this.availableNamespaces().length === 0,
  );

  /** What the step will actually grant to. */
  private effectiveNamespaces = computed(() =>
    this.impliedNew() ? [NEW_NAMESPACE] : this.draftNamespaces(),
  );

  /** Nothing ticked at all, once the button has been pressed. */
  namespaceSelectionInvalid = computed(
    () => this.accessSubmitted() && this.effectiveNamespaces().length === 0,
  );

  /** Namespaces this member has no access to yet, for the add step. */
  availableNamespaces = computed(() => {
    if (this.hasAllGrant()) return [];
    const taken = new Set(this.bindings().map((binding) => binding.namespace));
    return this.projectNamespaces().filter((namespace) => !taken.has(namespace));
  });

  projectNamespaces = signal<string[]>([]);

  ProjectMemberRole = ProjectMemberRole;

  roleLabel = roleLabel;

  isLocked = isLocked;

  formatMemberDate = formatMemberDate;

  ngOnInit() {
    // The parameters live on the parent route: this sheet is its child.
    this.projectId.set(this.route.snapshot.parent?.params['id'] ?? '');
    this.memberId.set(this.route.snapshot.params['memberId'] ?? '');
    this.load();
    this.loadNamespaces();
  }

  async load() {
    this.isLoading.set(true);
    try {
      const [projectResponse, orgResponse] = await Promise.all([
        firstValueFrom(this.projectClient.listProjectMembers({ projectId: this.projectId() })),
        firstValueFrom(this.memberClient.listMembers({})),
      ]);

      const orgRoleByUserId = new Map(
        orgResponse.members.filter((m) => m.externalRef).map((m) => [m.userId, m.permission]),
      );
      const found = projectResponse.members.find((m) => m.id === this.memberId());
      this.member.set(
        found
          ? {
              member: found,
              source:
                roleLabel(found.role).toLowerCase() === orgRoleByUserId.get(found.userId)
                  ? 'org'
                  : 'project',
            }
          : null,
      );
    } catch {
      this.member.set(null);
    } finally {
      this.isLoading.set(false);
    }
  }

  async loadNamespaces() {
    try {
      const response = await firstValueFrom(
        this.namespaceClient.listProjectNamespaces({ projectId: this.projectId() }),
      );
      const names = response.namespaces.map((namespace) => namespace.name);
      this.projectNamespaces.set(names);
      this.bindings.set(mockBindingsFor(this.memberId(), names));
    } catch {
      this.projectNamespaces.set([]);
    }
  }

  /** Opens the access step for an existing namespace, or for a new one. */
  editAccess(namespace: string) {
    this.accessSubmitted.set(false);
    this.editingNamespace.set(namespace);
    this.draftNamespaces.set([namespace]);
    this.draftRoles.set(
      this.bindings().find((binding) => binding.namespace === namespace)?.roles ?? [],
    );
    this.step.set('access');
  }

  toggleDraftNamespace(namespace: string) {
    this.draftNamespaces.update((current) => toggleNamespace(current, namespace));
  }

  addAccess() {
    this.editingNamespace.set('');
    this.accessSubmitted.set(false);
    this.newNamespaceName.set('');
    this.draftNamespaces.set([]);
    this.draftRoles.set([]);
    this.step.set('access');
  }

  toggleDraftRole(role: string) {
    this.draftRoles.update((roles) =>
      roles.includes(role) ? roles.filter((current) => current !== role) : [...roles, role],
    );
  }

  async saveAccess() {
    this.accessSubmitted.set(true);
    if (!this.canSaveAccess()) return;

    let namespaces = [...this.effectiveNamespaces()];

    // The namespace is real even though the roles are not yet: create it first,
    // and keep the step open when that fails so nothing is silently lost.
    if (namespaces.includes(NEW_NAMESPACE)) {
      const name = this.newNamespaceName();
      this.creatingNamespace.set(true);
      try {
        await firstValueFrom(
          this.namespaceClient.createNamespace({ projectId: this.projectId(), name }),
        );
        this.projectNamespaces.update((names) => [...names, name]);
        this.notificationService.success(`Namespace '${name}' created`);
        // Under an all-namespaces grant the new one is covered already, so it
        // does not need a grant of its own.
        namespaces = namespaces.includes(ALL_NAMESPACES)
          ? namespaces.filter((entry) => entry !== NEW_NAMESPACE)
          : namespaces.map((entry) => (entry === NEW_NAMESPACE ? name : entry));
      } catch (err) {
        this.notificationService.error(
          err instanceof Error ? `Namespace not created: ${err.message}` : 'Namespace not created',
        );
        return;
      } finally {
        this.creatingNamespace.set(false);
      }
    }

    // Ordered like ALL_ROLES so the summary reads the same way every time.
    const roles = ALL_ROLES.filter((role) => this.draftRoles().includes(role));
    this.bindings.update((bindings) => {
      // An all-namespaces grant subsumes the per-namespace ones, so they go.
      const kept = namespaces.includes(ALL_NAMESPACES)
        ? []
        : bindings.filter(
            (binding) =>
              !namespaces.includes(binding.namespace) && binding.namespace !== ALL_NAMESPACES,
          );
      if (roles.length === 0) return kept;
      return [...kept, ...namespaces.map((namespace) => ({ namespace, roles }))];
    });
    setMockBindings(this.memberId(), this.bindings());
    this.step.set('overview');
  }

  askRemoveAccess(namespace: string) {
    this.pendingRemoveNamespace.set(namespace);
  }

  confirmRemoveAccess() {
    const namespace = this.pendingRemoveNamespace();
    this.bindings.update((bindings) =>
      bindings.filter((binding) => binding.namespace !== namespace),
    );
    setMockBindings(this.memberId(), this.bindings());
    this.pendingRemoveNamespace.set('');
  }

  async togglePermission(view: ProjectMemberView) {
    const role =
      view.member.role === ProjectMemberRole.ADMIN
        ? ProjectMemberRole.VIEWER
        : ProjectMemberRole.ADMIN;

    this.saving.set(true);
    try {
      await firstValueFrom(
        this.projectClient.updateProjectMemberRole({ memberId: view.member.id, role }),
      );
      this.notificationService.success(
        `${view.member.userName} is now ${roleLabel(role).toLowerCase()}`,
      );
      await this.load();
    } catch (err) {
      this.notificationService.error(
        err instanceof Error ? `Permission not changed: ${err.message}` : 'Permission not changed',
      );
    } finally {
      this.saving.set(false);
    }
  }

  async confirmRemove() {
    const view = this.member();
    if (!view) return;

    try {
      await firstValueFrom(this.projectClient.removeProjectMember({ memberId: view.member.id }));
      this.notificationService.success(`${view.member.userName} removed from the project`);
      this.showRemoveModal.set(false);
      this.onClose();
    } catch (err) {
      this.showRemoveModal.set(false);
      this.notificationService.error(
        err instanceof Error ? `Member not removed: ${err.message}` : 'Member not removed',
      );
    }
  }

  openNamespaces(event: Event): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.pageNav.goTo(`/projects/${this.projectId()}/namespaces`);
  }

  openOrganizationMembers(event: Event): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.pageNav.goTo('/members');
  }

  /** Back to the list, which reloads and picks up whatever changed here. */
  onClose(): void {
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
