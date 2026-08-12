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
import { PROJECT, NAMESPACE } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatDate } from '../utils/date-format';
import { mockBindingsFor, setMockBindings, ALL_ROLES } from '../utils/mock-role-bindings';
import { ALL_NAMESPACES } from '../utils/namespace-grants';
import type { ProjectMember } from '../../generated/v1/project_pb';
import { ProjectMemberRole } from '../../generated/v1/project_pb';
import PageNavService from '../page-nav.service';

/** One member's standing in this namespace. */
interface NamespaceMember {
  member: ProjectMember;
  roles: string[];
  /** Granted for every namespace at once, so this one cannot be taken away here. */
  viaAll: boolean;
}

const roleLabel = (role: ProjectMemberRole): string =>
  role === ProjectMemberRole.ADMIN ? 'Admin' : 'Viewer';

/** What a member may do here, and where it comes from when it was not given for
 *  this namespace alone. */
const accessSummary = (entry: NamespaceMember): string => {
  const roles = entry.roles.join(', ') || 'No roles';
  return entry.viaAll ? `${roles}, through all namespaces` : roles;
};

/**
 * Everything about one namespace: who works in it and what they may do. The
 * member sheet says it the other way round, and the two write to the same place.
 */
@Component({
  selector: 'app-namespace-sheet',
  imports: [DialogSyncDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespace-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespaceSheetComponent implements OnInit {
  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private namespaceClient = inject(NAMESPACE);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  projectId = signal('');

  namespaceName = signal('');

  /** Set once the namespace is found; the delete call needs it. */
  namespaceId = signal('');

  created = signal('');

  isLoading = signal(true);

  /** Null while loading, and again when this namespace turns out not to exist. */
  found = signal<boolean | null>(null);

  members = signal<NamespaceMember[]>([]);

  projectNamespaces = signal<string[]>([]);

  showDeleteModal = signal(false);

  /** The member waiting for a confirmed removal, or null when none is. */
  pendingRemove = signal<NamespaceMember | null>(null);

  /** The dialog is in the DOM before anyone is picked, so this has to read as a
   *  sentence with the blank still open. It said "Remove undefined". */
  removeAccessTitle = computed(() => {
    const name = this.pendingRemove()?.member?.userName;
    const where = this.namespaceName();
    return name ? `Remove ${name} from ${where}?` : `Remove this member from ${where}?`;
  });

  step = signal<'overview' | 'roles'>('overview');

  /** The member the roles step is about, empty while adding someone new. */
  editingMemberId = signal('');

  /** Several at once: the same roles usually go to more than one person. */
  draftMemberIds = signal<string[]>([]);

  draftRoles = signal<string[]>([]);

  /** Set by pressing the button: it is never disabled, so the field is what says
   *  what is missing. */
  rolesSubmitted = signal(false);

  allRoles = ALL_ROLES;

  roleLabel = roleLabel;

  accessSummary = accessSummary;

  formatDate = formatDate;

  ProjectMemberRole = ProjectMemberRole;

  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  /** Project members with no role here, for the add step. */
  candidates = computed(() => {
    const taken = new Set(this.members().map((entry) => entry.member.id));
    return this.allMembers().filter((member) => !taken.has(member.id));
  });

  private allMembers = signal<ProjectMember[]>([]);

  editingMember = computed(() =>
    this.members().find((entry) => entry.member.id === this.editingMemberId()),
  );

  memberSelectionInvalid = computed(
    () => this.rolesSubmitted() && !this.editingMemberId() && this.draftMemberIds().length === 0,
  );

  onMembersChange(event: Event) {
    this.draftMemberIds.set((event as CustomEvent<{ values: string[] }>).detail.values);
  }

  ngOnInit() {
    // The project id lives on the parent route: this sheet is its child.
    this.projectId.set(this.route.snapshot.parent?.params['id'] ?? '');
    this.namespaceName.set(this.route.snapshot.params['name'] ?? '');
    this.load();
  }

  async load() {
    this.isLoading.set(true);
    try {
      const [namespaceResponse, memberResponse] = await Promise.all([
        firstValueFrom(this.namespaceClient.listProjectNamespaces({ projectId: this.projectId() })),
        firstValueFrom(this.projectClient.listProjectMembers({ projectId: this.projectId() })),
      ]);

      const names = namespaceResponse.namespaces.map((namespace) => namespace.name);
      this.projectNamespaces.set(names);

      const namespace = namespaceResponse.namespaces.find(
        (entry) => entry.name === this.namespaceName(),
      );
      this.found.set(!!namespace);
      this.namespaceId.set(namespace?.id ?? '');
      this.created.set(namespace?.created ? formatDate(namespace.created) : 'unknown');

      this.allMembers.set(memberResponse.members);
      this.readMembers();
    } catch {
      this.found.set(false);
    } finally {
      this.isLoading.set(false);
    }
  }

  /** Who has a role here, read from what every member holds. */
  private readMembers() {
    const name = this.namespaceName();
    const withAccess: NamespaceMember[] = [];
    this.allMembers().forEach((member) => {
      const bindings = mockBindingsFor(member.id, this.projectNamespaces());
      const direct = bindings.find((binding) => binding.namespace === name);
      if (direct) {
        withAccess.push({ member, roles: direct.roles, viaAll: false });
        return;
      }
      const all = bindings.find((binding) => binding.namespace === ALL_NAMESPACES);
      if (all) withAccess.push({ member, roles: all.roles, viaAll: true });
    });
    this.members.set(withAccess);
  }

  /** Writes this namespace back into what the member holds, leaving their other
   *  namespaces alone. No roles means no access. */
  private writeRoles(memberId: string, roles: string[]) {
    const name = this.namespaceName();
    const kept = mockBindingsFor(memberId, this.projectNamespaces()).filter(
      (binding) => binding.namespace !== name,
    );
    setMockBindings(memberId, roles.length === 0 ? kept : [...kept, { namespace: name, roles }]);
    this.readMembers();
  }

  addMember() {
    this.editingMemberId.set('');
    // One candidate is not a choice: it is filled in already, and the roles are
    // the only thing left to pick.
    const [only] = this.candidates();
    this.draftMemberIds.set(only && this.candidates().length === 1 ? [only.id] : []);
    this.draftRoles.set([]);
    this.rolesSubmitted.set(false);
    this.step.set('roles');
  }

  editRoles(entry: NamespaceMember) {
    this.editingMemberId.set(entry.member.id);
    this.draftMemberIds.set([entry.member.id]);
    this.draftRoles.set(entry.roles);
    this.rolesSubmitted.set(false);
    this.step.set('roles');
  }

  toggleDraftRole(role: string) {
    this.draftRoles.update((roles) =>
      roles.includes(role) ? roles.filter((current) => current !== role) : [...roles, role],
    );
  }

  saveRoles(event?: Event) {
    // See createNamespace: Enter picking an option in the token field is not a
    // submit, and the DS says so by marking the event handled.
    if (event?.defaultPrevented) return;

    this.rolesSubmitted.set(true);
    const memberIds = this.editingMemberId() ? [this.editingMemberId()] : this.draftMemberIds();
    if (memberIds.length === 0) return;

    // Ordered like ALL_ROLES so the summary reads the same way every time.
    const roles = ALL_ROLES.filter((role) => this.draftRoles().includes(role));
    memberIds.forEach((memberId) => this.writeRoles(memberId, roles));
    this.step.set('overview');
  }

  confirmRemove() {
    const entry = this.pendingRemove();
    if (!entry) return;
    this.writeRoles(entry.member.id, []);
    this.pendingRemove.set(null);
  }

  async deleteNamespace() {
    this.showDeleteModal.set(false);
    try {
      await firstValueFrom(
        this.namespaceClient.deleteNamespace({ namespaceId: this.namespaceId() }),
      );
      this.notificationService.success(`Namespace '${this.namespaceName()}' deleted`);
      await this.organizationData.loadOrganizationData();
      this.onClose();
    } catch (err) {
      this.notificationService.error(
        err instanceof Error ? `Namespace not deleted: ${err.message}` : 'Namespace not deleted',
      );
    }
  }

  /** Routes client-side while leaving the link a real link. */
  openMember(event: Event, memberId: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.pageNav.goTo(`/projects/${this.projectId()}/members/${memberId}`);
  }

  /** Back to the list this sheet was opened from. */
  onClose(): void {
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
