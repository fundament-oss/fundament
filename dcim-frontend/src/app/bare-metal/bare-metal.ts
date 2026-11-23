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
import { clusterPath } from '../platform/platform-views';
import MetalMetricsService from './metal-metrics.service';
import { bareMetalPath, bareMetalViewOf } from './bare-metal-views';
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
  errorLabels: string[];
  errorSeries: ChartSeries[];
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
 * one snapshot, read through the MetalMetricsService seam. The real app has no
 * Prometheus read yet and says so; the demo fills the snapshot.
 */
@Component({
  selector: 'app-bare-metal',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [NgTemplateOutlet, MetricChartComponent],
  templateUrl: './bare-metal.html',
})
export default class BareMetalComponent implements OnDestroy {
  protected readonly metrics = inject(MetalMetricsService).snapshot;

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
      errorLabels: snapshot.coreErrors.labels,
      errorSeries: [{ label: 'Errors', values: snapshot.coreErrors.values }],
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

  protected readonly clusterPath = clusterPath;

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
    effect(() => {
      const template = this.navTemplate();
      if (template) this.secondaryNav.set(template);
    });
  }

  ngOnDestroy(): void {
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

  protected goToCapacity(): void {
    this.router.navigateByUrl(this.paths.capacity);
  }
}
