import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { NAMESPACE } from '../../connect/tokens';
import {
  ListProjectNamespacesRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { formatDate as formatDateUtil } from '../utils/date-format';
import { NamespaceSelection } from '../utils/namespace-selection';

@Component({
  selector: 'app-namespaces',
  imports: [ReactiveFormsModule, DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespaces.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespacesComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private fb = inject(FormBuilder);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');

  namespaces = signal<Namespace[]>([]);

  protected selection = new NamespaceSelection(() => this.namespaces().map((n) => n.id));

  showBulkDeleteModal = signal<boolean>(false);

  isBulkDeleting = signal<boolean>(false);

  errorMessage = signal<string | null>(null);

  showCreateNamespaceModal = signal<boolean>(false);

  isCreatingNamespace = signal<boolean>(false);

  showDeleteNamespaceModal = signal<boolean>(false);

  pendingNamespaceId = signal<string | null>(null);

  pendingNamespaceName = signal<string | null>(null);

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
    await this.loadNamespaces(projectId);
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
      this.selection.retainVisible();
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    }
  }

  openCreateNamespaceModal() {
    this.namespaceForm.reset();
    this.showCreateNamespaceModal.set(true);
  }

  async createNamespace(event?: Event) {
    event?.preventDefault();
    this.errorMessage.set(null);

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

      this.showCreateNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);

      // Reload organization data to update the selector modal
      await Promise.all([
        this.loadNamespaces(this.projectId()),
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

  openDeleteNamespaceModal(namespaceId: string, namespaceName: string) {
    this.pendingNamespaceId.set(namespaceId);
    this.pendingNamespaceName.set(namespaceName);
    this.showDeleteNamespaceModal.set(true);
  }

  async confirmDeleteNamespace() {
    const namespaceId = this.pendingNamespaceId();
    const namespaceName = this.pendingNamespaceName();
    if (!namespaceId) return;

    this.errorMessage.set(null);
    this.showDeleteNamespaceModal.set(false);

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.namespaceClient.deleteNamespace(request));

      this.toastService.success(`Namespace '${namespaceName}' deleted`);

      // Reload organization data to update the selector modal
      await Promise.all([
        this.loadNamespaces(this.projectId()),
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

  readonly formatDate = formatDateUtil;

  openBulkDeleteModal(): void {
    if (this.selection.count() === 0) return;
    this.showBulkDeleteModal.set(true);
  }

  async confirmBulkDelete(): Promise<void> {
    const ids = this.selection.ids();
    if (ids.length === 0) return;

    this.errorMessage.set(null);
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

      this.selection.clear();
      await Promise.all([
        this.loadNamespaces(this.projectId()),
        this.organizationDataService.loadOrganizationData(),
      ]);
    } finally {
      this.isBulkDeleting.set(false);
    }
  }

  bulkDeleteDialogRef = viewChild<ElementRef<HTMLElement>>('bulkDeleteDialog');

  onBulkDeleteModalOpen(): void {
    const el = this.bulkDeleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.namespaceForm.get('name')?.setValue(value);
    this.namespaceForm.get('name')?.markAsDirty();
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

  createNamespaceDialogRef = viewChild<ElementRef<HTMLElement>>('createNamespaceDialog');

  onCreateNamespaceModalOpen(): void {
    const el = this.createNamespaceDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  deleteNamespaceDialogRef = viewChild<ElementRef<HTMLElement>>('deleteNamespaceDialog');

  onDeleteNamespaceModalOpen(): void {
    const el = this.deleteNamespaceDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
