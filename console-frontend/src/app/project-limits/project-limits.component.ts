import {
  Component,
  inject,
  signal,
  computed,
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
import PageNavService from '../page-nav.service';
import SheetSyncDirective from '../sheet-sync.directive';
import { positive } from '../utils/limits';
import ResourceLimitSectionComponent, {
  modeFor,
  MEMORY_SECTION,
  CPU_SECTION,
  type ResourceMode,
} from '../resource-limit-section/resource-limit-section.component';

/** Where a section's values come from, for the page that only shows them. */
const stateText = (mode: ResourceMode): string =>
  mode === 'defaults' ? "The platform's defaults." : "This project's own values.";

/** An unlimited pair has no number, and the row says that where the number
 *  would have been: the two rows stay the same rows whatever the mode. */
const valueText = (value: number | null, unit: string): string =>
  value === null ? 'Unlimited' : `${value} ${unit}`;

/** Platform defaults for a namespace LimitRange, as returned by the API. */
interface NamespaceDefaults {
  defaultMemoryRequestMi: number | undefined;
  defaultMemoryLimitMi: number | undefined;
  defaultCpuRequestM: number | undefined;
  defaultCpuLimitM: number | undefined;
}

@Component({
  selector: 'app-project-limits',
  imports: [ResourceLimitSectionComponent, SheetSyncDirective],
  templateUrl: './project-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ProjectLimitsComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private projectClient = inject(PROJECT);

  private route = inject(ActivatedRoute);

  protected pageNav = inject(PageNavService);

  projectId = signal('');

  initialLoading = signal(true);

  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  // Owned here rather than in the fields component so a load can put the modes
  // back on what is actually stored.
  memoryMode = signal<ResourceMode>('unlimited');

  cpuMode = signal<ResourceMode>('unlimited');

  saving = signal(false);

  showEdit = signal(false);

  /** The sheet edits a copy: closing it must leave the page showing what is
   *  stored, not what somebody typed and abandoned. */
  draftMemoryMode = signal<ResourceMode>('unlimited');

  draftMemoryRequestMi = signal<number | undefined>(undefined);

  draftMemoryLimitMi = signal<number | undefined>(undefined);

  draftCpuMode = signal<ResourceMode>('unlimited');

  draftCpuRequestM = signal<number | undefined>(undefined);

  draftCpuLimitM = signal<number | undefined>(undefined);

  /** A failed save, kept in view: a toast is gone before the reader has decided
   *  what to do, and what is on screen is not what is stored. */
  saveError = signal<string | null>(null);

  // Platform defaults returned by the API, used by the "Reset to defaults" action.
  protected namespaceDefaults = signal<NamespaceDefaults>({
    defaultMemoryRequestMi: undefined,
    defaultMemoryLimitMi: undefined,
    defaultCpuRequestM: undefined,
    defaultCpuLimitM: undefined,
  });

  readonly MEMORY_SECTION = MEMORY_SECTION;

  readonly CPU_SECTION = CPU_SECTION;

  /** The platform pair each section falls back on, split the way a section
   *  wants it. */
  memorySeed = computed(() => ({
    request: this.namespaceDefaults().defaultMemoryRequestMi,
    limit: this.namespaceDefaults().defaultMemoryLimitMi,
  }));

  cpuSeed = computed(() => ({
    request: this.namespaceDefaults().defaultCpuRequestM,
    limit: this.namespaceDefaults().defaultCpuLimitM,
  }));

  /** What the page shows: the stored values, per section. */
  summaries = computed(() => [
    {
      copy: MEMORY_SECTION,
      mode: this.memoryMode(),
      request: this.defaultMemoryRequestMi() ?? null,
      limit: this.defaultMemoryLimitMi() ?? null,
    },
    {
      copy: CPU_SECTION,
      mode: this.cpuMode(),
      request: this.defaultCpuRequestM() ?? null,
      limit: this.defaultCpuLimitM() ?? null,
    },
  ]);

  stateText = stateText;

  valueText = valueText;

  constructor() {
    this.titleService.setTitle('Limits');
  }

  openEdit(): void {
    this.saveError.set(null);
    this.draftMemoryMode.set(this.memoryMode());
    this.draftMemoryRequestMi.set(this.defaultMemoryRequestMi());
    this.draftMemoryLimitMi.set(this.defaultMemoryLimitMi());
    this.draftCpuMode.set(this.cpuMode());
    this.draftCpuRequestM.set(this.defaultCpuRequestM());
    this.draftCpuLimitM.set(this.defaultCpuLimitM());
    this.showEdit.set(true);
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);

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
      this.syncModes();
    } catch {
      this.toastService.error('Failed to load project limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  /** Picking a mode changes the values, so the values decide the mode on load:
   *  no values is unlimited, the platform's values are defaults, anything else
   *  is this project's own. */
  private syncModes(): void {
    this.memoryMode.set(
      modeFor(this.defaultMemoryRequestMi(), this.defaultMemoryLimitMi(), this.memorySeed()),
    );
    this.cpuMode.set(modeFor(this.defaultCpuRequestM(), this.defaultCpuLimitM(), this.cpuSeed()));
  }

  async save(event?: Event) {
    event?.preventDefault();
    if (this.saving()) return;

    const projectId = this.route.snapshot.params['id'];

    this.saving.set(true);
    this.saveError.set(null);
    try {
      await firstValueFrom(
        this.projectClient.updateProjectLimits(
          create(UpdateProjectLimitsRequestSchema, {
            projectId,
            defaultMemoryRequestMi: this.draftMemoryRequestMi(),
            defaultMemoryLimitMi: this.draftMemoryLimitMi(),
            defaultCpuRequestM: this.draftCpuRequestM(),
            defaultCpuLimitM: this.draftCpuLimitM(),
          }),
        ),
      );
      // Only now: what the page shows has to be what the API accepted.
      this.defaultMemoryRequestMi.set(this.draftMemoryRequestMi());
      this.defaultMemoryLimitMi.set(this.draftMemoryLimitMi());
      this.defaultCpuRequestM.set(this.draftCpuRequestM());
      this.defaultCpuLimitM.set(this.draftCpuLimitM());
      this.memoryMode.set(this.draftMemoryMode());
      this.cpuMode.set(this.draftCpuMode());
      this.showEdit.set(false);
      this.toastService.success('Namespace defaults saved');
    } catch (err) {
      this.saveError.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.saving.set(false);
    }
  }
}
