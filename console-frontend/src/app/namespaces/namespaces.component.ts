import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  viewChild,
  ElementRef,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListProjectNamespacesRequestSchema,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerTrash } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { ModalComponent } from '../modal/modal.component';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-namespaces',
  imports: [CommonModule, ReactiveFormsModule, NgIcon, ModalComponent, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPlus,
      tablerTrash,
    }),
  ],
  templateUrl: './namespaces.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class NamespacesComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private fb = inject(FormBuilder);
  private projectClient = inject(PROJECT);
  private clusterClient = inject(CLUSTER);
  private toastService = inject(ToastService);
  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');
  namespaces = signal<ProjectNamespace[]>([]);
  clusters = signal<ClusterSummary[]>([]);

  errorMessage = signal<string | null>(null);
  showCreateNamespaceModal = signal<boolean>(false);
  isLoadingClusters = signal<boolean>(false);
  isCreatingNamespace = signal<boolean>(false);

  namespaceNameInput = viewChild<ElementRef<HTMLInputElement>>('namespaceNameInput');

  namespaceForm = this.fb.group({
    clusterId: ['', Validators.required],
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
    await Promise.all([this.loadNamespaces(projectId), this.loadClusters()]);
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      console.error('Failed to fetch namespaces:', error);
      this.toastService.error('Failed to load namespaces');
    }
  }

  async loadClusters() {
    try {
      this.isLoadingClusters.set(true);
      const request = create(ListClustersRequestSchema, {});
      const response = await firstValueFrom(this.clusterClient.listClusters(request));
      this.clusters.set(response.clusters);
      if (response.clusters.length > 0) {
        this.namespaceForm.patchValue({ clusterId: response.clusters[0].id });
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error);
    } finally {
      this.isLoadingClusters.set(false);
    }
  }

  getClusterName(clusterId: string): string {
    const cluster = this.clusters().find((c) => c.id === clusterId);
    return cluster?.name || clusterId;
  }

  openCreateNamespaceModal() {
    this.namespaceForm.reset();
    this.showCreateNamespaceModal.set(true);
    this.loadClusters();
    setTimeout(() => this.namespaceNameInput()?.nativeElement.focus());
  }

  async createNamespace() {
    if (this.namespaceForm.invalid) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.projectId(),
        clusterId: this.namespaceForm.value.clusterId!,
        name: this.namespaceForm.value.name!,
      });

      await firstValueFrom(this.clusterClient.createNamespace(request));

      this.showCreateNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);

      // Reload organization data to update the selector modal
      await Promise.all([
        this.loadNamespaces(this.projectId()),
        this.organizationDataService.reloadOrganizationData(),
      ]);
    } catch (error) {
      console.error('Failed to create namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create namespace: ${error.message}`
          : 'Failed to create namespace',
      );
    } finally {
      this.isCreatingNamespace.set(false);
    }
  }

  async deleteNamespace(namespaceId: string, namespaceName: string) {
    if (!confirm(`Are you sure you want to delete namespace '${namespaceName}'?`)) {
      return;
    }

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.clusterClient.deleteNamespace(request));

      this.toastService.info(`Namespace '${namespaceName}' deleted`);

      // Reload organization data to update the selector modal
      await Promise.all([
        this.loadNamespaces(this.projectId()),
        this.organizationDataService.reloadOrganizationData(),
      ]);
    } catch (error) {
      console.error('Failed to delete namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete namespace: ${error.message}`
          : 'Failed to delete namespace',
      );
    }
  }

  readonly formatDate = formatDateUtil;

  timestampToDate(timestamp: Timestamp | undefined): string | undefined {
    if (!timestamp) return undefined;
    return timestampDate(timestamp).toISOString();
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

  get breadcrumbSegments(): BreadcrumbSegment[] {
    return [
      { label: 'Projects', route: '/projects' },
      { label: 'Project', route: `/projects/${this.projectId()}` },
      { label: 'Namespaces' },
    ];
  }
}
