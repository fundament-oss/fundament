import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerGauge } from '@ng-icons/tabler-icons';
import { ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';

import {
  GetProjectLimitsRequestSchema,
  UpdateProjectLimitsRequestSchema,
} from '../../generated/v1/project_pb';
import { PROJECT } from '../../connect/tokens';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';

@Component({
  selector: 'app-project-limits',
  imports: [FormsModule, NgIcon],
  viewProviders: [provideIcons({ tablerGauge })],
  templateUrl: './project-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectLimitsComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private projectClient = inject(PROJECT);

  private route = inject(ActivatedRoute);

  initialLoading = signal(true);

  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  saving = signal(false);

  constructor() {
    this.titleService.setTitle('Limits');
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];

    try {
      const response = await firstValueFrom(
        this.projectClient.getProjectLimits(
          create(GetProjectLimitsRequestSchema, { projectId }),
        ),
      );
      const limits = response.limits;
      if (limits) {
        if (limits.defaultMemoryRequestMi > 0) this.defaultMemoryRequestMi.set(limits.defaultMemoryRequestMi);
        if (limits.defaultMemoryLimitMi > 0) this.defaultMemoryLimitMi.set(limits.defaultMemoryLimitMi);
        if (limits.defaultCpuRequestM > 0) this.defaultCpuRequestM.set(limits.defaultCpuRequestM);
        if (limits.defaultCpuLimitM > 0) this.defaultCpuLimitM.set(limits.defaultCpuLimitM);
      }
    } catch {
      this.toastService.error('Failed to load project limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  protected toInt(value: unknown): number | undefined {
    const n = Math.trunc(Number(value));
    return n > 0 ? n : undefined;
  }

  async save() {
    const projectId = this.route.snapshot.params['id'];

    this.saving.set(true);
    try {
      await firstValueFrom(
        this.projectClient.updateProjectLimits(
          create(UpdateProjectLimitsRequestSchema, {
            projectId,
            defaultMemoryRequestMi: this.defaultMemoryRequestMi(),
            defaultMemoryLimitMi: this.defaultMemoryLimitMi(),
            defaultCpuRequestM: this.defaultCpuRequestM(),
            defaultCpuLimitM: this.defaultCpuLimitM(),
          }),
        ),
      );
      this.toastService.success('Namespace defaults saved');
    } catch {
      this.toastService.error('Failed to save namespace defaults');
    } finally {
      this.saving.set(false);
    }
  }
}
