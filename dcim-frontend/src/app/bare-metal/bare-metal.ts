import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnDestroy,
  TemplateRef,
  viewChild,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import SecondaryNavService from '../shell/secondary-nav.service';
import opensElsewhere from '../shared/opens-elsewhere';
import MetalMetricsService from './metal-metrics.service';
import { BARE_METAL_PATH, bareMetalPath, bareMetalViewOf } from './bare-metal-views';
import {
  bucketCounts,
  bucketLabels,
  capacitySegments,
  histogramQuantile,
  issueGroups,
  machineCounts,
  powerHistogram,
  reservationsBySize,
  totalPowerWatts,
  type IssueGroup,
  type SizeReservations,
} from './metal-derive';
import type {
  ChartSeries,
  ComponentStatus,
  Histogram,
  Machine,
  Network,
  PartitionCapacity,
  Switch,
} from './metal-metrics';
import {
  bgpText,
  componentTag,
  componentType,
  formatCount,
  formatKilowatts,
  formatMilliseconds,
  formatRate,
  formatSeconds,
  formatTimestamp,
  formatWatts,
  ipText,
  issueSeverityTag,
  machineIssueTag,
  machineName,
  machineRole,
  machineStateTag,
  powerText,
  prefixText,
  psuText,
  syncTag,
  tokenTag,
  tokenText,
} from './metal-format';
import MetricChartComponent from './metric-chart';

interface Tile {
  label: string;
  value: string;
}

interface CapacityView {
  tiles: Tile[];
  chartLabels: string[];
  chartSeries: ChartSeries[];
  reservations: SizeReservations[];
  allocationLabels: string[];
  allocationSeries: ChartSeries[];
}

interface MachinesView {
  tiles: Tile[];
  powerLabels: string[];
  powerSeries: ChartSeries[];
  powerHistogramLabels: string[];
  powerHistogramSeries: ChartSeries[];
  issues: IssueGroup[];
  machines: Machine[];
}

interface NetworkView {
  networks: Network[];
  switches: Switch[];
  syncLabels: string[];
  syncSeries: ChartSeries[];
  /** Null while nothing collects metal-core errors. */
  errors: { labels: string[]; series: ChartSeries[] } | null;
}

interface ApiView {
  requestLabels: string[];
  requestSeries: ChartSeries[];
  restPercentiles: Tile[];
  grpcPercentiles: Tile[];
  restBucketLabels: string[];
  restBucketSeries: ChartSeries[];
  grpcBucketLabels: string[];
  grpcBucketSeries: ChartSeries[];
  components: ComponentStatus[];
}

/** How often the page reads the snapshot again while it is open. dcim-api
 *  answers from a snapshot of up to 30 s old, and the series change every
 *  minute or two. */
const REFRESH_INTERVAL_MS = 60_000;

/** The quantiles a latency histogram is read for. */
function percentileTiles(histogram: Histogram): Tile[] {
  return [
    { label: 'p50', value: formatSeconds(histogramQuantile(0.5, histogram)) },
    { label: 'p95', value: formatSeconds(histogramQuantile(0.95, histogram)) },
    { label: 'p99', value: formatSeconds(histogramQuantile(0.99, histogram)) },
  ];
}

/**
 * The Bare metal section: what metal-stack says about the estate. Four views of
 * one snapshot, read through MetalMetricsService. An environment without
 * metal-stack metrics says so; the demo fills the snapshot from fixtures.
 */
@Component({
  selector: 'app-bare-metal',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [NgTemplateOutlet, MetricChartComponent],
  templateUrl: './bare-metal.html',
})
export default class BareMetalComponent implements OnDestroy {
  private readonly metalMetrics = inject(MetalMetricsService);

  protected readonly metrics = this.metalMetrics.snapshot;

  protected readonly status = this.metalMetrics.status;

  /** Why the snapshot on screen is not the latest, or null while it is. A
   *  failed read keeps the last snapshot, so the page still has something to
   *  show; this says it may be out of date. */
  protected readonly staleReason = computed(() => {
    const status = this.status();
    switch (status) {
      case 'loading':
      case 'ok':
        return null;
      case 'not-configured':
        return 'This environment no longer has metal-stack metrics configured.';
      case 'no-data':
        return 'The store no longer holds metal-stack series.';
      case 'unavailable':
        return 'metal-stack metrics are unreachable right now.';
      default: {
        const unhandled: never = status;
        throw new Error(`unhandled metal status ${JSON.stringify(unhandled)}`);
      }
    }
  });

  private refreshTimer?: ReturnType<typeof setInterval>;

  private readonly secondaryNav = inject(SecondaryNavService);

  private readonly router = inject(Router);

  private readonly route = inject(ActivatedRoute);

  private readonly navTemplate = viewChild<TemplateRef<unknown>>('secondaryNav');

  private readonly params = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  protected readonly view = computed(() => bareMetalViewOf(this.params()));

  protected readonly paths = {
    capacity: bareMetalPath({ kind: 'capacity' }),
    machines: bareMetalPath({ kind: 'machines' }),
    network: bareMetalPath({ kind: 'network' }),
    api: bareMetalPath({ kind: 'api' }),
  };

  protected readonly title = computed(() => {
    const view = this.view();
    switch (view.kind) {
      case 'capacity':
        return 'Capacity';
      case 'machines':
        return 'Machines';
      case 'network':
        return 'Network';
      case 'api':
        return 'API';
      default: {
        const unhandled: never = view;
        throw new Error(`unhandled bare metal view ${JSON.stringify(unhandled)}`);
      }
    }
  });

