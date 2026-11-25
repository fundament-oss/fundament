import { timestampDate } from '@bufbuild/protobuf/wkt';
import type {
  GetMetalSnapshotResponse,
  MetalSample,
  MetalSeries,
} from '../../generated/v1/metal_pb';
import type {
  ChartSeries,
  ComponentStatus,
  Histogram,
  IssueSeverity,
  Machine,
  MachineIssue,
  MachineRole,
  MachineState,
  MetalMetrics,
  Network,
  PartitionCapacity,
  PowerState,
  Switch,
} from './metal-metrics';
import { formatAxisTime } from './metal-format';

// dcim-api answers a fixed set of PromQL queries by name (dcim-api/pkg/metal,
// reader.go) and leaves the joins to this file. The names below are that
// contract; the label names are the exporter's.

/** The exporter's placeholder for a label a machine without allocation has. */
const NOT_ALLOCATED = 'NOTALLOCATED';

/** The range steps dcim-api queries at: 24 h at 5 min, 7 d at 1 h. */
const DAY_STEP_MS = 5 * 60_000;
const WEEK_STEP_MS = 60 * 60_000;

type Response = GetMetalSnapshotResponse;

function samplesOf(res: Response, name: string): MetalSample[] {
  return res.results[name]?.samples ?? [];
}

function seriesOf(res: Response, name: string): MetalSeries[] {
  return res.results[name]?.series ?? [];
}

/** One value per key label, e.g. power usage per machineid. */
function valueBy(samples: MetalSample[], key: string): Map<string, number> {
  return new Map(samples.map((sample) => [sample.labels[key] ?? '', sample.value]));
}

/** For queries that select several metrics at once: value per key, per metric
 *  name. */
function valuesByMetric(samples: MetalSample[], key: string): Map<string, Map<string, number>> {
  const out = new Map<string, Map<string, number>>();
  samples.forEach((sample) => {
    const metric = sample.labels['__name__'] ?? '';
    const byKey = out.get(metric) ?? new Map<string, number>();
    byKey.set(sample.labels[key] ?? '', sample.value);
    out.set(metric, byKey);
  });
  return out;
}

function allocatedOrEmpty(value: string | undefined): string {
  return value === undefined || value === NOT_ALLOCATED ? '' : value;
}

/** The v2 condition states, upper-cased by the exporter. A condition without a
 *  state arrives empty. */
export function machineStateOf(label: string | undefined): MachineState {
  switch (label) {
    case 'AVAILABLE':
      return 'available';
    case 'TAINTED':
      return 'tainted';
    case 'LOCKED':
      return 'locked';
    default:
      return 'other';
  }
}

function machineRoleOf(label: string | undefined): MachineRole {
  switch (label) {
    case 'machine':
      return 'machine';
    case 'firewall':
      return 'firewall';
    default:
      return 'none';
  }
}

/** The BMC's power state: 1 on, 0 off, -1 anything else. */
function powerStateOf(value: number | undefined): PowerState | null {
  if (value === undefined) return null;
  if (value === 1) return 'on';
  if (value === 0) return 'off';
  return 'unknown';
}

function severityOf(label: string | undefined): IssueSeverity {
  switch (label) {
    case 'critical':
      return 'critical';
    case 'major':
      return 'major';
    default:
      return 'minor';
  }
}

const compare = (a: string, b: string): number => a.localeCompare(b);

function partitionsOf(res: Response): PartitionCapacity[] {
  const rows = new Map<string, PartitionCapacity>();
  const fields: Record<string, keyof PartitionCapacity> = {
    metal_partition_capacity_total: 'total',
    metal_partition_capacity_allocated: 'allocated',
    metal_partition_capacity_allocatable: 'allocatable',
    metal_partition_capacity_waiting: 'waiting',
    metal_partition_capacity_phoned_home: 'phonedHome',
    metal_partition_capacity_faulty: 'faulty',
    metal_partition_capacity_unavailable: 'unavailable',
    metal_partition_capacity_other: 'other',
    metal_partition_capacity_reservations_total: 'reservationsTotal',
    metal_partition_capacity_reservations_used: 'reservationsUsed',
  };
  samplesOf(res, 'capacity').forEach((sample) => {
    const field = fields[sample.labels['__name__'] ?? ''];
    if (!field) return;
    const partition = sample.labels['partition'] ?? '';
    const size = sample.labels['size'] ?? '';
    const key = `${partition}\u0000${size}`;
    const row = rows.get(key) ?? {
      partition,
      size,
      total: 0,
      allocated: 0,
      allocatable: 0,
      waiting: 0,
      phonedHome: 0,
      faulty: 0,
      unavailable: 0,
      other: 0,
      reservationsTotal: 0,
      reservationsUsed: 0,
    };
    rows.set(key, { ...row, [field]: sample.value });
  });
  return [...rows.values()].sort(
    (a, b) => compare(a.partition, b.partition) || compare(a.size, b.size),
  );
}

