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
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import {
  ListProjectNamespacesRequestSchema,
  CreateNamespaceRequestSchema,
} from '../../generated/v1/namespace_pb';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { ALL_ROLES, mockBindingsFor, setMockBindings } from '../utils/mock-role-bindings';
import { ALL_NAMESPACES } from '../utils/namespace-grants';
import { NAMESPACE, PROJECT } from '../../connect/tokens';
import type { ProjectMember } from '../../generated/v1/project_pb';
import '@nldd/design-system/token-field';

/**
 * Making a namespace, from wherever you were. The shell owns this sheet rather
 * than the namespace list, so the toolbar can open it over any page; that also
 * means it cannot borrow the list's data and loads the members and the existing
 * names itself when it opens.
 */
@Component({
  selector: 'app-new-namespace-sheet',
  imports: [ReactiveFormsModule, SheetSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './new-namespace-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NewNamespaceSheetComponent {
  readonly projectId = input.required<string>();

  readonly show = input(false);

  @Output() closed = new EventEmitter<void>();

  private fb = inject(FormBuilder);

  private projectClient = inject(PROJECT);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  /** Named in the sheet title, so it says which project the namespace lands in. */
  projectName = computed(() => {
    const gevonden = this.organizationDataService.getProjectById(this.projectId())?.project;
    return gevonden?.alias || gevonden?.name || 'this project';
  });

  members = signal<ProjectMember[]>([]);

  namespaceNames = signal<string[]>([]);

  createErrorMessage = signal<string | null>(null);

  isCreatingNamespace = signal<boolean>(false);

  /**
   * Set by pressing Create. A field you have not filled in yet is not wrong, it
   * is unfinished, so nothing goes red until you say you are done. The rules
   * under the field are visible from the start as the requirements; they only
   * turn critical once this is true.
   */
  createAttempted = signal<boolean>(false);

  /** Who gets access to the namespace being created, and with which roles. A
   *  namespace is made to work in, so the people come with it rather than in a
   *  second visit to the sheet. */
  draftMemberIds = signal<string[]>([]);

  draftRoles = signal<string[]>([]);

  readonly allRoles = ALL_ROLES;

  /** Members with an all-namespaces grant are left out: they reach the new one
   *  the moment it exists, so offering them here would promise a change that
   *  does not happen. */
  memberCandidates = computed(() =>
    this.members().filter(
      (member) =>
        !mockBindingsFor(member.id, this.namespaceNames()).some(
          (binding) => binding.namespace === ALL_NAMESPACES,
        ),
    ),
  );

  coveredMemberCount = computed(() => this.members().length - this.memberCandidates().length);

  namespaceForm = this.fb.group({
    name: [
      '',
      [
        Validators.required,
        Validators.minLength(1),
        Validators.maxLength(63),
        Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
      ],
    ],
  });

  constructor() {
    // Opening is the moment to start from nothing and to fetch what the form
    // offers: the sheet outlives the page it was opened over, so neither the
    // draft nor the member list can be left over from last time.
    effect(() => {
      if (!this.show()) return;
      this.namespaceForm.reset();
      this.createErrorMessage.set(null);
      this.draftMemberIds.set([]);
      this.draftRoles.set([]);
      this.load(this.projectId());
    });
  }

  private async load(projectId: string) {
    const [members, namespaces] = await Promise.all([
      firstValueFrom(this.projectClient.listProjectMembers({ projectId })).catch(() => null),
      firstValueFrom(
        this.namespaceClient.listProjectNamespaces(
          create(ListProjectNamespacesRequestSchema, { projectId }),
        ),
      ).catch(() => null),
    ]);
    this.members.set(members?.members ?? []);
    this.namespaceNames.set((namespaces?.namespaces ?? []).map((namespace) => namespace.name));
  }

  onClose() {
    this.closed.emit();
  }

  onMembersChange(event: Event): void {
    this.draftMemberIds.set((event as CustomEvent<{ values: string[] }>).detail.values);
  }

  toggleDraftRole(role: string): void {
    this.draftRoles.update((current) =>
      current.includes(role) ? current.filter((entry) => entry !== role) : [...current, role],
    );
  }

  async createNamespace(event?: Event) {
    // A control that acts on Enter itself, like picking an option in the token
    // field, marks the event handled. Submitting on that Enter would put the
    // form in error while the user is still filling it in.
    if (event?.defaultPrevented) return;

    event?.preventDefault();
    this.createErrorMessage.set(null);
    this.createAttempted.set(true);

    if (this.namespaceForm.invalid) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.projectId(),
        name: this.namespaceForm.value.name!,
      });

      await withIdempotency((opts) => this.namespaceClient.createNamespace(request, opts), {
        signal: this.idempotency.reset(),
      });

      const name = this.namespaceForm.value.name!;
      const granted = this.grantDraftAccess(name);

      this.closed.emit();
      this.notificationService.success(
        granted === 0
          ? `Namespace '${name}' created`
          : `Namespace '${name}' created, ${granted === 1 ? '1 member has' : `${granted} members have`} access`,
      );

      // The list that shows namespaces is a page of its own, and it may well be
      // the page behind this sheet, so it hears about the new one from here.
      this.organizationDataService.namespacesChanged.update((count) => count + 1);
      await this.organizationDataService.loadOrganizationData();
    } catch (error) {
      this.createErrorMessage.set(
        error instanceof Error
          ? `Failed to create namespace: ${error.message}`
          : 'Failed to create namespace',
      );
    } finally {
      this.isCreatingNamespace.set(false);
    }
  }

  /** TEMPORARY, dev only: roles have no API, so the grant goes into the mock
   *  store the rest of the screens read from. Delete with the mock bindings. */
  private grantDraftAccess(namespace: string): number {
    const memberIds = this.draftMemberIds();
    if (memberIds.length === 0) return 0;

    const roles = ALL_ROLES.filter((role) => this.draftRoles().includes(role));
    memberIds.forEach((memberId) => {
      const existing = mockBindingsFor(memberId, this.namespaceNames());
      setMockBindings(memberId, [...existing, { namespace, roles }]);
    });
    return memberIds.length;
  }

  onNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.namespaceForm.get('name')?.setValue(value);
    this.namespaceForm.get('name')?.markAsDirty();
  }

  /**
   * Only after pressing Create. Typing towards a name that is not finished yet
   * is not a mistake, and a field that turns red on the second character reads
   * as one. The rules stand under the field the whole time, so there is
   * something to go on before the verdict.
   */
  isNameInvalid(): boolean {
    return this.createAttempted() && !!this.namespaceForm.get('name')?.invalid;
  }

  getNameError(): string {
    const nameControl = this.namespaceForm.get('name');
    if (nameControl?.hasError('required')) {
      return 'Namespace name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'Namespace name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return 'Namespace name must start with a lowercase letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens.';
    }
    return '';
  }
}
