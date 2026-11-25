import {
  bucketCounts,
  bucketLabels,
  capacitySegments,
  capacitySum,
  histogramQuantile,
  issueGroups,
  machineCounts,
  powerHistogram,
  reservationsBySize,
  totalPowerWatts,
} from './metal-derive';
import type { Histogram, Machine, PartitionCapacity } from './metal-metrics';

const machine = (overrides: Partial<Machine> = {}): Machine => ({
  id: 'machine-1',
  name: 'ams1-c1-large-01',
  partition: 'ams1',
  size: 'c1-large-x86',
  state: 'available',
  role: 'machine',
  clusterTag: 'cl-utrecht-prod',
  imageId: 'ubuntu-24.04',
  powerWatts: 300,
  powerState: 'on',
  psuTotal: 2,
  psuHealthy: 2,
  issues: [],
  ...overrides,
});

const partition = (overrides: Partial<PartitionCapacity> = {}): PartitionCapacity => ({
  partition: 'ams1',
  size: 'c1-large-x86',
  total: 10,
  allocated: 5,
  allocatable: 2,
  waiting: 1,
  phonedHome: 1,
  faulty: 1,
  unavailable: 0,
  other: 0,
  reservationsTotal: 4,
  reservationsUsed: 2,
  ...overrides,
});

const histogram = (): Histogram => ({
  buckets: [
    { le: 1, count: 50 },
    { le: 2, count: 100 },
    { le: Infinity, count: 100 },
  ],
  sum: 120,
  count: 100,
});

describe('histogramQuantile', () => {
  it('interpolates inside the bucket the rank falls in', () => {
    expect(histogramQuantile(0.5, histogram())).toBe(1);
    expect(histogramQuantile(0.25, histogram())).toBe(0.5);
    expect(histogramQuantile(0.75, histogram())).toBe(1.5);
  });

  it('answers with the highest finite bound when the rank lands in +Inf', () => {
    const withOverflow: Histogram = {
      buckets: [
        { le: 1, count: 50 },
        { le: 2, count: 90 },
        { le: Infinity, count: 100 },
      ],
      sum: 120,
      count: 100,
    };
    expect(histogramQuantile(0.99, withOverflow)).toBe(2);
  });

  it('answers zero for an empty histogram', () => {
    expect(histogramQuantile(0.5, { buckets: [], sum: 0, count: 0 })).toBe(0);
  });
});

describe('bucketCounts', () => {
  it('turns cumulative counts into per-bucket counts', () => {
    expect(bucketCounts(histogram())).toEqual([50, 50, 0]);
  });

  it('labels the bounds and the open end', () => {
    expect(bucketLabels(histogram())).toEqual(['≤ 1 s', '≤ 2 s', '+Inf']);
  });
});

describe('capacity', () => {
  it('adds the segments up', () => {
    expect(capacitySum(partition())).toBe(10);
  });

  it('draws one series per segment, in a fixed order', () => {
    const segments = capacitySegments([partition(), partition({ allocated: 1 })]);
    expect(segments.map((segment) => segment.label)).toEqual([
      'Allocated',
      'Allocatable',
      'Waiting',
      'Phoned home',
      'Faulty',
      'Unavailable',
      'Other',
    ]);
    expect(segments[0].values).toEqual([5, 1]);
  });

  it('sums reservations per size across partitions', () => {
    expect(
      reservationsBySize([
        partition(),
        partition({ partition: 'grn1', reservationsTotal: 2, reservationsUsed: 1 }),
      ]),
    ).toEqual([{ size: 'c1-large-x86', used: 3, total: 6 }]);
  });
});

describe('machineCounts', () => {
  it('counts power, PSU and critical issues', () => {
    const counts = machineCounts([
      machine(),
      machine({ powerState: 'off', psuHealthy: 1 }),
      machine({ powerState: 'unknown', powerWatts: null, psuTotal: 0, psuHealthy: 0 }),
      machine({
        issues: [
          { id: 'failed-machine-reclaim', severity: 'critical', description: '', refUrl: '' },
        ],
      }),
    ]);
    expect(counts).toEqual({
      total: 4,
      allocated: 4,
      poweredOn: 2,
      poweredOff: 1,
      poweredUnknown: 1,
      degradedPsu: 1,
      criticalIssues: 1,
    });
  });

  it('counts a machine whose BMC reported no power state as unknown', () => {
    expect(machineCounts([machine({ powerState: null, powerWatts: null })]).poweredUnknown).toBe(1);
  });

  it('only counts allocated machines as allocated', () => {
    expect(machineCounts([machine({ clusterTag: '' })]).allocated).toBe(0);
  });
});

describe('totalPowerWatts', () => {
  it('leaves machines without a BMC report out', () => {
    expect(totalPowerWatts([machine({ powerWatts: 300 }), machine({ powerWatts: null })])).toBe(
      300,
    );
  });
});

describe('powerHistogram', () => {
  it('buckets the machines that have a reading', () => {
    const bucketed = powerHistogram([
      machine({ powerWatts: 150 }),
      machine({ powerWatts: 250 }),
      machine({ powerWatts: 350 }),
      machine({ powerWatts: 550 }),
      machine({ powerWatts: 700 }),
      machine({ powerWatts: null }),
    ]);
    expect(bucketed.values).toEqual([1, 1, 1, 0, 1, 1]);
    expect(bucketed.labels).toHaveLength(6);
  });
});

describe('issueGroups', () => {
  it('groups by issue id, worst severity first', () => {
    const issue = (id: string, severity: 'minor' | 'major' | 'critical') => ({
      id,
      severity,
      description: id,
      refUrl: '',
    });
    const groups = issueGroups([
      machine({ issues: [issue('liveliness-dead', 'major')] }),
      machine({ issues: [issue('liveliness-dead', 'major')] }),
      machine({ issues: [issue('failed-machine-reclaim', 'critical')] }),
      machine({ issues: [issue('last-event-error', 'minor')] }),
    ]);
    expect(groups.map((group) => [group.id, group.count])).toEqual([
      ['failed-machine-reclaim', 1],
      ['liveliness-dead', 2],
      ['last-event-error', 1],
    ]);
  });
});
