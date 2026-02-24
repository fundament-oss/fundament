import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { Router, RouterLink, ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerTrash, tablerAlertTriangle } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
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
import ModalComponent from '../modal/modal.component';
import { formatDateTime as formatDateTimeUtil } from '../utils/date-format';

@Component({
  selector: 'app-cluster-namespaces',
  imports: [ReactiveFormsModule, NgIcon, ModalComponent, RouterLink],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPlus,
      tablerTrash,
      tablerAlertTriangle,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-namespaces.component.html',
})
export default class ClusterNamespacesComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private namespaceClient = inject(NAMESPACE);

  private projectClient = inject(PROJECT);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  private fb = inject(FormBuilder);

  private clusterId = '';

  errorMessage = signal<string | null>(null);

  isLoading = signal(true);

  namespaces = signal<Namespace[]>([]);

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
    return project?.name || projectId;
  }

  openAddNamespaceModal(): void {
    this.namespaceForm.reset();
    this.showAddNamespaceModal.set(true);
    this.loadProjects();
  }

  async createNamespace(): Promise<void> {
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

      await firstValueFrom(this.namespaceClient.createNamespace(request));

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

      this.toastService.info(`Namespace '${namespaceName}' deleted`);

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
}
