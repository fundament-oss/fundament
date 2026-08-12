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
import { Router } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { NewClusterFormStateService } from '../new-cluster-form/new-cluster-form-state.service';
import { OrganizationDataService } from '../organization-data.service';
import { RegionCatalogService } from '../region-catalog.service';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { CLUSTER } from '../../connect/tokens';
import {
  CreateClusterRequestSchema,
  CreateNodePoolRequestSchema,
} from '../../generated/v1/cluster_pb';
import focusFirstModalInput from '../modal-focus';
import PageNavService from '../page-nav.service';

@Component({
  selector: 'app-new-cluster-summary',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './new-cluster-summary.component.html',
})
export default class NewClusterSummaryComponent {
  private pageNav = inject(PageNavService);


  private router = inject(Router);

  private client = inject(CLUSTER);

  protected stateService = inject(NewClusterFormStateService);

  private organizationDataService = inject(OrganizationDataService);

  private regionCatalog = inject(RegionCatalogService);

  protected state = computed(() => this.stateService.getState());

  /** Label per machine type name, e.g. "e2-standard-4 (4 lCPU, 16 GiB RAM)".
   *  Empty until the catalog answers. */
  private machineTypeLabels = signal<Record<string, string>>({});

  /** The summary repeats what you picked, and what you picked was a machine with
   *  those specs next to it. Dropping them here would make you go back a step to
   *  check what "e2-standard-4" buys you. Falls back to the bare name, which is
   *  what the pool carries and what the request sends. */
  protected machineTypeLabel(name: string): string {
    return this.machineTypeLabels()[name] || name;
  }

  private async loadMachineTypeLabels() {
    const regionName = this.state().region;
    if (!regionName) return;
    try {
      const region = await this.regionCatalog.getRegionByName(regionName);
      if (!region) return;
      const labels: Record<string, string> = {};
      RegionCatalogService.machineTypeOptions(region).forEach((option) => {
        labels[option.value] = option.label;
      });
      this.machineTypeLabels.set(labels);
    } catch {
      // The bare name still says which machine it is; only the specs go missing.
    }
  }

  protected errorMessage = signal<string | null>(null);

  protected isCreating = signal<boolean>(false);

  protected clusterId = signal<string | null>(null);

  private idempotency = createIdempotencyRef();

  constructor() {
    this.loadMachineTypeLabels();
  }

  /**
   * The cluster is the thing being made here. Once the request comes back, it
   * exists, and where it is in its provisioning is something the cluster page
   * already reports minute by minute. So this closes and hands over instead of
   * mirroring that in a dialog of its own. Fails the request, then nothing was
   * made, and you stay on the summary you can still correct.
   */
  async onCreateCluster() {
    if (this.isCreating()) return;

    const formState = this.state();
    if (!formState.clusterName || !formState.region || !formState.kubernetesVersion) {
      this.errorMessage.set('Missing required cluster information');
      return;
    }

    this.errorMessage.set(null);
    this.isCreating.set(true);

    let clusterId: string;
    try {
      const request = create(CreateClusterRequestSchema, {
        name: formState.clusterName,
        region: formState.region,
        kubernetesVersion: formState.kubernetesVersion,
      });
      const response = await withIdempotency((opts) => this.client.createCluster(request, opts), {
        signal: this.idempotency.reset(),
      });
      clusterId = response.clusterId;
    } catch (error) {
      this.errorMessage.set(
        error instanceof Error ? error.message : 'The cluster could not be created.',
      );
      this.isCreating.set(false);
      return;
    }

    this.clusterId.set(clusterId);
    this.organizationDataService.addCluster(clusterId, formState.clusterName);

    const notCreated = await this.createNodePools(clusterId, formState.nodePools ?? []);

    this.stateService.reset();
    this.isCreating.set(false);

    // The cluster is there either way, so the road leads to it. A pool that did
    // not make it travels along as navigation state: the cluster page says it
    // where the missing pool is, next to the section that should have held it,
    // and it stays there to be read instead of sliding away on its own.
    this.router.navigateByUrl(this.pageNav.path(`/clusters/${clusterId}`), {
      state: notCreated.length > 0 ? { nodePoolsNotCreated: notCreated } : undefined,
    });
  }

  /** Returns the names of the pools that did not make it. */
  private async createNodePools(
    clusterId: string,
    pools: { name: string; machineType: string; autoscaleMin: number; autoscaleMax: number }[],
  ): Promise<string[]> {
    if (pools.length === 0) return [];

    const abortSignal = this.idempotency.reset();
    const uitkomsten = await Promise.allSettled(
      pools.map((pool) => {
        const request = create(CreateNodePoolRequestSchema, {
          clusterId,
          name: pool.name,
          machineType: pool.machineType,
          autoscaleMin: pool.autoscaleMin,
          autoscaleMax: pool.autoscaleMax,
        });
        return withIdempotency((opts) => this.client.createNodePool(request, opts), {
          signal: abortSignal,
        });
      }),
    );

    return pools.filter((_, i) => uitkomsten[i].status === 'rejected').map((pool) => pool.name);
  }

  modalDialogRef = viewChild<ElementRef<HTMLElement>>('modalDialog');

  onModalOpen(): void {
    const el = this.modalDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }
}
