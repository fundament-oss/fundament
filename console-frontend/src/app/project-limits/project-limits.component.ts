import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
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

function toInt(value: unknown): number | undefined {
  const n = Math.trunc(Number(value));
  return n > 0 ? n : undefined;
}

// A proto int32 limit is unset when it is 0 (or absent); treat that as "no value".
function positive(value: number | undefined): number | undefined {
  return value && value > 0 ? value : undefined;
}

@Component({
  selector: 'app-project-limits',
  imports: [],
  templateUrl: './project-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
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

  // Platform defaults returned by the API, used to pre-fill empty fields and by "Reset to defaults".
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

  protected readonly toInt = toInt;

  constructor() {
    this.titleService.setTitle('Limits');
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];

    try {
      const response = await firstValueFrom(
        this.projectClient.getProjectLimits(create(GetProjectLimitsRequestSchema, { projectId })),
      );
      const limits = response.limits;
      const defaults = response.defaults;

      const namespaceDefaults = {
        defaultMemoryRequestMi: positive(defaults?.defaultMemoryRequestMi),
        defaultMemoryLimitMi: positive(defaults?.defaultMemoryLimitMi),
        defaultCpuRequestM: positive(defaults?.defaultCpuRequestM),
        defaultCpuLimitM: positive(defaults?.defaultCpuLimitM),
      };
      this.namespaceDefaults.set(namespaceDefaults);

      // Show the saved override where present, otherwise the platform default.
      this.defaultMemoryRequestMi.set(
        positive(limits?.defaultMemoryRequestMi) ?? namespaceDefaults.defaultMemoryRequestMi,
      );
      this.defaultMemoryLimitMi.set(
        positive(limits?.defaultMemoryLimitMi) ?? namespaceDefaults.defaultMemoryLimitMi,
      );
      this.defaultCpuRequestM.set(
        positive(limits?.defaultCpuRequestM) ?? namespaceDefaults.defaultCpuRequestM,
      );
      this.defaultCpuLimitM.set(
        positive(limits?.defaultCpuLimitM) ?? namespaceDefaults.defaultCpuLimitM,
      );
    } catch {
      this.toastService.error('Failed to load project limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  async resetNamespaceLimits(): Promise<void> {
    const defaults = this.namespaceDefaults();
    this.defaultMemoryRequestMi.set(defaults.defaultMemoryRequestMi);
    this.defaultMemoryLimitMi.set(defaults.defaultMemoryLimitMi);
    this.defaultCpuRequestM.set(defaults.defaultCpuRequestM);
    this.defaultCpuLimitM.set(defaults.defaultCpuLimitM);
    await this.save();
  }

  async save(event?: Event) {
    event?.preventDefault();
    if (this.saving()) return;

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
