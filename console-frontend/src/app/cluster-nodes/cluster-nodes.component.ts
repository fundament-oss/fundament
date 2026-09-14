import {
  Component,
  inject,
  signal,
  OnInit,
  ViewChild,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { CLUSTER } from '../../connect/tokens';
import SheetSyncDirective from '../sheet-sync.directive';
import {
  ListNodePoolsRequestSchema,
  CreateNodePoolRequestSchema,
  UpdateNodePoolRequestSchema,
  DeleteNodePoolRequestSchema,
  GetClusterRequestSchema,
  NodePool,
} from '../../generated/v1/cluster_pb';
import { MachineTypeOption, RegionCatalogService } from '../region-catalog.service';
import { fetchClusterName } from '../utils/cluster-status';
import PageNavService from '../page-nav.service';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/page';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

/**
 * What the submit did, in one line. One pool by name, because the name is what
 * you were just looking at; more than one by number, because a list of them is
 * not a sentence. A submit that both adds and removes says neither.
 */
function nodePoolChangeText(
  created: NodePoolData[],
  updated: NodePoolData[],
  deleted: NodePool[],
): string {
  const pools = (group: { name: string }[], verb: string) =>
    group.length === 1
      ? `Node pool '${group[0].name}' ${verb}`
      : `${group.length} node pools ${verb}`;

  if (created.length && !updated.length && !deleted.length) return pools(created, 'added');
  if (deleted.length && !created.length && !updated.length) return pools(deleted, 'removed');
  if (updated.length && !created.length && !deleted.length) return pools(updated, 'updated');
  return 'Node pools updated';
}

@Component({
  selector: 'app-cluster-nodes',
  imports: [SharedNodePoolsFormComponent, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-nodes.component.html',
})
export default class ClusterNodesComponent implements OnInit {
  private pageNav = inject(PageNavService);

  @ViewChild(SharedNodePoolsFormComponent) nodePoolsForm!: SharedNodePoolsFormComponent;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  private regionCatalog = inject(RegionCatalogService);

  private clusterId = '';

  private existingNodePools: NodePool[] = [];

  private idempotency = createIdempotencyRef();

  errorMessage = signal<string | null>(null);

  isSubmitting = signal(false);

  isLoading = signal(true);

  initialNodePools = signal<NodePoolData[]>([]);

  clusterName = signal<string | null>(null);

  // Region-scoped machine types when the cluster's region is in the catalog;
  // null keeps the form on its built-in fallback machine-type list.
  machineTypeOptions = signal<MachineTypeOption[] | null>(null);

  constructor() {
    this.titleService.setTitle('Node pools');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
    await Promise.all([
      fetchClusterName(this.client, this.clusterId).then((name) => this.clusterName.set(name)),
      this.loadNodePools(),
      this.loadMachineTypeOptions(),
    ]);
  }

  // Match the cluster's region (stored as the catalog region name) against the
  // catalog; no match (legacy cluster) leaves the form on its fallback list.
  private async loadMachineTypeOptions() {
    try {
      const response = await firstValueFrom(
        this.client.getCluster(create(GetClusterRequestSchema, { clusterId: this.clusterId })),
      );
      const regionName = response.cluster?.region;
      if (!regionName) {
        return;
      }
      const region = await this.regionCatalog.getRegionByName(regionName);
      if (region) {
        this.machineTypeOptions.set(RegionCatalogService.machineTypeOptions(region));
      }
    } catch {
      // Catalog unavailable: the form falls back to its built-in list.
    }
  }

  async loadNodePools() {
    try {
      this.isLoading.set(true);
      const request = create(ListNodePoolsRequestSchema, {
        clusterId: this.clusterId,
      });
      const response = await firstValueFrom(this.client.listNodePools(request));
      this.existingNodePools = response.nodePools;

      // Convert to NodePoolData format for the form
      this.initialNodePools.set(
        response.nodePools.map((pool) => ({
          name: pool.name,
          machineType: pool.machineType,
          autoscaleMin: pool.minNodes,
          autoscaleMax: pool.maxNodes,
        })),
      );
    } catch (error) {
      const message =
        error instanceof Error
          ? `Failed to load node pools: ${error.message}`
          : 'Failed to load node pools';
      this.errorMessage.set(message);
    } finally {
      this.isLoading.set(false);
    }
  }

  async onFormSubmit(data: { nodePools: NodePoolData[] }) {
    if (this.isSubmitting()) return;

    this.errorMessage.set(null);
    this.isSubmitting.set(true);

    try {
      const newPools = data.nodePools;
      const existingPoolsMap = new Map(this.existingNodePools.map((p) => [p.name, p]));
      const newPoolsMap = new Map(newPools.map((p) => [p.name, p]));

      const deleted = this.existingNodePools.filter(
        (existingPool) => !newPoolsMap.has(existingPool.name),
      );
      const created = newPools.filter((newPool) => !existingPoolsMap.has(newPool.name));
      const updated = newPools.filter((newPool) => {
        const existingPool = existingPoolsMap.get(newPool.name);
        return (
          !!existingPool &&
          (existingPool.minNodes !== newPool.autoscaleMin ||
            existingPool.maxNodes !== newPool.autoscaleMax)
        );
      });

      // Delete pools that no longer exist
      await Promise.all(
        deleted.map((existingPool) => {
          const deleteRequest = create(DeleteNodePoolRequestSchema, {
            nodePoolId: existingPool.id,
          });
          return firstValueFrom(this.client.deleteNodePool(deleteRequest));
        }),
      );

      // Create or update pools
      const idempotencySignal = this.idempotency.reset();

      await Promise.all(
        newPools.map((newPool) => {
          const existingPool = existingPoolsMap.get(newPool.name);

          if (existingPool) {
            // Update if values changed
            if (
              existingPool.minNodes !== newPool.autoscaleMin ||
              existingPool.maxNodes !== newPool.autoscaleMax
            ) {
              const updateRequest = create(UpdateNodePoolRequestSchema, {
                nodePoolId: existingPool.id,
                autoscaleMin: newPool.autoscaleMin,
                autoscaleMax: newPool.autoscaleMax,
              });
              return firstValueFrom(this.client.updateNodePool(updateRequest));
            }
          } else {
            // Create new pool
            const createRequest = create(CreateNodePoolRequestSchema, {
              clusterId: this.clusterId,
              name: newPool.name,
              machineType: newPool.machineType,
              autoscaleMin: newPool.autoscaleMin,
              autoscaleMax: newPool.autoscaleMax,
            });
            return withIdempotency((opts) => this.client.createNodePool(createRequest, opts), {
              signal: idempotencySignal,
            });
          }
          return undefined;
        }),
      );

      // The page behind this sheet is where the pools are listed, and it was
      // never unmounted, so it hears from here that its cards are out of date.
      this.organizationData.nodePoolsChanged.update((revision) => revision + 1);
      this.notificationService.success(nodePoolChangeText(created, updated, deleted));

      // Navigate back to cluster overview on success
      this.pageNav.goTo(`/clusters/${this.clusterId}`);
    } catch (error) {
      const message =
        error instanceof Error
          ? `Failed to update cluster nodes: ${error.message}`
          : 'Failed to update cluster nodes';
      this.errorMessage.set(message);
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.pageNav.goTo(`/clusters/${this.clusterId}`);
  }
}
