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
import PageNavService from '../page-nav.service';
import SheetSyncDirective from '../sheet-sync.directive';
import MockBadgeComponent from '../mock-badge/mock-badge.component';
import OrganizationContextService from '../organization-context.service';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import { positive } from '../utils/limits';
import ResourceLimitSectionComponent, {
  modeFor,
  MEMORY_SECTION,
  CPU_SECTION,
  type ResourceMode,
} from '../resource-limit-section/resource-limit-section.component';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
/** Platform defaults for a namespace LimitRange, as returned by the API. */
interface NamespaceDefaults {
  defaultMemoryRequestMi: number | undefined;
  defaultMemoryLimitMi: number | undefined;
  defaultCpuRequestM: number | undefined;
  defaultCpuLimitM: number | undefined;
}

/** Where a section's values come from, for the page that only shows them. */
const stateText = (mode: ResourceMode): string =>
  mode === 'defaults' ? "The platform's defaults." : "This organization's own values.";

/** An unlimited pair has no number, and the row says that where the number
 *  would have been. */
const valueText = (value: number | null, unit: string): string =>
  value === null ? 'Unlimited' : `${value} ${unit}`;

@Component({
  selector: 'app-organization-limits',
  imports: [ResourceLimitSectionComponent, SheetSyncDirective, MockBadgeComponent],
  templateUrl: './organization-limits.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class OrganizationLimitsComponent implements OnInit {
  private titleService = inject(TitleService);

  private notificationService = inject(NotificationService);

  private organizationClient = inject(ORGANIZATION);

  private organizationContextService = inject(OrganizationContextService);

  initialLoading = signal(true);

  showNamespaceEdit = signal(false);

  /** A failed save, kept in view in the sheet the changes are in. */
  namespaceError = signal<string | null>(null);

  /** The sheet edits a copy: closing it must leave the page showing what is
   *  stored, not what somebody typed and abandoned. */
  draftMemoryMode = signal<ResourceMode>('unlimited');

  draftMemoryRequestMi = signal<number | undefined>(undefined);

  draftMemoryLimitMi = signal<number | undefined>(undefined);

  draftCpuMode = signal<ResourceMode>('unlimited');

  draftCpuRequestM = signal<number | undefined>(undefined);

  draftCpuLimitM = signal<number | undefined>(undefined);

  stateText = stateText;

  valueText = valueText;

  /** What the page shows for the namespace defaults, per section. */
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

  // Kubernetes namespace resource defaults
  defaultMemoryRequestMi = signal<number | undefined>(undefined);

  defaultMemoryLimitMi = signal<number | undefined>(undefined);

  defaultCpuRequestM = signal<number | undefined>(undefined);

  defaultCpuLimitM = signal<number | undefined>(undefined);

  // Owned here rather than in the fields component so a load or a reset can put
  // the switches back on what is actually stored.
  memoryMode = signal<ResourceMode>('unlimited');

  cpuMode = signal<ResourceMode>('unlimited');

  namespaceSaving = signal(false);

  protected namespaceDefaults = signal<NamespaceDefaults>({
    defaultMemoryRequestMi: undefined,
    defaultMemoryLimitMi: undefined,
    defaultCpuRequestM: undefined,
    defaultCpuLimitM: undefined,
  });

  protected pageNav = inject(PageNavService);

  openNamespaceEdit(): void {
    this.namespaceError.set(null);
    this.draftMemoryMode.set(this.memoryMode());
    this.draftMemoryRequestMi.set(this.defaultMemoryRequestMi());
    this.draftMemoryLimitMi.set(this.defaultMemoryLimitMi());
    this.draftCpuMode.set(this.cpuMode());
    this.draftCpuRequestM.set(this.defaultCpuRequestM());
    this.draftCpuLimitM.set(this.defaultCpuLimitM());
    this.showNamespaceEdit.set(true);
  }

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

      const namespaceDefaults = {
        defaultMemoryRequestMi: positive(defaults?.defaultMemoryRequestMi),
        defaultMemoryLimitMi: positive(defaults?.defaultMemoryLimitMi),
        defaultCpuRequestM: positive(defaults?.defaultCpuRequestM),
        defaultCpuLimitM: positive(defaults?.defaultCpuLimitM),
      };
      this.namespaceDefaults.set(namespaceDefaults);

      // What the organization has actually saved (undefined where no override is set).
      const savedNamespace = {
        defaultMemoryRequestMi: positive(limits?.defaultMemoryRequestMi),
        defaultMemoryLimitMi: positive(limits?.defaultMemoryLimitMi),
        defaultCpuRequestM: positive(limits?.defaultCpuRequestM),
        defaultCpuLimitM: positive(limits?.defaultCpuLimitM),
      };
      // Show only what the organization has actually saved; an empty field means
      // "no limit". Platform defaults are offered via "Reset to defaults", never
      // silently persisted as overrides on save.
      this.defaultMemoryRequestMi.set(savedNamespace.defaultMemoryRequestMi);
      this.defaultMemoryLimitMi.set(savedNamespace.defaultMemoryLimitMi);
      this.defaultCpuRequestM.set(savedNamespace.defaultCpuRequestM);
      this.defaultCpuLimitM.set(savedNamespace.defaultCpuLimitM);
      this.syncNamespaceToggles();
    } catch {
      this.notificationService.error('Failed to load organization limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

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

  /** Picking a mode changes the values, so the values decide the mode on load. */
  private syncNamespaceToggles(): void {
    this.memoryMode.set(
      modeFor(this.defaultMemoryRequestMi(), this.defaultMemoryLimitMi(), this.memorySeed()),
    );
    this.cpuMode.set(modeFor(this.defaultCpuRequestM(), this.defaultCpuLimitM(), this.cpuSeed()));
  }

  async saveNamespaceLimits(event?: Event) {
    event?.preventDefault();
    if (this.namespaceSaving()) return;

    const orgId = this.organizationContextService.currentOrganizationId();
    if (!orgId) return;

    const defaultMemoryRequestMi = this.draftMemoryRequestMi();
    const defaultMemoryLimitMi = this.draftMemoryLimitMi();
    const defaultCpuRequestM = this.draftCpuRequestM();
    const defaultCpuLimitM = this.draftCpuLimitM();

    this.namespaceSaving.set(true);
    this.namespaceError.set(null);
    try {
      await firstValueFrom(
        this.organizationClient.updateOrganizationLimits(
          create(UpdateOrganizationLimitsRequestSchema, {
            id: orgId,
            defaultMemoryRequestMi,
            defaultMemoryLimitMi,
            defaultCpuRequestM,
            defaultCpuLimitM,
          }),
        ),
      );
      // Only now: what the page shows has to be what the API accepted.
      this.defaultMemoryRequestMi.set(defaultMemoryRequestMi);
      this.defaultMemoryLimitMi.set(defaultMemoryLimitMi);
      this.defaultCpuRequestM.set(defaultCpuRequestM);
      this.defaultCpuLimitM.set(defaultCpuLimitM);
      this.memoryMode.set(this.draftMemoryMode());
      this.cpuMode.set(this.draftCpuMode());
      this.showNamespaceEdit.set(false);
      this.notificationService.success('Namespace defaults saved');
    } catch (err) {
      this.namespaceError.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.namespaceSaving.set(false);
    }
  }
}
