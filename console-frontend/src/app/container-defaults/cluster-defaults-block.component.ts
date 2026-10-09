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

import { CLUSTER } from '../../connect/tokens';
import {
  GetClusterDefaultsRequestSchema,
  UpdateClusterDefaultsRequestSchema,
} from '../../generated/v1/cluster_pb';
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
  clusterSummary,
  NO_DEFAULTS,
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
 * The cluster's per-container defaults, as a block on the cluster detail page:
 * a read-only summary that opens one sheet for both resources, because they are
 * saved together anyway.
 */
@Component({
  selector: 'app-cluster-defaults-block',
  imports: [ContainerDefaultsSummaryComponent, ResourceDefaultsSectionComponent, SheetSyncDirective],
  templateUrl: './cluster-defaults-block.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ClusterDefaultsBlockComponent implements OnInit {
  clusterId = input.required<string>();

  private client = inject(CLUSTER);

  private notificationService = inject(NotificationService);

  protected loading = signal(true);

  protected saving = signal(false);

  protected showEdit = signal(false);

  /** A failed save, kept in view in the sheet the changes are in: a
   *  notification is gone before the reader has decided what to do. */
  protected saveError = signal<string | null>(null);

  protected loadError = signal<string | null>(null);

  private values = signal<ContainerDefaultValues>(NO_DEFAULTS);

  /** The platform's suggestion, offered as a starting point. */
  private suggested = signal<ContainerDefaultValues>(NO_DEFAULTS);

  protected sections = computed(() => clusterSummary(this.values()));

  protected readonly MEMORY_SECTION = MEMORY_SECTION;

  protected readonly CPU_SECTION = CPU_SECTION;

  protected memorySeed = computed(() => ({
    request: this.suggested().memoryRequestMi,
    limit: this.suggested().memoryLimitMi,
  }));

  protected cpuSeed = computed(() => ({
    request: this.suggested().cpuRequestM,
    limit: this.suggested().cpuLimitM,
  }));

  /** The sheet edits a copy: closing it must leave the block showing what is
   *  stored, not what somebody typed and abandoned. */
  protected draftMemoryMode = signal<ResourceMode>('none');

  protected draftMemoryRequestMi = signal<number | undefined>(undefined);

  protected draftMemoryLimitMi = signal<number | undefined>(undefined);

  protected draftCpuMode = signal<ResourceMode>('none');

  protected draftCpuRequestM = signal<number | undefined>(undefined);

  protected draftCpuLimitM = signal<number | undefined>(undefined);

  async ngOnInit() {
    try {
      const response = await firstValueFrom(
        this.client.getClusterDefaults(
          create(GetClusterDefaultsRequestSchema, { clusterId: this.clusterId() }),
        ),
      );
      this.values.set(valuesFrom(response.defaults));
      this.suggested.set(valuesFrom(response.suggested));
    } catch {
      this.loadError.set('Failed to load the cluster defaults.');
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
    // Picking a mode changes the values, so the values decide the mode.
    this.draftMemoryMode.set(
      modeFor('cluster', values.memoryRequestMi, values.memoryLimitMi, this.memorySeed()),
    );
    this.draftCpuMode.set(
      modeFor('cluster', values.cpuRequestM, values.cpuLimitM, this.cpuSeed()),
    );
    this.showEdit.set(true);
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
        this.client.updateClusterDefaults(
          create(UpdateClusterDefaultsRequestSchema, {
            clusterId: this.clusterId(),
            defaults: next,
          }),
        ),
      );
      // Only now: what the block shows has to be what the API accepted.
      this.values.set(next);
      this.showEdit.set(false);
      this.notificationService.success('Cluster defaults saved');
    } catch (err) {
      // A FailedPrecondition names the project to lower first, so the API's own
      // message is more use here than anything this page could write.
      this.saveError.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.saving.set(false);
    }
  }
}
