import { Component, inject, signal, OnInit, ChangeDetectionStrategy, viewChild, ElementRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetProjectRequestSchema,
  DeleteProjectRequestSchema,
  ListProjectNamespacesRequestSchema,
  Project,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import {
  PlusIconComponent,
  TrashIconComponent,
  ErrorIconComponent,
  WarningIconComponent,
} from '../icons';

@Component({
  selector: 'app-project-detail',
  imports: [
    CommonModule,
    RouterLink,
    ReactiveFormsModule,
    PlusIconComponent,
    TrashIconComponent,
    ErrorIconComponent,
    WarningIconComponent,
  ],
  templateUrl: './project-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private fb = inject(FormBuilder);
  private projectClient = inject(PROJECT);
  private clusterClient = inject(CLUSTER);
  private toastService = inject(ToastService);

  project = signal<Project | null>(null);
  namespaces = signal<ProjectNamespace[]>([]);
  clusters = signal<ClusterSummary[]>([]);

  isLoading = signal<boolean>(true);
  errorMessage = signal<string | null>(null);

  showDeleteModal = signal<boolean>(false);
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

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    await this.loadProject(projectId);
  }

  async loadProject(projectId: string) {
    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(GetProjectRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.getProject(request));

      if (!response.project) {
        throw new Error('Project not found');
      }

      this.project.set(response.project);
      this.titleService.setTitle(response.project.name);

      // Load namespaces and clusters
      await Promise.all([this.loadNamespaces(projectId), this.loadClusters()]);
    } catch (error) {
      console.error('Failed to fetch project:', error);
      this.errorMessage.set(
        error instanceof Error ? `Failed to load project: ${error.message}` : 'Failed to load project',
      );
    } finally {
      this.isLoading.set(false);
    }
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
    const cluster = this.clusters().find(c => c.id === clusterId);
    return cluster?.name || clusterId;
  }

  openCreateNamespaceModal() {
    this.namespaceForm.reset();
    this.showCreateNamespaceModal.set(true);
    this.loadClusters();
    setTimeout(() => this.namespaceNameInput()?.nativeElement.focus());
  }

  async createNamespace() {
    if (this.namespaceForm.invalid || !this.project()) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.project()!.id,
        clusterId: this.namespaceForm.value.clusterId!,
        name: this.namespaceForm.value.name!,
      });

      await firstValueFrom(this.clusterClient.createNamespace(request));

      this.showCreateNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);
      await this.loadNamespaces(this.project()!.id);
    } catch (error) {
      console.error('Failed to create namespace:', error);
      this.errorMessage.set(
        error instanceof Error ? `Failed to create namespace: ${error.message}` : 'Failed to create namespace',
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
      await this.loadNamespaces(this.project()!.id);
    } catch (error) {
      console.error('Failed to delete namespace:', error);
      this.errorMessage.set(
        error instanceof Error ? `Failed to delete namespace: ${error.message}` : 'Failed to delete namespace',
      );
    }
  }

  async deleteProject() {
    if (!this.project()) return;

    try {
      const request = create(DeleteProjectRequestSchema, {
        projectId: this.project()!.id,
      });

      await firstValueFrom(this.projectClient.deleteProject(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`Project '${this.project()!.name}' deleted`);
      this.router.navigate(['/projects']);
    } catch (error) {
      console.error('Failed to delete project:', error);
      this.showDeleteModal.set(false);
      this.errorMessage.set(
        error instanceof Error ? `Failed to delete project: ${error.message}` : 'Failed to delete project',
      );
    }
  }

  formatDate(dateString?: string): string {
    if (!dateString) return 'Unknown';
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
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
