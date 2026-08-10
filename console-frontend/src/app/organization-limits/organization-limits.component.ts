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
import OrganizationContextService from '../organization-context.service';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import { positive, toInt } from '../utils/limits';
import ResourceLimitSectionComponent, {
  modeFor,
  MEMORY_SECTION,
  CPU_SECTION,
  type ResourceMode,
} from '../resource-limit-section/resource-limit-section.component';

/** Platform defaults for a namespace LimitRange, as returned by the API. */
interface NamespaceDefaults {
  defaultMemoryRequestMi: number | undefined;
  defaultMemoryLimitMi: number | undefined;
  defaultCpuRequestM: number | undefined;
  defaultCpuLimitM: number | undefined;
}

/** The three caps, and the words that go with them. */
type ClusterKey = 'maxNodesPerCluster' | 'maxNodePools' | 'maxNodesPerNodePool';

interface ClusterLimits {
  maxNodesPerCluster: number | null;
  maxNodePools: number | null;
  maxNodesPerNodePool: number | null;
}

const CLUSTER_FIELDS: {
  key: ClusterKey;
  name: string;
  title: string;
  description: string;
  label: string;
}[] = [
  {
    key: 'maxNodesPerCluster',
    name: 'maxNodesPerClusterLimited',
    title: 'Nodes per cluster',
    description: 'Caps the total number of nodes across all node pools in a shoot cluster.',
    label: 'Max nodes per cluster',
  },
  {
    key: 'maxNodePools',
    name: 'maxNodePoolsLimited',
    title: 'Node pools per cluster',
    description: 'Caps how many node pools can be configured per shoot cluster.',
    label: 'Max node pools per cluster',
  },
  {
    key: 'maxNodesPerNodePool',
    name: 'maxNodesPerNodePoolLimited',
    title: 'Nodes per node pool',
    description:
      'Caps how many nodes a single node pool may hold, including the autoscaler maximum.',
    label: 'Max nodes per node pool',
  },
];

/**
 * The same three states as a namespace section, for a single number: no cap, the
 * platform's number, or one of this organization's own. Derived rather than
 * stored, so typing the platform's number back lands on "defaults" by itself.
 */
const modeForValue = (value: number | null, seed: number | undefined): ResourceMode => {
  if (value === null) return 'unlimited';
  return value === seed ? 'defaults' : 'custom';
};

/** Where a section's values come from, for the page that only shows them. */
const stateText = (mode: ResourceMode): string =>
  mode === 'defaults' ? "The platform's defaults." : "This organization's own values.";

/** An unlimited pair has no number, and the row says that where the number
 *  would have been. */
const valueText = (value: number | null, unit: string): string =>
  value === null ? 'Unlimited' : `${value} ${unit}`;

