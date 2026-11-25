import type {
  ChartSeries,
  Histogram,
  Machine,
  MachineIssue,
  PartitionCapacity,
} from './metal-metrics';

/**
 * A quantile from a classic Prometheus histogram, the way PromQL
 * `histogram_quantile` computes it: find the bucket the rank falls in and
 * interpolate inside it. A rank that lands in the `+Inf` bucket has no upper
 * bound to interpolate towards, so the highest finite bound is the answer.
 */
export function histogramQuantile(q: number, histogram: Histogram): number {
  if (histogram.count <= 0 || histogram.buckets.length === 0) return 0;
  const buckets = [...histogram.buckets].sort((a, b) => a.le - b.le);
  const rank = q * histogram.count;
  const index = buckets.findIndex((bucket) => bucket.count >= rank);
  if (index === -1) {
    return [...buckets].reverse().find((bucket) => Number.isFinite(bucket.le))?.le ?? 0;
  }
  const bucket = buckets[index];
  const lower = index === 0 ? { le: 0, count: 0 } : buckets[index - 1];
  if (!Number.isFinite(bucket.le)) return lower.le;
  const inBucket = bucket.count - lower.count;
  if (inBucket <= 0) return bucket.le;
  return lower.le + ((bucket.le - lower.le) * (rank - lower.count)) / inBucket;
}

/** What a histogram bar chart draws: the count per bucket, not the cumulative
 *  count the exporter publishes. */
export function bucketCounts(histogram: Histogram): number[] {
  let previous = 0;
  return histogram.buckets.map((bucket) => {
    const count = Math.max(0, bucket.count - previous);
    previous = bucket.count;
    return count;
  });
}

/** The bucket bounds as the chart and its table label them. */
export function bucketLabels(histogram: Histogram): string[] {
  return histogram.buckets.map((bucket) =>
    Number.isFinite(bucket.le) ? `≤ ${bucket.le} s` : '+Inf',
  );
}

/** What the seven capacity segments add up to. The exporter does not check
 *  this against `total`, so the page keeps the two apart rather than assuming
 *  one. */
export function capacitySum(partition: PartitionCapacity): number {
  return (
    partition.allocated +
    partition.allocatable +
    partition.waiting +
    partition.phonedHome +
    partition.faulty +
    partition.unavailable +
    partition.other
  );
}

/** The stacked bar's series, in the order the legend and the colours read. */
export function capacitySegments(partitions: PartitionCapacity[]): ChartSeries[] {
  return [
    { label: 'Allocated', values: partitions.map((p) => p.allocated) },
    { label: 'Allocatable', values: partitions.map((p) => p.allocatable) },
    { label: 'Waiting', values: partitions.map((p) => p.waiting) },
    { label: 'Phoned home', values: partitions.map((p) => p.phonedHome) },
    { label: 'Faulty', values: partitions.map((p) => p.faulty) },
    { label: 'Unavailable', values: partitions.map((p) => p.unavailable) },
    { label: 'Other', values: partitions.map((p) => p.other) },
  ];
}

export interface SizeReservations {
  size: string;
  used: number;
  total: number;
}

/** Reservations per machine size, summed over the partitions that offer it. */
export function reservationsBySize(partitions: PartitionCapacity[]): SizeReservations[] {
  const bySize = new Map<string, SizeReservations>();
  partitions.forEach((partition) => {
    const existing = bySize.get(partition.size);
    if (existing) {
      existing.used += partition.reservationsUsed;
      existing.total += partition.reservationsTotal;
    } else {
      bySize.set(partition.size, {
        size: partition.size,
        used: partition.reservationsUsed,
        total: partition.reservationsTotal,
      });
    }
  });
  return [...bySize.values()].sort((a, b) => a.size.localeCompare(b.size));
}

export interface MachineCounts {
  total: number;
  allocated: number;
  poweredOn: number;
  poweredOff: number;
  poweredUnknown: number;
  degradedPsu: number;
  criticalIssues: number;
}

export function machineCounts(machines: Machine[]): MachineCounts {
  return {
    total: machines.length,
    allocated: machines.filter((machine) => machine.clusterTag !== '').length,
    poweredOn: machines.filter((machine) => machine.powerState === 'on').length,
    poweredOff: machines.filter((machine) => machine.powerState === 'off').length,
    // Unknown is a state the BMC reported as neither on nor off, or no state
    // reported at all: either way nobody can tell from here.
    poweredUnknown: machines.filter(
      (machine) => machine.powerState !== 'on' && machine.powerState !== 'off',
    ).length,
    degradedPsu: machines.filter(
      (machine) => machine.psuTotal > 0 && machine.psuHealthy < machine.psuTotal,
    ).length,
    criticalIssues: machines.filter((machine) =>
      machine.issues.some((issue) => issue.severity === 'critical'),
    ).length,
  };
}

/** Total draw of everything the BMC reported, in watts. Machines without a
 *  BMC report contribute nothing rather than a made-up zero. */
export function totalPowerWatts(machines: Machine[]): number {
  return machines.reduce((sum, machine) => sum + (machine.powerWatts ?? 0), 0);
}

export interface BucketedSeries {
  labels: string[];
  values: number[];
}

const POWER_BUCKETS = [200, 300, 400, 500, 600];

/** How many machines draw how much: the shape of the power histogram. */
export function powerHistogram(machines: Machine[]): BucketedSeries {
  const labels = [
    ...POWER_BUCKETS.map((bound, index) => {
      const lower = index === 0 ? 0 : POWER_BUCKETS[index - 1];
      return `${lower} to ${bound} W`;
    }),
    `${POWER_BUCKETS[POWER_BUCKETS.length - 1]} W and up`,
  ];
  const values = labels.map(() => 0);
  machines.forEach((machine) => {
    const watts = machine.powerWatts;
    if (watts === null) return;
    const index = POWER_BUCKETS.findIndex((bound) => watts < bound);
    const bucket = index === -1 ? values.length - 1 : index;
    values[bucket] += 1;
  });
  return { labels, values };
}

export interface IssueGroup {
  id: string;
  description: string;
  severity: MachineIssue['severity'];
  refUrl: string;
  count: number;
}

/** The issues present in the estate, grouped by issue id and worst first. */
export function issueGroups(machines: Machine[]): IssueGroup[] {
  const byId = new Map<string, IssueGroup>();
  machines.forEach((machine) => {
    machine.issues.forEach((issue) => {
      const group = byId.get(issue.id);
      if (group) {
        group.count += 1;
      } else {
        byId.set(issue.id, {
          id: issue.id,
          description: issue.description,
          severity: issue.severity,
          refUrl: issue.refUrl,
          count: 1,
        });
      }
    });
  });
  const severityOrder: Record<IssueGroup['severity'], number> = {
    critical: 0,
    major: 1,
    minor: 2,
  };
  return [...byId.values()].sort(
    (a, b) =>
      severityOrder[a.severity] - severityOrder[b.severity] ||
      b.count - a.count ||
      a.id.localeCompare(b.id),
  );
}
