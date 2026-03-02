import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';

@Component({
  selector: 'app-project-limits',
  imports: [FormsModule],
  templateUrl: './project-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectLimitsComponent {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  // Gardener cluster limits (project-scoped; node pool limits are org-level only)
  maxNodesPerCluster = signal<number>(50);

  clusterSaving = signal(false);

  // Kubernetes namespace resource defaults
  defaultMemoryRequestMi = signal<number>(128);

  defaultMemoryLimitMi = signal<number>(256);

  defaultCpuRequestM = signal<number>(100);

  defaultCpuLimitM = signal<number>(500);

  namespaceSaving = signal(false);

  constructor() {
    this.titleService.setTitle('Limits');
  }

  async saveClusterLimits() {
    this.clusterSaving.set(true);
    await new Promise<void>((resolve) => { setTimeout(resolve, 600); });
    this.clusterSaving.set(false);
    this.toastService.success('Cluster limits saved');
  }

  async saveNamespaceLimits() {
    this.namespaceSaving.set(true);
    await new Promise<void>((resolve) => { setTimeout(resolve, 600); });
    this.namespaceSaving.set(false);
    this.toastService.success('Namespace defaults saved');
  }
}
