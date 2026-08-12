import {
  Component,
  inject,
  input,
  signal,
  computed,
  effect,
  Output,
  EventEmitter,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  isDevMode,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT, MEMBER, NAMESPACE } from '../../connect/tokens';
import { ListProjectNamespacesRequestSchema } from '../../generated/v1/namespace_pb';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { ALL_ROLES } from '../utils/mock-role-bindings';
import {
  NEW_NAMESPACE,
  ALL_NAMESPACES,
  NAMESPACE_NAME,
  namespaceLabel,
  toggleNamespace,
} from '../utils/namespace-grants';
import { ProjectMemberRole } from '../../generated/v1/project_pb';
import '@nldd/design-system/combo-box';

const stringToRole = (value: string): ProjectMemberRole =>
  value === 'admin' ? ProjectMemberRole.ADMIN : ProjectMemberRole.VIEWER;

/** TEMPORARY, dev only: this environment has nobody left to add, so the add form
 *  could never be seen. Delete before merging. */
const SAMPLE_USERS = [
  { id: 'sample-user-1', name: 'Sanne Bakker' },
  { id: 'sample-user-2', name: 'Omar Aydin' },
];

/**
 * Adding someone to a project, from wherever you were. The shell owns this sheet
 * rather than the member list, so the toolbar can open it over any page; it
 * therefore loads the users it can offer and the project's namespaces itself.
 */
@Component({
  selector: 'app-add-project-member-sheet',
  imports: [ReactiveFormsModule, SheetSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './add-project-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class AddProjectMemberSheetComponent {
  readonly projectId = input.required<string>();

  readonly show = input(false);

  @Output() closed = new EventEmitter<void>();

  private fb = inject(FormBuilder);

  private projectClient = inject(PROJECT);

  private memberClient = inject(MEMBER);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  /** Falls back to the neutral word while the organization data is still
   *  loading, so the sheet title never reads "Add member to undefined". */
  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  availableUsers = signal<{ id: string; name: string }[]>([]);

  projectNamespaces = signal<string[]>([]);

  isAddingMember = signal(false);

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    permission: ['viewer', Validators.required],
  });

  private selectedUserId = signal('');

  selectedUserName = computed(() => {
    const id = this.selectedUserId();
    return this.availableUsers().find((user) => user.id === id)?.name ?? '';
  });

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

  constructor() {
    // The sheet outlives the page it was opened over, so opening starts from
    // nothing and fetches what the form offers rather than trusting last time.
    effect(() => {
      if (!this.show()) return;
      this.selectedUserId.set('');
      this.userFieldTouched.set(false);
      this.addSubmitted.set(false);
      this.draftNamespaces.set([]);
      this.draftRoles.set([]);
      this.newNamespaceName.set('');
      this.memberForm.reset({ userId: '', permission: 'viewer' });
      this.load(this.projectId());
    });
  }

  private async load(projectId: string) {
    const [project, organization, namespaces] = await Promise.all([
      firstValueFrom(this.projectClient.listProjectMembers({ projectId })).catch(() => null),
      firstValueFrom(this.memberClient.listMembers({})).catch(() => null),
      firstValueFrom(
        this.namespaceClient.listProjectNamespaces(
          create(ListProjectNamespacesRequestSchema, { projectId }),
        ),
      ).catch(() => null),
    ]);

    const inProject = new Set((project?.members ?? []).map((member) => member.userId));
    const available = (organization?.members ?? [])
      .filter((member) => member.externalRef && !inProject.has(member.userId))
      .map((member) => ({ id: member.userId, name: member.name }));
    this.availableUsers.set(available.length === 0 && isDevMode() ? SAMPLE_USERS : available);
    this.projectNamespaces.set((namespaces?.namespaces ?? []).map((namespace) => namespace.name));
  }

  onClose() {
    this.closed.emit();
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

  toggleDraftNamespace(namespace: string) {
    this.draftNamespaces.update((current) => toggleNamespace(current, namespace));
  }

  toggleDraftRole(role: string) {
    this.draftRoles.update((roles) =>
      roles.includes(role) ? roles.filter((current) => current !== role) : [...roles, role],
    );
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
      this.closed.emit();
      await this.grantDraftAccess(userId);
      this.notificationService.success(`'${this.selectedUserName()}' added to ${this.projectName()}`);
      // The member list is a page of its own, and it may well be the page behind
      // this sheet, so it hears about the new member from here.
      this.organizationData.membersChanged.update((count) => count + 1);
    } catch (err) {
      // The list's own retry banner belongs to that page and this sheet may be
      // standing over another one, so the failure reports where it can be seen.
      this.closed.emit();
      this.notificationService.error(
        err instanceof Error ? `Member not added: ${err.message}` : 'Member not added',
      );
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
        this.notificationService.success(`Namespace '${name}' created`);
        this.organizationData.namespacesChanged.update((count) => count + 1);
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
    // parked against the user and attached by the list that reloads.
    const roles = ALL_ROLES.filter((role) => this.draftRoles().includes(role));
    this.organizationData.pendingProjectGrant.set({
      userId,
      bindings: namespaces.map((namespace) => ({ namespace, roles })),
    });
  }
}