  protected readonly collectedAt = computed(() => {
    const snapshot = this.metrics();
    return snapshot ? formatTimestamp(snapshot.collectedAt) : '';
  });

  protected readonly capacityView = computed<CapacityView | null>(() => {
    const snapshot = this.metrics();
    if (!snapshot) return null;
    const sum = (pick: (partition: PartitionCapacity) => number) =>
      snapshot.partitions.reduce((total, partition) => total + pick(partition), 0);
    return {
      tiles: [
        { label: 'Machines', value: String(machineCounts(snapshot.machines).total) },
        { label: 'Allocated', value: String(sum((partition) => partition.allocated)) },
        { label: 'Allocatable', value: String(sum((partition) => partition.allocatable)) },
        {
          label: 'Faulty and unavailable',
          value: String(sum((partition) => partition.faulty + partition.unavailable)),
        },
        {
          label: 'Total power',
          value: formatKilowatts(totalPowerWatts(snapshot.machines)),
        },
      ],
      chartLabels: snapshot.partitions.map(
        (partition) => `${partition.partition} ${partition.size}`,
      ),
      chartSeries: capacitySegments(snapshot.partitions),
      reservations: reservationsBySize(snapshot.partitions),
      allocationLabels: snapshot.allocation.labels,
      allocationSeries: [
        { label: 'Allocated', values: snapshot.allocation.allocated },
        { label: 'Total', values: snapshot.allocation.total },
      ],
    };
  });

  protected readonly machinesView = computed<MachinesView | null>(() => {
    const snapshot = this.metrics();
    if (!snapshot) return null;
    const counts = machineCounts(snapshot.machines);
    const histogram = powerHistogram(snapshot.machines);
    return {
      tiles: [
        { label: 'Powered on', value: String(counts.poweredOn) },
        { label: 'Powered off', value: String(counts.poweredOff) },
        { label: 'Power unknown', value: String(counts.poweredUnknown) },
        { label: 'Degraded PSU', value: String(counts.degradedPsu) },
      ],
      powerLabels: snapshot.power.labels,
      powerSeries: [{ label: 'Power', values: snapshot.power.values }],
      powerHistogramLabels: histogram.labels,
      powerHistogramSeries: [{ label: 'Machines', values: histogram.values }],
      issues: issueGroups(snapshot.machines),
      machines: snapshot.machines,
    };
  });

  protected readonly networkView = computed<NetworkView | null>(() => {
    const snapshot = this.metrics();
    if (!snapshot) return null;
    return {
      networks: snapshot.networks,
      switches: snapshot.switches,
      syncLabels: snapshot.syncDurations.labels,
      syncSeries: snapshot.syncDurations.switches,
      errors: snapshot.coreErrors
        ? {
            labels: snapshot.coreErrors.labels,
            series: [{ label: 'Errors', values: snapshot.coreErrors.values }],
          }
        : null,
    };
  });

  protected readonly apiView = computed<ApiView | null>(() => {
    const snapshot = this.metrics();
    if (!snapshot) return null;
    return {
      requestLabels: snapshot.requestRate.labels,
      requestSeries: snapshot.requestRate.series,
      restPercentiles: percentileTiles(snapshot.restLatency),
      grpcPercentiles: percentileTiles(snapshot.grpcLatency),
      restBucketLabels: bucketLabels(snapshot.restLatency),
      restBucketSeries: [{ label: 'Requests', values: bucketCounts(snapshot.restLatency) }],
      grpcBucketLabels: bucketLabels(snapshot.grpcLatency),
      grpcBucketSeries: [{ label: 'Requests', values: bucketCounts(snapshot.grpcLatency) }],
      components: snapshot.components,
    };
  });

  // Formatters and tags the template reads. Imported function declarations are
  // stable references, so a chart input does not see a new function each tick.
  protected readonly formatCount = formatCount;

  protected readonly formatWatts = formatWatts;

  protected readonly formatMilliseconds = formatMilliseconds;

  protected readonly formatRate = formatRate;

  protected readonly machineStateTag = machineStateTag;

  protected readonly issueSeverityTag = issueSeverityTag;

  protected readonly machineIssueTag = machineIssueTag;

  protected readonly componentTag = componentTag;

  protected readonly syncTag = syncTag;

  protected readonly tokenTag = tokenTag;

  protected readonly machineName = machineName;

  protected readonly machineRole = machineRole;

  protected readonly powerText = powerText;

  protected readonly psuText = psuText;

  protected readonly ipText = ipText;

  protected readonly prefixText = prefixText;

  protected readonly bgpText = bgpText;

  protected readonly tokenText = tokenText;

  protected readonly componentType = componentType;

  constructor() {
    // The shell reads once at start; entering the section reads again, so the
    // page is not as old as the session, and so does every minute after.
    this.metalMetrics.refresh();
    this.refreshTimer = setInterval(() => this.metalMetrics.refresh(), REFRESH_INTERVAL_MS);
    effect(() => {
      const template = this.navTemplate();
      if (template) this.secondaryNav.set(template);
    });
  }

  ngOnDestroy(): void {
    clearInterval(this.refreshTimer);
    const template = this.navTemplate();
    if (template) this.secondaryNav.clear(template);
  }

  protected isCurrent(path: string): boolean {
    return bareMetalPath(this.view()) === path;
  }

  /** Routes in-app; a click that asks for another tab or window is left to the browser. */
  protected go(event: Event, path: string): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.router.navigateByUrl(path);
  }

  /** Back from a view is back to the menu, so the address says so too. */
  protected goToMenu(): void {
    this.router.navigateByUrl(BARE_METAL_PATH);
  }
}
