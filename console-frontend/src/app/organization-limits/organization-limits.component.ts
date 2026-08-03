import {
  Component,
  inject,
  signal,
  computed,
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
import { positive, toInt } from '../utils/limits';

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

  // A limit that is switched off is stored as undefined, which is how the API
  // encodes "no limit".
  maxNodesPerClusterLimited = signal(false);

  maxNodePoolsLimited = signal(false);

  maxNodesPerNodePoolLimited = signal(false);

  clusterSaving = signal(false);

  private savedCluster = signal<{
    maxNodesPerCluster: number | undefined;
    maxNodePools: number | undefined;
    maxNodesPerNodePool: number | undefined;
  }>({ maxNodesPerCluster: undefined, maxNodePools: undefined, maxNodesPerNodePool: undefined });

  // Platform defaults returned by the API, used by the "Reset to defaults" action.
  private clusterDefaults = signal<{
    maxNodesPerCluster: number | undefined;
    maxNodePools: number | undefined;
    maxNodesPerNodePool: number | undefined;
  }>({ maxNodesPerCluster: undefined, maxNodePools: undefined, maxNodesPerNodePool: undefined });

  // Kubernetes namespace resource defaults
  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  // Request and limit are switched as one pair: a LimitRange without either is
  // no constraint at all.
  memoryLimited = signal(false);

  cpuLimited = signal(false);

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

  private namespaceDefaults = signal<{
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

  // Any save in flight disables every button so a cluster save and a namespace
  // save can never run concurrently and clobber each other's snapshot.
  protected saving = computed(() => this.clusterSaving() || this.namespaceSaving());

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
      const defaults = response.defaults;

      const clusterDefaults = {
        maxNodesPerCluster: positive(defaults?.maxNodesPerCluster),
        maxNodePools: positive(defaults?.maxNodePoolsPerCluster),
        maxNodesPerNodePool: positive(defaults?.maxNodesPerNodePool),
      };
      const namespaceDefaults = {
        defaultMemoryRequestMi: positive(defaults?.defaultMemoryRequestMi),
        defaultMemoryLimitMi: positive(defaults?.defaultMemoryLimitMi),
        defaultCpuRequestM: positive(defaults?.defaultCpuRequestM),
        defaultCpuLimitM: positive(defaults?.defaultCpuLimitM),
      };
      this.clusterDefaults.set(clusterDefaults);
      this.namespaceDefaults.set(namespaceDefaults);

      // What the organization has actually saved (undefined where no override is set).
      const savedCluster = {
        maxNodesPerCluster: positive(limits?.maxNodesPerCluster),
        maxNodePools: positive(limits?.maxNodePoolsPerCluster),
        maxNodesPerNodePool: positive(limits?.maxNodesPerNodePool),
      };
      const savedNamespace = {
        defaultMemoryRequestMi: positive(limits?.defaultMemoryRequestMi),
        defaultMemoryLimitMi: positive(limits?.defaultMemoryLimitMi),
        defaultCpuRequestM: positive(limits?.defaultCpuRequestM),
        defaultCpuLimitM: positive(limits?.defaultCpuLimitM),
      };
      this.savedCluster.set(savedCluster);
      this.savedNamespace.set(savedNamespace);

      // Show only what the organization has actually saved; an empty field means
      // "no limit". Platform defaults are offered via "Reset to defaults", never
      // silently persisted as overrides on save.
      this.maxNodesPerCluster.set(savedCluster.maxNodesPerCluster);
      this.maxNodePools.set(savedCluster.maxNodePools);
      this.maxNodesPerNodePool.set(savedCluster.maxNodesPerNodePool);
      this.defaultMemoryRequestMi.set(savedNamespace.defaultMemoryRequestMi);
      this.defaultMemoryLimitMi.set(savedNamespace.defaultMemoryLimitMi);
      this.defaultCpuRequestM.set(savedNamespace.defaultCpuRequestM);
      this.defaultCpuLimitM.set(savedNamespace.defaultCpuLimitM);
      this.syncClusterToggles();
      this.syncNamespaceToggles();
    } catch {
      this.toastService.error('Failed to load organization limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  async saveClusterLimits(event?: Event) {
    event?.preventDefault();
    if (this.clusterSaving()) return;

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

  // Reset only repopulates the form with the platform defaults; the user still
  // has to click Save to persist them, so a misclick can't silently overwrite
  // the organization's saved overrides.
  resetClusterLimits(): void {
    const defaults = this.clusterDefaults();
    this.maxNodesPerCluster.set(defaults.maxNodesPerCluster);
    this.maxNodePools.set(defaults.maxNodePools);
    this.maxNodesPerNodePool.set(defaults.maxNodesPerNodePool);
    this.syncClusterToggles();
  }

  resetNamespaceLimits(): void {
    const defaults = this.namespaceDefaults();
    this.defaultMemoryRequestMi.set(defaults.defaultMemoryRequestMi);
    this.defaultMemoryLimitMi.set(defaults.defaultMemoryLimitMi);
    this.defaultCpuRequestM.set(defaults.defaultCpuRequestM);
    this.defaultCpuLimitM.set(defaults.defaultCpuLimitM);
    this.syncNamespaceToggles();
  }

  // A switch is on exactly when a value is set, so the form always opens on
  // what is actually stored.
  private syncClusterToggles(): void {
    this.maxNodesPerClusterLimited.set(this.maxNodesPerCluster() !== undefined);
    this.maxNodePoolsLimited.set(this.maxNodePools() !== undefined);
    this.maxNodesPerNodePoolLimited.set(this.maxNodesPerNodePool() !== undefined);
  }

  private syncNamespaceToggles(): void {
    this.memoryLimited.set(
      this.defaultMemoryRequestMi() !== undefined || this.defaultMemoryLimitMi() !== undefined,
    );
    this.cpuLimited.set(
      this.defaultCpuRequestM() !== undefined || this.defaultCpuLimitM() !== undefined,
    );
  }

  toggleMaxNodesPerCluster(limited: boolean): void {
    this.maxNodesPerClusterLimited.set(limited);
    if (!limited) {
      this.maxNodesPerCluster.set(undefined);
    } else if (this.maxNodesPerCluster() === undefined) {
      this.maxNodesPerCluster.set(this.clusterDefaults().maxNodesPerCluster);
    }
  }

  toggleMaxNodePools(limited: boolean): void {
    this.maxNodePoolsLimited.set(limited);
    if (!limited) {
      this.maxNodePools.set(undefined);
    } else if (this.maxNodePools() === undefined) {
      this.maxNodePools.set(this.clusterDefaults().maxNodePools);
    }
  }

  toggleMaxNodesPerNodePool(limited: boolean): void {
    this.maxNodesPerNodePoolLimited.set(limited);
    if (!limited) {
      this.maxNodesPerNodePool.set(undefined);
    } else if (this.maxNodesPerNodePool() === undefined) {
      this.maxNodesPerNodePool.set(this.clusterDefaults().maxNodesPerNodePool);
    }
  }

  toggleMemoryDefaults(limited: boolean): void {
    this.memoryLimited.set(limited);
    const defaults = this.namespaceDefaults();
    if (!limited) {
      this.defaultMemoryRequestMi.set(undefined);
      this.defaultMemoryLimitMi.set(undefined);
      return;
    }
    if (this.defaultMemoryRequestMi() === undefined) {
      this.defaultMemoryRequestMi.set(defaults.defaultMemoryRequestMi);
    }
    if (this.defaultMemoryLimitMi() === undefined) {
      this.defaultMemoryLimitMi.set(defaults.defaultMemoryLimitMi);
    }
  }

  toggleCpuDefaults(limited: boolean): void {
    this.cpuLimited.set(limited);
    const defaults = this.namespaceDefaults();
    if (!limited) {
      this.defaultCpuRequestM.set(undefined);
      this.defaultCpuLimitM.set(undefined);
      return;
    }
    if (this.defaultCpuRequestM() === undefined) {
      this.defaultCpuRequestM.set(defaults.defaultCpuRequestM);
    }
    if (this.defaultCpuLimitM() === undefined) {
      this.defaultCpuLimitM.set(defaults.defaultCpuLimitM);
    }
  }

  async saveNamespaceLimits(event?: Event) {
    event?.preventDefault();
    if (this.namespaceSaving()) return;

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
