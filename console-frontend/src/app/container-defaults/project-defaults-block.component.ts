import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
  signal,
  OnInit,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';

import { PROJECT } from '../../connect/tokens';
import {
  GetProjectDefaultsRequestSchema,
  UpdateProjectDefaultsRequestSchema,
} from '../../generated/v1/project_pb';
import { NotificationService } from '../notification.service';
import SheetSyncDirective from '../sheet-sync.directive';
import ResourceDefaultsSectionComponent, {
  CPU_SECTION,
  MEMORY_SECTION,
  modeFor,
  type ResourceMode,
} from '../resource-defaults-section/resource-defaults-section.component';
import ContainerDefaultsSummaryComponent from './container-defaults-summary.component';
import {
  NO_DEFAULTS,
  projectSummary,
  valuesFrom,
  type ContainerDefaultValues,
} from './container-defaults';

import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

/**
 * The project's per-container defaults, as a block on the project General page.
 * A field the project does not set inherits the cluster's value, and a value it
 * does set may only be lower — which the cluster's values, loaded alongside,
 * are there to show and to cap the inputs with.
 */
@Component({
  selector: 'app-project-defaults-block',
  imports: [ContainerDefaultsSummaryComponent, ResourceDefaultsSectionComponent, SheetSyncDirective],
  templateUrl: './project-defaults-block.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ProjectDefaultsBlockComponent implements OnInit {
  projectId = input.required<string>();

  private client = inject(PROJECT);

  private notificationService = inject(NotificationService);

  protected loading = signal(true);

  protected saving = signal(false);

  protected showEdit = signal(false);

  protected saveError = signal<string | null>(null);

  protected loadError = signal<string | null>(null);

  private values = signal<ContainerDefaultValues>(NO_DEFAULTS);

  private clusterValues = signal<ContainerDefaultValues>(NO_DEFAULTS);

  protected sections = computed(() => projectSummary(this.values(), this.clusterValues()));

  protected readonly MEMORY_SECTION = MEMORY_SECTION;

  protected readonly CPU_SECTION = CPU_SECTION;

  /** The cluster's pair, which is both what a field inherits and how far a
   *  custom value may go. */
  protected memoryFromCluster = computed(() => ({
    request: this.clusterValues().memoryRequestMi,
    limit: this.clusterValues().memoryLimitMi,
  }));

  protected cpuFromCluster = computed(() => ({
    request: this.clusterValues().cpuRequestM,
    limit: this.clusterValues().cpuLimitM,
  }));

  protected draftMemoryMode = signal<ResourceMode>('inherit');

  protected draftMemoryRequestMi = signal<number | undefined>(undefined);

  protected draftMemoryLimitMi = signal<number | undefined>(undefined);

  protected draftCpuMode = signal<ResourceMode>('inherit');

  protected draftCpuRequestM = signal<number | undefined>(undefined);

  protected draftCpuLimitM = signal<number | undefined>(undefined);

  async ngOnInit() {
    try {
      const response = await firstValueFrom(
        this.client.getProjectDefaults(
          create(GetProjectDefaultsRequestSchema, { projectId: this.projectId() }),
        ),
      );
      this.values.set(valuesFrom(response.defaults));
      this.clusterValues.set(valuesFrom(response.clusterDefaults));
    } catch {
      this.loadError.set('Failed to load the project defaults.');
    } finally {
      this.loading.set(false);
    }
  }

  protected openEdit(): void {
    const values = this.values();
    this.saveError.set(null);
    this.draftMemoryRequestMi.set(values.memoryRequestMi);
    this.draftMemoryLimitMi.set(values.memoryLimitMi);
    this.draftCpuRequestM.set(values.cpuRequestM);
    this.draftCpuLimitM.set(values.cpuLimitM);
    this.draftMemoryMode.set(
      modeFor('project', values.memoryRequestMi, values.memoryLimitMi, this.memoryFromCluster()),
    );
    this.draftCpuMode.set(
      modeFor('project', values.cpuRequestM, values.cpuLimitM, this.cpuFromCluster()),
    );
    this.showEdit.set(true);
  }

  /** Reset means inherit everything: all four fields go back to unset. */
  protected reset(): void {
    this.draftMemoryMode.set('inherit');
    this.draftMemoryRequestMi.set(undefined);
    this.draftMemoryLimitMi.set(undefined);
    this.draftCpuMode.set('inherit');
    this.draftCpuRequestM.set(undefined);
    this.draftCpuLimitM.set(undefined);
  }

  protected async save(event?: Event) {
    event?.preventDefault();
    if (this.saving()) return;

    const next: ContainerDefaultValues = {
      memoryRequestMi: this.draftMemoryRequestMi(),
      memoryLimitMi: this.draftMemoryLimitMi(),
      cpuRequestM: this.draftCpuRequestM(),
      cpuLimitM: this.draftCpuLimitM(),
    };

    this.saving.set(true);
    this.saveError.set(null);
    try {
      await firstValueFrom(
        this.client.updateProjectDefaults(
          create(UpdateProjectDefaultsRequestSchema, {
            projectId: this.projectId(),
            defaults: next,
          }),
        ),
      );
      this.values.set(next);
      this.showEdit.set(false);
      this.notificationService.success('Project defaults saved');
    } catch (err) {
      // An InvalidArgument says which value is too high and what the cluster
      // allows, so the API's own message is what belongs here.
      this.saveError.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.saving.set(false);
    }
  }
}
