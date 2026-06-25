import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { Router, RouterLink, ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { CLUSTER, NAMESPACE, PROJECT } from '../../connect/tokens';
import {
  ListClusterNamespacesRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { fetchClusterName } from '../utils/cluster-status';
import DialogSyncDirective from '../dialog-sync.directive';
import DropdownSyncDirective from '../dropdown-sync.directive';
import focusFirstModalInput from '../modal-focus';
import LoadingIndicatorComponent from '../icons/loading-indicator.component';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';

@Component({
  selector: 'app-cluster-namespaces',
  imports: [
    ReactiveFormsModule,
    DialogSyncDirective,
    DropdownSyncDirective,
    RouterLink,
    LoadingIndicatorComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-namespaces.component.html',
})
export default class ClusterNamespacesComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private idempotency = createIdempotencyRef();

  private namespaceClient = inject(NAMESPACE);

  private projectClient = inject(PROJECT);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  private fb = inject(FormBuilder);

  private clusterId = '';

  errorMessage = signal<string | null>(null);

  isLoading = signal(true);

  namespaces = signal<Namespace[]>([]);

  selectedNamespaceIds = signal<Set<string>>(new Set());

  selectedCount = computed(() => this.selectedNamespaceIds().size);

  allSelected = computed(() => {
    const ns = this.namespaces();
    const selected = this.selectedNamespaceIds();
    return ns.length > 0 && ns.every((n) => selected.has(n.id));
  });

  someSelected = computed(() => this.selectedCount() > 0 && !this.allSelected());

  showBulkDeleteModal = signal<boolean>(false);

  isBulkDeleting = signal<boolean>(false);

  projects = signal<Project[]>([]);

  showAddNamespaceModal = signal<boolean>(false);

  isLoadingProjects = signal<boolean>(false);

  isCreatingNamespace = signal<boolean>(false);

  showDeleteNamespaceModal = signal<boolean>(false);

  pendingNamespaceId = signal<string | null>(null);

  pendingNamespaceName = signal<string | null>(null);

  clusterName = signal<string | null>(null);

  namespaceForm = this.fb.group({
    projectId: ['', Validators.required],
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
    this.titleService.setTitle('Cluster namespaces');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
    await Promise.all([
      fetchClusterName(this.client, this.clusterId).then((name) => this.clusterName.set(name)),
      this.loadNamespaces(),
      this.loadProjects(),
    ]);
    this.isLoading.set(false);
  }

  readonly formatDate = formatDateTimeUtil;

  async loadNamespaces(): Promise<void> {
    try {
      const request = create(ListClusterNamespacesRequestSchema, { clusterId: this.clusterId });
      const response = await firstValueFrom(this.namespaceClient.listClusterNamespaces(request));
      this.namespaces.set(response.namespaces);
      // Drop any selected ids that no longer exist (e.g. after a delete).
      const existing = new Set(response.namespaces.map((n) => n.id));
      this.selectedNamespaceIds.update((set) => new Set([...set].filter((id) => existing.has(id))));
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    }
  }

  async loadProjects(): Promise<void> {
    try {
      this.isLoadingProjects.set(true);
      const request = create(ListProjectsRequestSchema, { clusterId: this.clusterId });
      const response = await firstValueFrom(this.projectClient.listProjects(request));
      this.projects.set(response.projects);
      if (response.projects.length > 0) {
        this.namespaceForm.patchValue({ projectId: response.projects[0].id });
      }
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load projects: ${error.message}`
          : 'Failed to load projects',
      );
    } finally {
      this.isLoadingProjects.set(false);
    }
  }

  getProjectName(projectId: string): string {
    const project = this.projects().find((p) => p.id === projectId);
    return project?.alias || projectId;
  }

  openAddNamespaceModal(): void {
    this.namespaceForm.reset();
    this.showAddNamespaceModal.set(true);
    this.loadProjects();
  }

  async createNamespace(event?: Event): Promise<void> {
    event?.preventDefault();

    if (this.namespaceForm.invalid) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.namespaceForm.value.projectId!,
        name: this.namespaceForm.value.name!,
      });

      await withIdempotency((opts) => this.namespaceClient.createNamespace(request, opts), {
        signal: this.idempotency.reset(),
      });

      this.showAddNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);

      // Reload namespaces and organization data
      await Promise.all([
        this.loadNamespaces(),
        this.organizationDataService.loadOrganizationData(),
      ]);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create namespace: ${error.message}`
          : 'Failed to create namespace',
      );
    } finally {
      this.isCreatingNamespace.set(false);
    }
  }

  openDeleteNamespaceModal(namespaceId: string, namespaceName: string): void {
    this.pendingNamespaceId.set(namespaceId);
    this.pendingNamespaceName.set(namespaceName);
    this.showDeleteNamespaceModal.set(true);
  }

  async confirmDeleteNamespace(): Promise<void> {
    const namespaceId = this.pendingNamespaceId();
    const namespaceName = this.pendingNamespaceName();
    if (!namespaceId) return;

    this.showDeleteNamespaceModal.set(false);

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.namespaceClient.deleteNamespace(request));

      this.toastService.success(`Namespace '${namespaceName}' deleted`);

      // Reload namespaces and organization data
      await Promise.all([
        this.loadNamespaces(),
        this.organizationDataService.loadOrganizationData(),
      ]);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete namespace: ${error.message}`
          : 'Failed to delete namespace',
      );
    }
  }

  isSelected(namespaceId: string): boolean {
    return this.selectedNamespaceIds().has(namespaceId);
  }

  setNamespaceSelected(namespaceId: string, checked: boolean): void {
    this.selectedNamespaceIds.update((set) => {
      const next = new Set(set);
      if (checked) {
        next.add(namespaceId);
      } else {
        next.delete(namespaceId);
      }
      return next;
    });
  }

  toggleSelectAll(checked: boolean): void {
    this.selectedNamespaceIds.set(
      checked ? new Set(this.namespaces().map((n) => n.id)) : new Set(),
    );
  }

  openBulkDeleteModal(): void {
    if (this.selectedCount() === 0) return;
    this.showBulkDeleteModal.set(true);
  }

  async confirmBulkDelete(): Promise<void> {
    const ids = [...this.selectedNamespaceIds()];
    if (ids.length === 0) return;

    this.showBulkDeleteModal.set(false);
    this.isBulkDeleting.set(true);

    try {
      const results = await Promise.allSettled(
        ids.map((namespaceId) =>
          firstValueFrom(
            this.namespaceClient.deleteNamespace(
              create(DeleteNamespaceRequestSchema, { namespaceId }),
            ),
          ),
        ),
      );

      const failed = results.filter((r) => r.status === 'rejected').length;
      const succeeded = ids.length - failed;

      if (succeeded > 0) {
        this.toastService.success(`${succeeded} namespace${succeeded === 1 ? '' : 's'} deleted`);
      }
      if (failed > 0) {
        this.errorMessage.set(`Failed to delete ${failed} namespace${failed === 1 ? '' : 's'}.`);
      }

      this.selectedNamespaceIds.set(new Set());
      await Promise.all([this.loadNamespaces(), this.organizationDataService.loadOrganizationData()]);
    } finally {
      this.isBulkDeleting.set(false);
    }
  }

  bulkDeleteDialogRef = viewChild<ElementRef<HTMLElement>>('bulkDeleteDialog');

  onBulkDeleteModalOpen(): void {
    const el = this.bulkDeleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onNamespaceNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.namespaceForm.get('name')?.setValue(value);
    this.namespaceForm.get('name')?.markAsDirty();
  }

  getNamespaceNameError(): string {
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

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }

  addNamespaceDialogRef = viewChild<ElementRef<HTMLElement>>('addNamespaceDialog');

  onAddNamespaceModalOpen(): void {
    const el = this.addNamespaceDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  deleteNamespaceDialogRef = viewChild<ElementRef<HTMLElement>>('deleteNamespaceDialog');

  onDeleteNamespaceModalOpen(): void {
    const el = this.deleteNamespaceDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
