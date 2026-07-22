import {
  Component,
  inject,
  computed,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { TitleService } from '../title.service';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { OrganizationDataService } from '../organization-data.service';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { CLUSTER } from '../../connect/tokens';
import {
  CreateClusterRequestSchema,
  CreateNodePoolRequestSchema,
} from '../../generated/v1/cluster_pb';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import LoadingIndicatorComponent from '../icons/loading-indicator.component';

interface ProgressItem {
  key: string;
  type: 'cluster' | 'nodepool';
  name: string;
  requestStatus: 'pending' | 'in_progress' | 'succeeded' | 'failed';
  syncStatus: 'none' | 'syncing' | 'synced' | 'failed';
  error?: string;
  shootStatus?: string;
  nodePoolConfig?: {
    name: string;
    machineType: string;
    regionMachineTypeId?: string;
    autoscaleMin: number;
    autoscaleMax: number;
  };
  createdId?: string;
}

@Component({
  selector: 'app-add-cluster-summary',
  imports: [RouterLink, DialogSyncDirective, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-summary.component.html',
})
export default class AddClusterSummaryComponent {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private client = inject(CLUSTER);

  protected stateService = inject(ClusterWizardStateService);

  private organizationDataService = inject(OrganizationDataService);

  protected state = computed(() => this.stateService.getState());

  protected errorMessage = signal<string | null>(null);

  protected isCreating = signal<boolean>(false);

  // Modal state
  protected showModal = signal(false);

  protected progressItems = signal<ProgressItem[]>([]);

  protected clusterId = signal<string | null>(null);

  protected clusterDisplayName = signal<string>('');

  protected hasCreationStarted = computed(() => this.progressItems().length > 0);

  protected bannerState = computed(
    (): 'creating' | 'provisioning' | 'partial' | 'ready' | 'failed' => {
      if (this.isCreating()) return 'creating';

      const items = this.progressItems();
      const clusterItem = items.find((i) => i.key === 'cluster');
      if (clusterItem?.requestStatus === 'failed') return 'failed';

      const hasAnyInProgress = items.some(
        (i) => i.requestStatus === 'in_progress' || i.syncStatus === 'syncing',
      );
      if (hasAnyInProgress) return 'provisioning';

      const hasAnyFailed = items.some(
        (i) => i.requestStatus === 'failed' || i.syncStatus === 'failed',
      );
      if (hasAnyFailed) return 'partial';

      return 'ready';
    },
  );

  private clusterConfig?: {
    name: string;
    region: string;
    regionId?: string;
    kubernetesVersion: string;
    kubernetesVersionId?: string;
  };

  private idempotency = createIdempotencyRef();

  constructor() {
    this.titleService.setTitle('Cluster summary');
  }

  private updateItem(key: string, updates: Partial<ProgressItem>) {
    this.progressItems.update((items) =>
      items.map((item) => (item.key === key ? { ...item, ...updates } : item)),
    );
  }

  async onCreateCluster() {
    const wizardState = this.state();

    // Validate required fields
    if (!wizardState.clusterName || !wizardState.region || !wizardState.kubernetesVersion) {
      this.errorMessage.set('Missing required cluster information');
      return;
    }

    // Save cluster config for retries
    this.clusterConfig = {
      name: wizardState.clusterName,
      region: wizardState.region,
      regionId: wizardState.regionId,
      kubernetesVersion: wizardState.kubernetesVersion,
      kubernetesVersionId: wizardState.kubernetesVersionId,
    };
    this.clusterDisplayName.set(wizardState.clusterName);

    // Clear previous errors and set loading state
    this.errorMessage.set(null);
    this.isCreating.set(true);

    // Build progress items
    const items: ProgressItem[] = [
      {
        key: 'cluster',
        type: 'cluster',
        name: 'Cluster creation',
        requestStatus: 'pending',
        syncStatus: 'none',
      },
    ];

    if (wizardState.nodePools) {
      items.push(
        ...wizardState.nodePools.map((pool) => ({
          key: `nodepool-${pool.name}`,
          type: 'nodepool' as const,
          name: pool.name,
          requestStatus: 'pending' as const,
          syncStatus: 'none' as const,
          nodePoolConfig: pool,
        })),
      );
    }

    this.progressItems.set(items);
    this.showModal.set(true);

    await this.executeCreation();
  }

  private async executeCreation() {
    if (!this.clusterConfig) return;

    // Step 1: Create cluster
    this.updateItem('cluster', { requestStatus: 'in_progress' });

    try {
      const request = create(CreateClusterRequestSchema, {
        name: this.clusterConfig.name,
        region: this.clusterConfig.region,
        kubernetesVersion: this.clusterConfig.kubernetesVersion,
        regionId: this.clusterConfig.regionId ?? '',
        kubernetesVersionId: this.clusterConfig.kubernetesVersionId ?? '',
      });

      const response = await withIdempotency((opts) => this.client.createCluster(request, opts), {
        signal: this.idempotency.reset(),
      });
      this.clusterId.set(response.clusterId);
      this.updateItem('cluster', {
        requestStatus: 'succeeded',
        syncStatus: 'none',
        createdId: response.clusterId,
      });
      this.organizationDataService.addCluster(response.clusterId, this.clusterConfig.name);

      // Reset wizard state since we have the cluster now
      this.stateService.reset();
    } catch (error) {
      this.updateItem('cluster', {
        requestStatus: 'failed',
        error: error instanceof Error ? error.message : 'Failed to create cluster',
      });
      this.isCreating.set(false);
      return;
    }

    const cid = this.clusterId()!;

    // Step 2: Create node pools
    const nodePoolItems = this.progressItems().filter(
      (item) => item.type === 'nodepool' && item.nodePoolConfig,
    );

    const abortSignal = this.idempotency.reset();

    await Promise.allSettled([
      ...nodePoolItems.map((item) =>
        this.createNodePool(item.key, item.nodePoolConfig!, cid, abortSignal),
      ),
    ]);

    // The create requests have returned (HTTP 200); Gardener provisioning
    // continues in the background and is tracked on the cluster details page.
    this.isCreating.set(false);
  }

  private async createNodePool(
    key: string,
    config: {
      name: string;
      machineType: string;
      regionMachineTypeId?: string;
      autoscaleMin: number;
      autoscaleMax: number;
    },
    clusterId?: string,
    abortSignal?: AbortSignal,
  ) {
    const cid = clusterId || this.clusterId();
    if (!cid) return;

    this.updateItem(key, { requestStatus: 'in_progress', error: undefined });

    try {
      const request = create(CreateNodePoolRequestSchema, {
        clusterId: cid,
        name: config.name,
        machineType: config.machineType,
        regionMachineTypeId: config.regionMachineTypeId ?? '',
        autoscaleMin: config.autoscaleMin,
        autoscaleMax: config.autoscaleMax,
      });

      const response = await withIdempotency((opts) => this.client.createNodePool(request, opts), {
        signal: abortSignal,
      });
      this.updateItem(key, {
        requestStatus: 'succeeded',
        syncStatus: 'none',
        createdId: response.nodePoolId,
      });
    } catch (error) {
      this.updateItem(key, {
        requestStatus: 'failed',
        error: error instanceof Error ? error.message : 'Failed to create node pool',
      });
    }
  }

  protected async retryItem(key: string) {
    const item = this.progressItems().find((i) => i.key === key);
    if (!item) return;

    if (item.type === 'cluster') {
      // Reset all items and restart the entire creation
      this.progressItems.update((items) =>
        items.map((i) => ({
          ...i,
          requestStatus: 'pending' as const,
          syncStatus: 'none' as const,
          error: undefined,
          shootStatus: undefined,
          createdId: undefined,
        })),
      );
      this.clusterId.set(null);
      this.isCreating.set(true);
      await this.executeCreation();
    } else if (item.type === 'nodepool' && item.nodePoolConfig) {
      await this.createNodePool(key, item.nodePoolConfig, undefined, this.idempotency.reset());
    }
  }

  protected onModalClose() {
    // Don't allow closing while requests are in progress
    const hasInProgress = this.progressItems().some((i) => i.requestStatus === 'in_progress');
    if (!hasInProgress) {
      this.showModal.set(false);
    }
  }

  protected reopenModal() {
    this.showModal.set(true);
  }

  protected navigateToCluster() {
    const cid = this.clusterId();
    if (cid) {
      this.router.navigate(['/clusters', cid]);
    }
  }

  modalDialogRef = viewChild<ElementRef<HTMLElement>>('modalDialog');

  onModalOpen(): void {
    const el = this.modalDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
