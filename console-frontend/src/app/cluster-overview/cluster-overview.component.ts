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
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { CLUSTER, PROJECT, PLUGIN } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetClusterRequestSchema,
  ListNodePoolsRequestSchema,
  DeleteClusterRequestSchema,
  ListClusterNamespacesRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  ListInstallsRequestSchema,
  NodePool,
  ClusterNamespace,
} from '../../generated/v1/cluster_pb';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import { NodePoolStatus } from '../../generated/v1/common_pb';
import { firstValueFrom } from 'rxjs';
import {
  EditIconComponent,
  TerminalIconComponent,
  DownloadIconComponent,
  UpgradeIconComponent,
  ErrorIconComponent,
  WarningIconComponent,
  PlusIconComponent,
  TrashIconComponent,
} from '../icons';
import {
  CardComponent,
  ButtonComponent,
  BadgeComponent,
  AlertComponent,
  SpinnerComponent,
  ModalComponent,
  ProgressBarComponent,
  EmptyStateComponent,
} from '../shared/components';

@Component({
  selector: 'app-cluster-overview',
  imports: [
    CommonModule,
    RouterLink,
    ReactiveFormsModule,
    EditIconComponent,
    TerminalIconComponent,
    DownloadIconComponent,
    UpgradeIconComponent,
    ErrorIconComponent,
    WarningIconComponent,
    PlusIconComponent,
    TrashIconComponent,
    CardComponent,
    ButtonComponent,
    BadgeComponent,
    AlertComponent,
    SpinnerComponent,
    ModalComponent,
    ProgressBarComponent,
    EmptyStateComponent,
  ],
  templateUrl: './cluster-overview.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ClusterOverviewComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private client = inject(CLUSTER);
  private projectClient = inject(PROJECT);
  private pluginClient = inject(PLUGIN);
  private toastService = inject(ToastService);
  private fb = inject(FormBuilder);

  // Expose enum for use in template
  NodePoolStatus = NodePoolStatus;

  errorMessage = signal<string | null>(null);
  isLoading = signal<boolean>(true);
  showDeleteModal = signal<boolean>(false);

  // Namespace management
  namespaces = signal<ClusterNamespace[]>([]);
  projects = signal<Project[]>([]);
  showAddNamespaceModal = signal<boolean>(false);
  isLoadingProjects = signal<boolean>(false);
  isCreatingNamespace = signal<boolean>(false);

  // Plugin data
  installedPlugins = signal<PluginSummary[]>([]);
  isLoadingPlugins = signal<boolean>(true);

  namespaceNameInput = viewChild<ElementRef<HTMLInputElement>>('namespaceNameInput');

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

  // Cluster data with API-fetched and mock data
  clusterData = {
    basics: {
      id: '',
      name: '',
      region: '',
      kubernetesVersion: '',
    },
    status: '',
    creationDate: '2024-11-15T10:30:00Z', // Mock data - not available from API
    activity: [
      {
        timestamp: '2024-12-06T14:30:00Z',
        action: 'Node pool scaled up',
        details: 'Added 2 nodes to default pool',
      },
      {
        timestamp: '2024-12-06T12:15:00Z',
        action: 'Plugin updated',
        details: 'Updated monitoring plugin to v2.1.3',
      },
      {
        timestamp: '2024-12-04T11:10:00Z',
        action: 'Node maintenance',
        details: 'Completed maintenance on node-3',
      },
      {
        timestamp: '2024-12-03T08:40:00Z',
        action: 'Resource limit adjusted',
        details: 'Increased memory limit for database pod',
      },
      {
        timestamp: '2024-12-02T13:55:00Z',
        action: 'User access granted',
        details: 'Added developer@company.com to cluster',
      },
      {
        timestamp: '2024-12-01T10:15:00Z',
        action: 'Monitoring alert resolved',
        details: 'High CPU usage alert cleared',
      },
    ],
    members: [
      { name: 'John Doe', role: 'admin', lastActive: '2024-12-06T14:20:00Z' },
      { name: 'Jane Smith', role: 'edit', lastActive: '2024-12-06T11:45:00Z' },
      { name: 'Mike Johnson', role: 'view', lastActive: '2024-12-05T16:30:00Z' },
      { name: 'Sarah Wilson', role: 'edit', lastActive: '2024-12-04T09:15:00Z' },
    ],
    nodePools: [] as NodePool[],
    resourceUsage: {
      cpu: { used: 2.4, limit: 8.0, unit: 'cores' },
      memory: { used: 12.8, limit: 32.0, unit: 'GB' },
      disk: { used: 45.2, limit: 100.0, unit: 'GB' },
      pods: { used: 28, limit: 110, unit: 'pods' },
    },
    workerNodes: {
      nodeType: 'n1-standard-2 (2 vCPU, 7.5 GB RAM)',
      minAutoscaling: 1,
      maxAutoscaling: 5,
    },
  };

  async ngOnInit() {
    const clusterId = this.route.snapshot.params['id'];

    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(GetClusterRequestSchema, { clusterId });
      const response = await firstValueFrom(this.client.getCluster(request));

      if (!response.cluster) {
        throw new Error('Cluster not found');
      }

      // Update cluster data with API response
      this.clusterData.basics = {
        id: response.cluster.id,
        name: response.cluster.name,
        region: response.cluster.region,
        kubernetesVersion: response.cluster.kubernetesVersion,
      };
      this.clusterData.status = response.cluster.status.toString();

      this.titleService.setTitle(response.cluster.name);

      // Fetch node pools
      const nodePoolsRequest = create(ListNodePoolsRequestSchema, { clusterId });
      const nodePoolsResponse = await firstValueFrom(this.client.listNodePools(nodePoolsRequest));

      // Map node pools to the expected format
      this.clusterData.nodePools = nodePoolsResponse.nodePools;

      // Fetch namespaces, projects, and plugins in parallel
      await Promise.all([
        this.loadNamespaces(clusterId),
        this.loadProjects(),
        this.loadInstalledPlugins(clusterId),
      ]);
    } catch (error) {
      console.error('Failed to fetch cluster data:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load cluster: ${error.message}`
          : 'Failed to load cluster data',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  getStatusColor(status: string): string {
    const colors: Record<string, string> = {
      provisioning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
      starting: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
      running: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
      upgrading: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-200',
      error: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200',
      stopping: 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200',
      stopped: 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200',
    };
    return colors[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200';
  }

  formatDate(dateString: string): string {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  getUsagePercentage(used: number, limit: number): number {
    return Math.round((used / limit) * 100);
  }

  getUsageVariant(percentage: number): 'default' | 'success' | 'warning' | 'danger' {
    if (percentage >= 90) return 'danger';
    if (percentage >= 75) return 'warning';
    return 'success';
  }

  openTerminal(): void {
    // Mock implementation - would open terminal in real app
    console.log('Opening terminal for cluster:', this.clusterData.basics.name);
  }

  downloadKubeconfig(): void {
    // Mock implementation - would download kubeconfig in real app
    console.log('Downloading kubeconfig for cluster:', this.clusterData.basics.name);
  }

  getNodePoolStatusLabel(status: NodePoolStatus): string {
    const labels: Record<NodePoolStatus, string> = {
      [NodePoolStatus.UNSPECIFIED]: 'Unspecified',
      [NodePoolStatus.HEALTHY]: 'Healthy',
      [NodePoolStatus.DEGRADED]: 'Degraded',
      [NodePoolStatus.UNHEALTHY]: 'Unhealthy',
    };
    return labels[status] || 'Unknown';
  }

  async deleteCluster(): Promise<void> {
    try {
      const request = create(DeleteClusterRequestSchema, {
        clusterId: this.clusterData.basics.id,
      });

      await firstValueFrom(this.client.deleteCluster(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`The cluster '${this.clusterData.basics.name}' has been deleted`);
      this.router.navigate(['/']);
    } catch (error) {
      console.error('Failed to delete cluster:', error);
      this.showDeleteModal.set(false);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete cluster: ${error.message}`
          : 'Failed to delete cluster',
      );
    }
  }

  // Namespace management methods
  async loadNamespaces(clusterId: string): Promise<void> {
    try {
      const request = create(ListClusterNamespacesRequestSchema, { clusterId });
      const response = await firstValueFrom(this.client.listClusterNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      console.error('Failed to load namespaces:', error);
      this.toastService.error('Failed to load namespaces');
    }
  }

  async loadProjects(): Promise<void> {
    try {
      this.isLoadingProjects.set(true);
      const request = create(ListProjectsRequestSchema, {});
      const response = await firstValueFrom(this.projectClient.listProjects(request));
      this.projects.set(response.projects);
      if (response.projects.length > 0) {
        this.namespaceForm.patchValue({ projectId: response.projects[0].id });
      }
    } catch (error) {
      console.error('Failed to load projects:', error);
      this.toastService.error('Failed to load projects');
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
    setTimeout(() => this.namespaceNameInput()?.nativeElement.focus());
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
        clusterId: this.clusterData.basics.id,
        name: this.namespaceForm.value.name!,
      });

      await firstValueFrom(this.client.createNamespace(request));

      this.showAddNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);
      await this.loadNamespaces(this.clusterData.basics.id);
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

  async deleteNamespace(namespaceId: string, namespaceName: string): Promise<void> {
    if (!confirm(`Are you sure you want to delete namespace '${namespaceName}'?`)) {
      return;
    }

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.client.deleteNamespace(request));

      this.toastService.info(`Namespace '${namespaceName}' deleted`);
      await this.loadNamespaces(this.clusterData.basics.id);
    } catch (error) {
      console.error('Failed to delete namespace:', error);
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

  // Load installed plugins for the cluster
  async loadInstalledPlugins(clusterId: string): Promise<void> {
    try {
      this.isLoadingPlugins.set(true);

      // Fetch installs and all available plugins in parallel
      const [installsResponse, pluginsResponse] = await Promise.all([
        firstValueFrom(this.client.listInstalls(create(ListInstallsRequestSchema, { clusterId }))),
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
      ]);

      // Get the IDs of installed plugins
      const installedPluginIds = installsResponse.installs.map((install) => install.pluginId);

      // Filter the plugins to only include installed ones
      const installed = pluginsResponse.plugins.filter((plugin) =>
        installedPluginIds.includes(plugin.id),
      );

      this.installedPlugins.set(installed);
    } catch (error) {
      console.error('Failed to load installed plugins:', error);
      this.toastService.error('Failed to load installed plugins');
    } finally {
      this.isLoadingPlugins.set(false);
    }
  }
}
