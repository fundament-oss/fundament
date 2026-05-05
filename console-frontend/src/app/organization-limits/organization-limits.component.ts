import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';

import {
  GetOrganizationLimitsRequestSchema,
  UpdateOrganizationLimitsRequestSchema,
} from '../../generated/v1/organization_pb';
import { ORGANIZATION } from '../../connect/tokens';
import OrganizationContextService from '../organization-context.service';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';

function toInt(value: unknown): number | undefined {
  const n = Math.trunc(Number(value));
  return n > 0 ? n : undefined;
}

@Component({
  selector: 'app-organization-limits',
  imports: [],
  templateUrl: './organization-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class OrganizationLimitsComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private organizationClient = inject(ORGANIZATION);

  private organizationContextService = inject(OrganizationContextService);

  initialLoading = signal(true);

  // Gardener cluster limits
  maxNodesPerCluster = signal<number | undefined>(undefined);

  maxNodePools = signal<number | undefined>(undefined);

  maxNodesPerNodePool = signal<number | undefined>(undefined);

  clusterSaving = signal(false);

  private savedCluster = signal<{
    maxNodesPerCluster: number | undefined;
    maxNodePools: number | undefined;
    maxNodesPerNodePool: number | undefined;
  }>({ maxNodesPerCluster: undefined, maxNodePools: undefined, maxNodesPerNodePool: undefined });

  // Kubernetes namespace resource defaults
  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  namespaceSaving = signal(false);

  private savedNamespace = signal<{
    defaultMemoryRequestMi: number | undefined;
    defaultMemoryLimitMi: number | undefined;
    defaultCpuRequestM: number | undefined;
    defaultCpuLimitM: number | undefined;
  }>({
    defaultMemoryRequestMi: undefined,
    defaultMemoryLimitMi: undefined,
    defaultCpuRequestM: undefined,
    defaultCpuLimitM: undefined,
  });

  protected readonly toInt = toInt;

  constructor() {
    this.titleService.setTitle('Limits');
  }

  async ngOnInit() {
    const orgId = this.organizationContextService.currentOrganizationId();
    if (!orgId) return;

    try {
      const response = await firstValueFrom(
        this.organizationClient.getOrganizationLimits(
          create(GetOrganizationLimitsRequestSchema, { id: orgId }),
        ),
      );
      const limits = response.limits;
      if (limits) {
        const maxNodesPerCluster =
          limits.maxNodesPerCluster > 0 ? limits.maxNodesPerCluster : undefined;
        const maxNodePools =
          limits.maxNodePoolsPerCluster > 0 ? limits.maxNodePoolsPerCluster : undefined;
        const maxNodesPerNodePool =
          limits.maxNodesPerNodePool > 0 ? limits.maxNodesPerNodePool : undefined;
        const defaultMemoryRequestMi =
          limits.defaultMemoryRequestMi > 0 ? limits.defaultMemoryRequestMi : undefined;
        const defaultMemoryLimitMi =
          limits.defaultMemoryLimitMi > 0 ? limits.defaultMemoryLimitMi : undefined;
        const defaultCpuRequestM =
          limits.defaultCpuRequestM > 0 ? limits.defaultCpuRequestM : undefined;
        const defaultCpuLimitM = limits.defaultCpuLimitM > 0 ? limits.defaultCpuLimitM : undefined;

        this.maxNodesPerCluster.set(maxNodesPerCluster);
        this.maxNodePools.set(maxNodePools);
        this.maxNodesPerNodePool.set(maxNodesPerNodePool);
        this.defaultMemoryRequestMi.set(defaultMemoryRequestMi);
        this.defaultMemoryLimitMi.set(defaultMemoryLimitMi);
        this.defaultCpuRequestM.set(defaultCpuRequestM);
        this.defaultCpuLimitM.set(defaultCpuLimitM);

        this.savedCluster.set({ maxNodesPerCluster, maxNodePools, maxNodesPerNodePool });
        this.savedNamespace.set({
          defaultMemoryRequestMi,
          defaultMemoryLimitMi,
          defaultCpuRequestM,
          defaultCpuLimitM,
        });
      }
    } catch {
      this.toastService.error('Failed to load organization limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  async saveClusterLimits() {
    const orgId = this.organizationContextService.currentOrganizationId();
    if (!orgId) return;

    const maxNodesPerCluster = this.maxNodesPerCluster();
    const maxNodePools = this.maxNodePools();
    const maxNodesPerNodePool = this.maxNodesPerNodePool();

    this.clusterSaving.set(true);
    try {
      await firstValueFrom(
        this.organizationClient.updateOrganizationLimits(
          create(UpdateOrganizationLimitsRequestSchema, {
            id: orgId,
            maxNodesPerCluster,
            maxNodePoolsPerCluster: maxNodePools,
            maxNodesPerNodePool,
            ...this.savedNamespace(),
          }),
        ),
      );
      this.savedCluster.set({ maxNodesPerCluster, maxNodePools, maxNodesPerNodePool });
      this.toastService.success('Cluster limits saved');
    } catch {
      this.toastService.error('Failed to save cluster limits');
    } finally {
      this.clusterSaving.set(false);
    }
  }

  async saveNamespaceLimits() {
    const orgId = this.organizationContextService.currentOrganizationId();
    if (!orgId) return;

    const defaultMemoryRequestMi = this.defaultMemoryRequestMi();
    const defaultMemoryLimitMi = this.defaultMemoryLimitMi();
    const defaultCpuRequestM = this.defaultCpuRequestM();
    const defaultCpuLimitM = this.defaultCpuLimitM();

    this.namespaceSaving.set(true);
    try {
      const cluster = this.savedCluster();
      await firstValueFrom(
        this.organizationClient.updateOrganizationLimits(
          create(UpdateOrganizationLimitsRequestSchema, {
            id: orgId,
            maxNodesPerCluster: cluster.maxNodesPerCluster,
            maxNodePoolsPerCluster: cluster.maxNodePools,
            maxNodesPerNodePool: cluster.maxNodesPerNodePool,
            defaultMemoryRequestMi,
            defaultMemoryLimitMi,
            defaultCpuRequestM,
            defaultCpuLimitM,
          }),
        ),
      );
      this.savedNamespace.set({
        defaultMemoryRequestMi,
        defaultMemoryLimitMi,
        defaultCpuRequestM,
        defaultCpuLimitM,
      });
      this.toastService.success('Namespace defaults saved');
    } catch {
      this.toastService.error('Failed to save namespace defaults');
    } finally {
      this.namespaceSaving.set(false);
    }
  }
}
