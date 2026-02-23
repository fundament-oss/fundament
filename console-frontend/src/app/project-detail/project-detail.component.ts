import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink, ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPencil } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import {
  GetProjectRequestSchema,
  ListProjectNamespacesRequestSchema,
  Project,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { LoadingIndicatorComponent } from '../icons';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-project-detail',
  imports: [RouterLink, NgIcon, LoadingIndicatorComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPencil,
    }),
  ],
  templateUrl: './project-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private projectClient = inject(PROJECT);

  private clusterClient = inject(CLUSTER);

  private toastService = inject(ToastService);

  project = signal<Project | null>(null);

  namespaces = signal<ProjectNamespace[]>([]);

  clusters = signal<ClusterSummary[]>([]);

  isLoading = signal<boolean>(true);

  errorMessage = signal<string | null>(null);

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
      const response = await firstValueFrom(this.projectClient.listProjectNamespaces(request));
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

  readonly formatDate = formatDateUtil;
}
