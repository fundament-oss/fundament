import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  effect,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router, RouterOutlet } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import PageNavService from '../page-nav.service';
import { OrganizationDataService } from '../organization-data.service';
import {
  ListProjectNamespacesRequestSchema,
  CreateNamespaceRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { formatDate as formatDateUtil } from '../utils/date-format';
import { ALL_ROLES, mockBindingsFor, setMockBindings } from '../utils/mock-role-bindings';
import { ALL_NAMESPACES } from '../utils/namespace-grants';
import { NAMESPACE, PROJECT } from '../../connect/tokens';
import type { ProjectMember } from '../../generated/v1/project_pb';

@Component({
  selector: 'app-namespaces',
  imports: [
    ReactiveFormsModule,
    DialogSyncDirective,
    SheetSyncDirective,
    AutofocusDirective,
    RouterOutlet,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespaces.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespacesComponent implements OnInit {
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
    if (this.routeQuery().get('add') !== null && !this.showCreateNamespaceModal()) this.openCreateNamespaceModal();
  });

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private fb = inject(FormBuilder);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');

  /** Named in the sheet title, so it says which project the namespace lands in;
   *  the page header is behind the sheet while it is open. */
  projectName = computed(() => {
    const gevonden = this.organizationDataService.getProjectById(this.projectId())?.project;
    return gevonden?.alias || gevonden?.name || 'this project';
  });

  namespaces = signal<Namespace[]>([]);

  /** True until the first load lands: an empty list and a list not yet loaded
   *  look the same, and only one of them should offer to create something. */
  isLoading = signal(true);

  members = signal<ProjectMember[]>([]);

  namespaceNames = computed(() => this.namespaces().map((namespace) => namespace.name));

  errorMessage = signal<string | null>(null);

  /** Kept apart from errorMessage: a failed create belongs in the sheet the user is still in. */
  createErrorMessage = signal<string | null>(null);

  showCreateNamespaceModal = signal<boolean>(false);

  isCreatingNamespace = signal<boolean>(false);

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
    this.titleService.setTitle('Namespaces');
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    await Promise.all([this.loadNamespaces(projectId), this.loadMembers(projectId)]);
  }

  /** Only for the member summary on a row; the sheet has the detail. */
  async loadMembers(projectId: string) {
    try {
      const response = await firstValueFrom(this.projectClient.listProjectMembers({ projectId }));
      this.members.set(response.members);
    } catch {
      this.members.set([]);
    }
  }

  /** Who works in this namespace. One of them is worth naming: the name says
   *  more than the number does. */
  memberSummary = (namespace: string): string => {
    const names = this.members()
      .filter((member) => {
        const bindings = mockBindingsFor(member.id, this.namespaceNames());
        return bindings.some(
          (binding) => binding.namespace === namespace || binding.namespace === ALL_NAMESPACES,
        );
      })
      .map((member) => member.userName);
    if (names.length === 1) return names[0];
    return `${names.length} members`;
  };

  /** Routes client-side while leaving the row a real link, so middle-click and
   *  "open in new tab" keep working. */
  openNamespace(event: Event, name: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.router.navigateByUrl(`/projects/${this.projectId()}/namespaces/${name}`);
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  closeCreateNamespaceModal() {
    this.showCreateNamespaceModal.set(false);
    if (this.route.snapshot.queryParamMap.get('add') !== null) {
      this.router.navigate([], {
        relativeTo: this.route,
        queryParams: { add: null },
        queryParamsHandling: 'merge',
        replaceUrl: true,
      });
    }
  }

  openCreateNamespaceModal() {
    this.namespaceForm.reset();
    this.createErrorMessage.set(null);
    this.draftMemberIds.set([]);
    this.draftRoles.set([]);
    this.showCreateNamespaceModal.set(true);
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

      this.showCreateNamespaceModal.set(false);
      this.notificationService.success(
        granted === 0
          ? `Namespace '${name}' created`
          : `Namespace '${name}' created, ${granted === 1 ? '1 member has' : `${granted} members have`} access`,
      );

      // Reload organization data to update the selector modal
      await Promise.all([
        this.loadNamespaces(this.projectId()),
        this.organizationDataService.loadOrganizationData(),
      ]);
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

  readonly formatDate = formatDateUtil;

  onNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.namespaceForm.get('name')?.setValue(value);
    this.namespaceForm.get('name')?.markAsDirty();
  }

  /** Also covers `touched`, so submitting an untouched empty field shows the error. */
  isNameInvalid(): boolean {
    const nameControl = this.namespaceForm.get('name');
    return !!nameControl?.invalid && (nameControl.dirty || nameControl.touched);
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
