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
import { positive, toInt } from '../utils/limits';

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

  // Platform defaults returned by the API, used by the "Reset to defaults" action.
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

      // Show only what the project has actually saved; an empty field means "no
      // default set". Platform defaults are offered via "Reset to defaults",
      // never silently persisted as overrides on save.
      this.defaultMemoryRequestMi.set(positive(limits?.defaultMemoryRequestMi));
      this.defaultMemoryLimitMi.set(positive(limits?.defaultMemoryLimitMi));
      this.defaultCpuRequestM.set(positive(limits?.defaultCpuRequestM));
      this.defaultCpuLimitM.set(positive(limits?.defaultCpuLimitM));
    } catch {
      this.toastService.error('Failed to load project limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  // Reset only repopulates the form with the platform defaults; the user still
  // has to click Save to persist them, so a misclick can't silently overwrite
  // the project's saved overrides.
  resetNamespaceLimits(): void {
    const defaults = this.namespaceDefaults();
    this.defaultMemoryRequestMi.set(defaults.defaultMemoryRequestMi);
    this.defaultMemoryLimitMi.set(defaults.defaultMemoryLimitMi);
    this.defaultCpuRequestM.set(defaults.defaultCpuRequestM);
    this.defaultCpuLimitM.set(defaults.defaultCpuLimitM);
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
