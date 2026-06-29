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
import { ActivatedRoute, Router } from '@angular/router';
import { ConnectError, Code } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import AutofocusDirective from '../autofocus.directive';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { PROJECT, NAMESPACE, CLUSTER } from '../../connect/tokens';
import {
  GetProjectRequestSchema,
  UpdateProjectRequestSchema,
  DeleteProjectRequestSchema,
  Project,
} from '../../generated/v1/project_pb';
import { ListProjectNamespacesRequestSchema, Namespace } from '../../generated/v1/namespace_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { LoadingIndicatorComponent } from '../icons';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-project-detail',
  imports: [LoadingIndicatorComponent, DialogSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './project-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private namespaceClient = inject(NAMESPACE);

  private clusterClient = inject(CLUSTER);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  project = signal<Project | null>(null);

  namespaces = signal<Namespace[]>([]);

  clusters = signal<ClusterSummary[]>([]);

  isLoading = signal<boolean>(true);

  errorMessage = signal<string | null>(null);

  showEditModal = signal<boolean>(false);

  editingAlias = signal<string>('');

  saving = signal<boolean>(false);

  showDeleteModal = signal<boolean>(false);

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
      this.titleService.setTitle(response.project.alias || response.project.name);

      // Load namespaces and clusters for read-only display
      await Promise.all([this.loadNamespaces(projectId), this.loadClusters()]);
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load project: ${error.message}`
          : 'Failed to load project',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    }
  }

  async loadClusters() {
    try {
      const request = create(ListClustersRequestSchema, {});
      const response = await firstValueFrom(this.clusterClient.listClusters(request));
      this.clusters.set(response.clusters);
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters',
      );
    }
  }

  getClusterName(clusterId: string): string {
    const cluster = this.clusters().find((c) => c.id === clusterId);
    return cluster?.name || clusterId;
  }

  openEditModal() {
    const currentProject = this.project();
    if (!currentProject) return;
    this.editingAlias.set(currentProject.alias || currentProject.name);
    this.showEditModal.set(true);
  }

  async saveEdit() {
    const currentProject = this.project();
    const aliasToSave = this.editingAlias();

    if (!aliasToSave.trim() || !currentProject) {
      return;
    }

    this.saving.set(true);
    this.errorMessage.set(null);

    try {
      const request = create(UpdateProjectRequestSchema, {
        projectId: currentProject.id,
        alias: aliasToSave.trim(),
      });
      await firstValueFrom(this.projectClient.updateProject(request));

      this.project.set({
        ...currentProject,
        alias: aliasToSave.trim(),
      });
      this.organizationDataService.updateProjectAlias(currentProject.id, aliasToSave.trim());
      this.titleService.setTitle(aliasToSave.trim());
      this.showEditModal.set(false);
      this.editingAlias.set('');
    } catch (err) {
      this.showEditModal.set(false);
      this.errorMessage.set(
        err instanceof Error
          ? `Failed to update project alias: ${err.message}`
          : 'Failed to update project alias',
      );
    } finally {
      this.saving.set(false);
    }
  }

  async deleteProject() {
    const currentProject = this.project();
    if (!currentProject) return;

    try {
      const request = create(DeleteProjectRequestSchema, {
        projectId: currentProject.id,
      });

      await firstValueFrom(this.projectClient.deleteProject(request));

      this.showDeleteModal.set(false);
      this.toastService.success(`Project '${currentProject.name}' deleted`);

      // Reload project data to update the selector modal
      await this.organizationDataService.reloadProjectsAndNamespaces();

      this.router.navigate(['/projects']);
    } catch (err) {
      this.showDeleteModal.set(false);
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        this.errorMessage.set('Delete all namespaces in this project before deleting the project.');
      } else {
        this.errorMessage.set(
          err instanceof Error
            ? `Failed to delete project: ${err.message}`
            : 'Failed to delete project',
        );
      }
    }
  }

  editDialogRef = viewChild<ElementRef<HTMLElement>>('editDialog');

  onEditModalOpen(): void {
    const el = this.editDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  readonly formatDate = formatDateUtil;
}