function machinesOf(res: Response): Machine[] {
  const sizes = new Map(
    samplesOf(res, 'machine_size').map((s) => [
      s.labels['machineid'] ?? '',
      s.labels['size'] ?? '',
    ]),
  );
  const power = valuesByMetric(samplesOf(res, 'machine_power'), 'machineid');
  const usage = power.get('metal_machine_power_usage');
  const state = power.get('metal_machine_power_state');
  const psuTotal = power.get('metal_machine_power_supplies_total');
  const psuHealthy = power.get('metal_machine_power_supplies_healthy');

  const issueInfo = new Map(
    samplesOf(res, 'machine_issue_info').map((s) => [s.labels['issueid'] ?? '', s.labels]),
  );
  const issues = new Map<string, MachineIssue[]>();
  samplesOf(res, 'machine_issues').forEach((sample) => {
    const machineId = sample.labels['machineid'] ?? '';
    const issueId = sample.labels['issueid'] ?? '';
    const info = issueInfo.get(issueId) ?? {};
    const list = issues.get(machineId) ?? [];
    list.push({
      id: issueId,
      severity: severityOf(info['severity']),
      description: info['description'] ?? '',
      refUrl: info['refurl'] ?? '',
    });
    issues.set(machineId, list);
  });

  return samplesOf(res, 'machine_allocation')
    .map((sample): Machine => {
      const labels = sample.labels;
      const id = labels['machineid'] ?? '';
      return {
        id,
        name: allocatedOrEmpty(labels['machinename']),
        partition: labels['partition'] ?? '',
        size: sizes.get(id) ?? '',
        state: machineStateOf(labels['state']),
        role: machineRoleOf(labels['role']),
        clusterTag: allocatedOrEmpty(labels['clusterTag']),
        imageId: allocatedOrEmpty(labels['imageId']),
        powerWatts: usage?.get(id) ?? null,
        powerState: powerStateOf(state?.get(id)),
        psuTotal: psuTotal?.get(id) ?? 0,
        psuHealthy: psuHealthy?.get(id) ?? 0,
        issues: (issues.get(id) ?? []).sort((a, b) => compare(a.id, b.id)),
      };
    })
    .sort(
      (a, b) =>
        compare(a.partition, b.partition) ||
        compare(a.size, b.size) ||
        compare(a.name, b.name) ||
        compare(a.id, b.id),
    );
}

function networksOf(res: Response): Network[] {
  const usage = valuesByMetric(samplesOf(res, 'network_usage'), 'networkId');
  const value = (metric: string, id: string): number => usage.get(metric)?.get(id) ?? 0;
  return samplesOf(res, 'network_info')
    .map((sample): Network => {
      const id = sample.labels['networkId'] ?? '';
      return {
        id,
        name: sample.labels['name'] || id,
        partition: sample.labels['partition'] ?? '',
        ipsUsed: value('metal_network_ip_used', id),
        ipsAvailable: value('metal_network_ip_available', id),
        prefixesUsed: value('metal_network_prefix_used', id),
        prefixesAvailable: value('metal_network_prefix_available', id),
      };
    })
    .sort((a, b) => compare(a.partition, b.partition) || compare(a.name, b.name));
}

function switchesOf(res: Response): Switch[] {
  const state = valuesByMetric(samplesOf(res, 'switch_state'), 'switchname');
  const coreUp = state.get('metal_switch_metal_core_up');
  const syncFailed = state.get('metal_switch_sync_failed');
  const bgpTotal = valueBy(samplesOf(res, 'switch_bgp_total'), 'switchname');
  const bgpEstablished = valueBy(samplesOf(res, 'switch_bgp_established'), 'switchname');
  return samplesOf(res, 'switch_info')
    .map((sample): Switch => {
      const labels = sample.labels;
      const name = labels['switchname'] ?? '';
      return {
        name,
        partition: labels['partition'] ?? '',
        rack: labels['rackid'] ?? '',
        metalCoreUp: coreUp?.get(name) === 1,
        syncFailing: syncFailed?.get(name) === 1,
        os: [labels['osVendor'], labels['osVersion']].filter(Boolean).join(' '),
        metalCoreVersion: labels['metalCoreVersion'] ?? '',
        bgpEstablished: bgpEstablished.get(name) ?? 0,
        bgpTotal: bgpTotal.get(name) ?? 0,
      };
    })
    .sort((a, b) => compare(a.name, b.name));
}

