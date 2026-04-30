import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerGauge } from '@ng-icons/tabler-icons';
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

@Component({
  selector: 'app-organization-limits',
  imports: [FormsModule, NgIcon],
  viewProviders: [provideIcons({ tablerGauge })],
  templateUrl: './organization-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
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

  // Kubernetes namespace resource defaults
  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  namespaceSaving = signal(false);

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
        if (limits.maxNodesPerCluster > 0) this.maxNodesPerCluster.set(limits.maxNodesPerCluster);
        if (limits.maxNodePoolsPerCluster > 0) this.maxNodePools.set(limits.maxNodePoolsPerCluster);
        if (limits.maxNodesPerNodePool > 0) this.maxNodesPerNodePool.set(limits.maxNodesPerNodePool);
        if (limits.defaultMemoryRequestMi > 0) this.defaultMemoryRequestMi.set(limits.defaultMemoryRequestMi);
        if (limits.defaultMemoryLimitMi > 0) this.defaultMemoryLimitMi.set(limits.defaultMemoryLimitMi);
        if (limits.defaultCpuRequestM > 0) this.defaultCpuRequestM.set(limits.defaultCpuRequestM);
        if (limits.defaultCpuLimitM > 0) this.defaultCpuLimitM.set(limits.defaultCpuLimitM);
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

    this.clusterSaving.set(true);
    try {
      await firstValueFrom(this.organizationClient.updateOrganizationLimits(this.buildUpdateRequest(orgId)));
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

    this.namespaceSaving.set(true);
    try {
      await firstValueFrom(this.organizationClient.updateOrganizationLimits(this.buildUpdateRequest(orgId)));
      this.toastService.success('Namespace defaults saved');
    } catch {
      this.toastService.error('Failed to save namespace defaults');
    } finally {
      this.namespaceSaving.set(false);
    }
  }

  protected toInt(value: unknown): number | undefined {
    const n = Math.trunc(Number(value));
    return n > 0 ? n : undefined;
  }

  private buildUpdateRequest(orgId: string) {
    return create(UpdateOrganizationLimitsRequestSchema, {
      id: orgId,
      maxNodesPerCluster: this.maxNodesPerCluster(),
      maxNodePoolsPerCluster: this.maxNodePools(),
      maxNodesPerNodePool: this.maxNodesPerNodePool(),
      defaultMemoryRequestMi: this.defaultMemoryRequestMi(),
      defaultMemoryLimitMi: this.defaultMemoryLimitMi(),
      defaultCpuRequestM: this.defaultCpuRequestM(),
      defaultCpuLimitM: this.defaultCpuLimitM(),
    });
  }
}