@Component({
  selector: 'app-organization-limits',
  imports: [ResourceLimitSectionComponent, SheetSyncDirective],
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

  // Gardener cluster limits
  maxNodesPerCluster = signal<number | undefined>(undefined);

  maxNodePools = signal<number | undefined>(undefined);

  maxNodesPerNodePool = signal<number | undefined>(undefined);

  clusterSaving = signal(false);

  showClusterEdit = signal(false);

  showNamespaceEdit = signal(false);

  /** A failed save, kept in view in the sheet the changes are in. */
  clusterError = signal<string | null>(null);

  namespaceError = signal<string | null>(null);

  /** The sheets edit a copy: closing one must leave the page showing what is
   *  stored, not what somebody typed and abandoned. Null is "no limit", which is
   *  how the API encodes it. */
  draftCluster = signal<ClusterLimits>({
    maxNodesPerCluster: null,
    maxNodePools: null,
    maxNodesPerNodePool: null,
  });

  draftMemoryMode = signal<ResourceMode>('unlimited');

  draftMemoryRequestMi = signal<number | undefined>(undefined);

  draftMemoryLimitMi = signal<number | undefined>(undefined);

  draftCpuMode = signal<ResourceMode>('unlimited');

  draftCpuRequestM = signal<number | undefined>(undefined);

  draftCpuLimitM = signal<number | undefined>(undefined);

  readonly clusterFields = CLUSTER_FIELDS;

  stateText = stateText;

  valueText = valueText;

  /** What the page shows for the caps. */
  clusterRows = computed(() =>
    CLUSTER_FIELDS.map((field) => ({
      label: field.title,
      value: this.savedCluster()[field.key] ?? null,
    })),
  );

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

  // Owned here rather than in the fields component so a load or a reset can put
  // the switches back on what is actually stored.
  memoryMode = signal<ResourceMode>('unlimited');

  cpuMode = signal<ResourceMode>('unlimited');

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

  protected namespaceDefaults = signal<NamespaceDefaults>({
    defaultMemoryRequestMi: undefined,
    defaultMemoryLimitMi: undefined,
    defaultCpuRequestM: undefined,
    defaultCpuLimitM: undefined,
  });

  // Any save in flight disables every button so a cluster save and a namespace
  // save can never run concurrently and clobber each other's snapshot.
  protected saving = computed(() => this.clusterSaving() || this.namespaceSaving());

  protected readonly toInt = toInt;

  protected pageNav = inject(PageNavService);

  openClusterEdit(): void {
    this.clusterError.set(null);
    const saved = this.savedCluster();
    this.draftCluster.set({
      maxNodesPerCluster: saved.maxNodesPerCluster ?? null,
      maxNodePools: saved.maxNodePools ?? null,
      maxNodesPerNodePool: saved.maxNodesPerNodePool ?? null,
    });
    this.showClusterEdit.set(true);
  }

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

  /** What a cap is on, read from what it holds. */
  clusterMode(key: ClusterKey): ResourceMode {
    return modeForValue(this.draftCluster()[key], this.clusterDefaults()[key]);
  }

  /**
   * Unlimited clears the cap, which is how the API reads "no limit"; defaults
   * writes the platform's number; custom keeps what is there and only fills an
   * empty field.
   */
  setClusterMode(key: ClusterKey, mode: ResourceMode): void {
    const seed = this.clusterDefaults()[key] ?? null;
    this.draftCluster.update((draft) => {
      if (mode === 'unlimited') return { ...draft, [key]: null };
      if (mode === 'defaults') return { ...draft, [key]: seed };
      return { ...draft, [key]: draft[key] ?? seed };
    });
  }

  setClusterValue(key: ClusterKey, value: number | undefined): void {
    this.draftCluster.update((draft) => ({ ...draft, [key]: value ?? null }));
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
      this.syncNamespaceToggles();
    } catch {
      this.notificationService.error('Failed to load organization limits');
    } finally {
      this.initialLoading.set(false);
    }
  }

  async saveClusterLimits(event?: Event) {
    event?.preventDefault();
    if (this.clusterSaving()) return;

    const orgId = this.organizationContextService.currentOrganizationId();
    if (!orgId) return;

    const draft = this.draftCluster();
    const maxNodesPerCluster = draft.maxNodesPerCluster ?? undefined;
    const maxNodePools = draft.maxNodePools ?? undefined;
    const maxNodesPerNodePool = draft.maxNodesPerNodePool ?? undefined;

    this.clusterSaving.set(true);
    this.clusterError.set(null);
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
      // Only now: what the page shows has to be what the API accepted.
      this.savedCluster.set({ maxNodesPerCluster, maxNodePools, maxNodesPerNodePool });
      this.maxNodesPerCluster.set(maxNodesPerCluster);
      this.maxNodePools.set(maxNodePools);
      this.maxNodesPerNodePool.set(maxNodesPerNodePool);
      this.showClusterEdit.set(false);
      this.notificationService.success('Cluster limits saved');
    } catch (err) {
      this.clusterError.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.clusterSaving.set(false);
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
