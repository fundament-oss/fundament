import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnDestroy,
  OnInit,
  signal,
  TemplateRef,
  untracked,
  viewChild,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import SecondaryNavService from '../shell/secondary-nav.service';
import opensElsewhere from '../shared/opens-elsewhere';
import { ConfigService } from '../config.service';
import PlatformApiService from './platform-api.service';
import {
  capacityText,
  clusterTag,
  driftKindTag,
  formatCores,
  formatDate,
  formatMemory,
  nodesText,
  observed,
  poolFlag,
  timeAgo,
} from './fleet-format';
import {
  clusterPath,
  findingPath,
  organizationPath,
  platformPath,
  platformViewOf,
  type PlatformView,
} from './platform-views';

/** How often the page reads the fleet again while it is open. The snapshots
 *  themselves change once per collection interval. */
const REFRESH_INTERVAL_MS = 60_000;

/**
 * The Platform section (FUN-23): every organization's clusters as Fundament
 * staff see them. Counts come from the snapshots cluster-worker stores, so
 * each number carries when it was observed; drift is what cluster-worker's
 * reconcile loop found the database and Gardener to disagree on.
 *
 * Read-only, all of it: what to change about a cluster is the organization's
 * call, made in their console.
 */
@Component({
  selector: 'app-platform',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [NgTemplateOutlet],
  templateUrl: './platform.html',
})
export default class PlatformComponent implements OnInit, OnDestroy {
  protected readonly api = inject(PlatformApiService);

  private readonly secondaryNav = inject(SecondaryNavService);

  private readonly router = inject(Router);

  private readonly route = inject(ActivatedRoute);

  private readonly navTemplate = viewChild<TemplateRef<unknown>>('secondaryNav');

  private readonly params = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  protected readonly configured = !!inject(ConfigService).getConfig().platformApiUrl;

  protected readonly view = computed(() => platformViewOf(this.params()));

  /** What "ago" is measured against; moves on every refresh. */
  protected readonly now = signal(new Date());

  private refreshTimer?: ReturnType<typeof setInterval>;

  protected readonly clusterTag = clusterTag;

  protected readonly driftKindTag = driftKindTag;

  protected readonly formatCores = formatCores;

  protected readonly formatMemory = formatMemory;

  protected readonly poolFlag = poolFlag;

  protected readonly date = formatDate;

  protected readonly nodesText = nodesText;

  protected readonly capacityText = capacityText;

  protected readonly organizationPath = organizationPath;

  protected readonly clusterPath = clusterPath;

  protected readonly findingPath = findingPath;

  protected readonly overviewPath = platformPath({ kind: 'overview' });

  protected readonly driftPath = platformPath({ kind: 'drift' });

  protected readonly title = computed(() => {
    const view = this.view();
    switch (view.kind) {
      case 'overview':
        return 'Platform';
      case 'drift':
        return 'Drift';
      case 'organization':
        return this.api.organization()?.alias || 'Organization';
      case 'cluster':
        return this.api.cluster()?.cluster?.name || 'Cluster';
      default: {
        const unhandled: never = view;
        throw new Error(`unhandled platform view ${JSON.stringify(unhandled)}`);
      }
    }
  });

  /** The tiles above the overview's table. */
  protected readonly tiles = computed(() => {
    const s = this.api.summary();
    if (!s) return [];
    return [
      { label: 'Organizations', value: String(s.organizations) },
      { label: 'Clusters', value: String(s.clusters) },
      { label: 'Unhealthy', value: String(s.clustersUnhealthy) },
      { label: 'Updating', value: String(s.clustersUpdating) },
      { label: 'Nodes ready', value: `${s.nodesReady} / ${s.nodes}` },
      { label: 'CPU allocatable', value: formatCores(s.cpuAllocatableM) },
      { label: 'Memory allocatable', value: formatMemory(s.memoryAllocatableMi) },
      { label: 'Drift findings', value: String(s.driftFindings) },
      { label: 'Stale snapshots', value: String(s.staleSnapshots) },
    ];
  });

  /** How current the findings and snapshots are: cluster-worker's loops
   *  record each pass they complete. */
  protected readonly freshness = computed(() => {
    const summary = this.api.summary();
    const reconcile = summary?.reconcile?.lastPass;
    const inventory = summary?.inventory?.lastPass;
    const drift = reconcile
      ? `Drift checked ${timeAgo(timestampDate(reconcile), this.now())}`
      : 'Drift not checked yet';
    const snapshots = inventory
      ? `snapshots collected ${timeAgo(timestampDate(inventory), this.now())}`
      : 'no snapshots collected yet';
    return `${drift}, ${snapshots}`;
  });

  /** Namespaces under their project, projects without any left out. */
  protected readonly namespaceGroups = computed(() =>
    (this.api.cluster()?.projects ?? []).filter((project) => project.namespaces.length > 0),
  );

  constructor() {
    effect(() => {
      const template = this.navTemplate();
      if (template) this.secondaryNav.set(template);
    });

    // A parameter change on the same page: load what the new view shows.
    effect(() => {
      const view = this.view();
      untracked(() => this.loadView(view));
    });
  }

  ngOnInit(): void {
    if (!this.configured) return;
    this.api.loadFleet();
    this.refreshTimer = setInterval(() => this.refresh(), REFRESH_INTERVAL_MS);
  }

  ngOnDestroy(): void {
    clearInterval(this.refreshTimer);
    const template = this.navTemplate();
    if (template) this.secondaryNav.clear(template);
  }

  protected refresh(): void {
    this.now.set(new Date());
    this.api.loadFleet();
    this.loadView(this.view());
  }

  private loadView(view: PlatformView): void {
    if (!this.configured) return;
    this.now.set(new Date());
    switch (view.kind) {
      case 'organization':
        this.api.loadOrganization(view.id);
        break;
      case 'cluster':
        this.api.loadCluster(view.id);
        break;
      case 'overview':
      case 'drift':
        break;
      default: {
        const unhandled: never = view;
        throw new Error(`unhandled platform view ${JSON.stringify(unhandled)}`);
      }
    }
  }

  protected isCurrent(path: string): boolean {
    return platformPath(this.view()) === path;
  }

  /** An organization's entry stays marked while one of its clusters is open. */
  protected isOrganizationCurrent(id: string): boolean {
    const view = this.view();
    if (view.kind === 'organization') return view.id === id;
    if (view.kind === 'cluster') return this.api.cluster()?.cluster?.organizationId === id;
    return false;
  }

  /** Routes in-app; a click that asks for another tab or window is left to the browser. */
  protected go(event: Event, path: string): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.router.navigateByUrl(path);
  }

  protected goToOverview(): void {
    this.router.navigateByUrl(this.overviewPath);
  }

  protected observed(at: Timestamp | undefined) {
    return observed(at, this.api.staleAfterSeconds(), this.now());
  }

  protected ago(at: Timestamp | undefined): string {
    return at ? timeAgo(timestampDate(at), this.now()) : '';
  }
}