function componentsOf(res: Response): ComponentStatus[] {
  const token = valueBy(samplesOf(res, 'component_token'), 'uuid');
  return samplesOf(res, 'component_up')
    .map((sample): ComponentStatus => {
      const id = sample.labels['uuid'] ?? '';
      return {
        id,
        type: sample.labels['type'] ?? '',
        identifier: sample.labels['identifier'] ?? '',
        up: sample.value === 1,
        tokenLifetimeSeconds: token.get(id) ?? null,
      };
    })
    .sort((a, b) => compare(a.type, b.type) || compare(a.identifier, b.identifier));
}

/**
 * Range results on one shared axis. The store leaves out the points it has no
 * sample for, so two series of one query need not line up: the axis is every
 * time any of them has, and a series without a point there gets NaN, a gap.
 */
function aligned(series: MetalSeries[], stepMs: number): { labels: string[]; values: number[][] } {
  const times = new Set<number>();
  const perSeries = series.map((item) => {
    const byTime = new Map<number, number>();
    item.points.forEach((point) => {
      if (!point.time) return;
      const at = timestampDate(point.time).getTime();
      times.add(at);
      byTime.set(at, point.value);
    });
    return byTime;
  });
  const axis = [...times].sort((a, b) => a - b);
  return {
    labels: axis.map((at) => formatAxisTime(new Date(at), stepMs)),
    values: perSeries.map((byTime) => axis.map((at) => byTime.get(at) ?? Number.NaN)),
  };
}

function namedSeries(
  series: MetalSeries[],
  label: string,
  stepMs: number,
): { labels: string[]; series: ChartSeries[] } {
  const sorted = [...series].sort((a, b) => compare(a.labels[label] ?? '', b.labels[label] ?? ''));
  const { labels, values } = aligned(sorted, stepMs);
  return {
    labels,
    series: sorted.map((item, index) => ({
      label: item.labels[label] ?? '',
      values: values[index],
    })),
  };
}

/** A bucket bound as Prometheus writes it: a number or `+Inf`. */
function parseLe(le: string | undefined): number {
  if (le === '+Inf') return Infinity;
  return Number(le ?? Number.NaN);
}

function histogramOf(res: Response, prefix: string): Histogram {
  const buckets = samplesOf(res, `${prefix}_buckets`)
    .map((sample) => ({ le: parseLe(sample.labels['le']), count: sample.value }))
    .filter((bucket) => !Number.isNaN(bucket.le))
    .sort((a, b) => a.le - b.le);
  return {
    buckets,
    sum: samplesOf(res, `${prefix}_sum`)[0]?.value ?? 0,
    count: samplesOf(res, `${prefix}_count`)[0]?.value ?? 0,
  };
}

function coreErrorsOf(res: Response): MetalMetrics['coreErrors'] {
  const samples = samplesOf(res, 'core_errors');
  if (samples.length === 0) return undefined;
  const sorted = [...samples].sort((a, b) => b.value - a.value);
  return {
    labels: sorted.map((sample) => sample.labels['operation'] ?? ''),
    values: sorted.map((sample) => Math.round(sample.value)),
  };
}

/** The section's snapshot from dcim-api's answer. */
export default function metalFromProto(res: Response): MetalMetrics {
  const power = aligned(seriesOf(res, 'power'), DAY_STEP_MS);
  const allocation = aligned(
    [...seriesOf(res, 'allocation_allocated'), ...seriesOf(res, 'allocation_total')],
    WEEK_STEP_MS,
  );
  const allocatedCount = seriesOf(res, 'allocation_allocated').length;
  const sync = namedSeries(seriesOf(res, 'sync_durations'), 'switchname', DAY_STEP_MS);
  const requests = namedSeries(seriesOf(res, 'request_rate'), 'class', DAY_STEP_MS);

  return {
    collectedAt: res.collectedAt ? timestampDate(res.collectedAt) : new Date(0),
    partitions: partitionsOf(res),
    machines: machinesOf(res),
    networks: networksOf(res),
    switches: switchesOf(res),
    components: componentsOf(res),
    power: { labels: power.labels, values: power.values[0] ?? [] },
    allocation: {
      labels: allocation.labels,
      allocated: allocatedCount > 0 ? allocation.values[0] : [],
      total: allocation.values[allocatedCount] ?? [],
    },
    syncDurations: { labels: sync.labels, switches: sync.series },
    coreErrors: coreErrorsOf(res),
    requestRate: requests,
    restLatency: histogramOf(res, 'rest_latency'),
    grpcLatency: histogramOf(res, 'grpc_latency'),
  };
}
