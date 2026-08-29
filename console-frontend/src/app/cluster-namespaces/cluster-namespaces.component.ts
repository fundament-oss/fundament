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
import { Router, ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { CLUSTER, NAMESPACE, PROJECT } from '../../connect/tokens';
import {
  ListClusterNamespacesRequestSchema,
  DeleteNamespaceRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { fetchClusterName } from '../utils/cluster-status';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';
import NamespaceSelection from '../utils/namespace-selection';
import PageNavService from '../page-nav.service';

@Component({
  selector: 'app-cluster-namespaces',
  imports: [ReactiveFormsModule, DialogSyncDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-namespaces.component.html',
})
export default class ClusterNamespacesComponent implements OnInit {
  private pageNav = inject(PageNavService);

  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private idempotency = createIdempotencyRef();

  private namespaceClient = inject(NAMESPACE);

  private projectClient = inject(PROJECT);

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  private fb = inject(FormBuilder);

  private clusterId = '';

  errorMessage = signal<string | null>(null);

  /** Kept apart from errorMessage: a failed create belongs in the sheet the user is still in. */

  isLoading = signal(true);

  namespaces = signal<Namespace[]>([]);

  protected selection = new NamespaceSelection(() => this.namespaces().map((n) => n.id));

  showBulkDeleteModal = signal<boolean>(false);

  isBulkDeleting = signal<boolean>(false);

  projects = signal<Project[]>([]);

  isLoadingProjects = signal<boolean>(false);

  showDeleteNamespaceModal = signal<boolean>(false);

  pendingNamespaceId = signal<string | null>(null);

  pendingNamespaceName = signal<string | null>(null);

  clusterName = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Namespaces');
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
      this.selection.retainVisible();
    } catch (error) {
      this.notificationService.error(
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
    } catch (error) {
      this.notificationService.error(
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

  openDeleteNamespaceModal(namespaceId: string, namespaceName: string): void {
    this.pendingNamespaceId.set(namespaceId);
    this.pendingNamespaceName.set(namespaceName);
    this.showDeleteNamespaceModal.set(true);
  }

  async confirmDeleteNamespace(): Promise<void> {
    const namespaceId = this.pendingNamespaceId();
    const namespaceName = this.pendingNamespaceName();
    if (!namespaceId) return;

    this.errorMessage.set(null);
    this.showDeleteNamespaceModal.set(false);

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.namespaceClient.deleteNamespace(request));

      this.notificationService.success(`Namespace '${namespaceName}' deleted`);

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
        this.notificationService.success(
          `${succeeded} namespace${succeeded === 1 ? '' : 's'} deleted`,
        );
      }
      if (failed > 0) {
        this.errorMessage.set(`Failed to delete ${failed} namespace${failed === 1 ? '' : 's'}.`);
      }

      this.selection.clear();
      await Promise.all([
        this.loadNamespaces(),
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

  onCancel() {
    this.pageNav.goTo(`/clusters/${this.clusterId}`);
  }

  deleteNamespaceDialogRef = viewChild<ElementRef<HTMLElement>>('deleteNamespaceDialog');

  onDeleteNamespaceModalOpen(): void {
    const el = this.deleteNamespaceDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
