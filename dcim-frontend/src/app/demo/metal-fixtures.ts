// Demo-only metal-stack metrics: one coherent estate, generated in memory so
// the Bare metal section can be walked through without a Prometheus. Never
// imported by the production entrypoint (main.ts).
//
// The shapes and the issue ids are the ones the exporters publish, including
// the parts that are awkward: machines without an allocation carry no name and
// no cluster, a machine whose BMC reports nothing has no power reading, and an
// issue series is only present when it is 1.
import type {
  ChartSeries,
  ComponentStatus,
  Histogram,
  Machine,
  MachineIssue,
  MachineRole,
  MachineState,
  MetalMetrics,
  Network,
  PartitionCapacity,
  PowerState,
  Switch,
} from '../bare-metal/metal-metrics';
import { NOW, id } from './fixtures';
import { formatAxisTime } from '../bare-metal/metal-format';

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

/** A small seeded generator, so the same estate every run: a screenshot from
 *  yesterday still matches what you see today. */
function seeded(seed: number): () => number {
  let state = seed % 2147483647;
  if (state <= 0) state += 2147483646;
  return () => {
    state = (state * 16807) % 2147483647;
    return (state - 1) / 2147483646;
  };
}

const random = seeded(0x5eed);

/** Axis labels for a series, the way the real read formats them. */
function timeLabels(step: number, count: number): string[] {
  const first = NOW.getTime() - (count - 1) * step;
  return Array.from({ length: count }, (unused, index) =>
    formatAxisTime(new Date(first + index * step), step),
  );
}

/** The cluster ids the fleet demo knows, so a machine's cluster tag links to a
 *  cluster page that exists. */
const CLUSTERS = ['cl-utrecht-prod', 'cl-utrecht-test', 'cl-zwolle-prod', 'cl-rvo-prod'];

const ISSUES: Record<string, Omit<MachineIssue, 'id'>> = {
  'liveliness-dead': {
    severity: 'major',
    description: 'the machine is not sending events anymore',
    refUrl: '',
  },
  crashloop: {
    severity: 'major',
    description: 'machine is in a provisioning crash loop',
    refUrl: '',
  },
  'bmc-info-outdated': {
    severity: 'major',
    description: 'BMC has not been updated from either metal-hammer or metal-bmc',
    refUrl: '',
  },
  'last-event-error': {
    severity: 'minor',
    description: 'the machine had an error during the provisioning lifecycle',
    refUrl: '',
  },
  'failed-machine-reclaim': {
    severity: 'critical',
    description: 'machine phones home but not allocated',
    refUrl: '',
  },
};

/** Which machines carry which issue, by generation order. One critical issue,
 *  a handful of majors and two minors gives every severity something to show. */
const ISSUE_BY_SEQ: Record<number, string[]> = {
  3: ['crashloop'],
  9: ['bmc-info-outdated'],
  12: ['liveliness-dead'],
  19: ['last-event-error'],
  22: ['bmc-info-outdated'],
  26: ['failed-machine-reclaim'],
  43: ['last-event-error'],
};

interface SizePlan {
  partition: string;
  size: string;
  total: number;
  allocated: number;
  allocatable: number;
  waiting: number;
  phonedHome: number;
  faulty: number;
  unavailable: number;
  other: number;
  reservationsTotal: number;
  reservationsUsed: number;
}

const PLANS: SizePlan[] = [
  {
    partition: 'ams1',
    size: 'c1-large-x86',
    total: 12,
    allocated: 8,
    allocatable: 2,
    waiting: 1,
    phonedHome: 0,
    faulty: 1,
    unavailable: 0,
    other: 0,
    reservationsTotal: 4,
    reservationsUsed: 2,
  },
  {
    partition: 'ams1',
    size: 'c1-xlarge-x86',
    total: 8,
    allocated: 6,
    allocatable: 2,
    waiting: 0,
    phonedHome: 0,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 2,
    reservationsUsed: 1,
  },
  {
    partition: 'ams1',
    size: 's3-large-x86',
    total: 6,
    allocated: 4,
    allocatable: 1,
    waiting: 0,
    phonedHome: 1,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 1,
    reservationsUsed: 0,
  },
  {
    partition: 'ams1',
    size: 'n1-medium-x86',
    total: 2,
    allocated: 2,
    allocatable: 0,
    waiting: 0,
    phonedHome: 0,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 0,
    reservationsUsed: 0,
  },
  {
    partition: 'grn1',
    size: 'c1-large-x86',
    total: 8,
    allocated: 5,
    allocatable: 1,
    waiting: 0,
    phonedHome: 0,
    faulty: 1,
    unavailable: 1,
    other: 0,
    reservationsTotal: 3,
    reservationsUsed: 1,
  },
  {
    partition: 'grn1',
    size: 'c1-xlarge-x86',
    total: 6,
    allocated: 4,
    allocatable: 2,
    waiting: 0,
    phonedHome: 0,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 2,
    reservationsUsed: 1,
  },
  {
    partition: 'grn1',
    size: 's3-large-x86',
    total: 4,
    allocated: 3,
    allocatable: 0,
    waiting: 1,
    phonedHome: 0,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 1,
    reservationsUsed: 0,
  },
  {
    partition: 'grn1',
    size: 'n1-medium-x86',
    total: 2,
    allocated: 2,
    allocatable: 0,
    waiting: 0,
    phonedHome: 0,
    faulty: 0,
    unavailable: 0,
    other: 0,
    reservationsTotal: 0,
    reservationsUsed: 0,
  },
];

const partitions: PartitionCapacity[] = PLANS.map((plan) => ({
  partition: plan.partition,
  size: plan.size,
  total: plan.total,
  allocated: plan.allocated,
  allocatable: plan.allocatable,
  waiting: plan.waiting,
  phonedHome: plan.phonedHome,
  faulty: plan.faulty,
  unavailable: plan.unavailable,
  other: plan.other,
  reservationsTotal: plan.reservationsTotal,
  reservationsUsed: plan.reservationsUsed,
}));

type Classification =
  'allocated' | 'allocatable' | 'waiting' | 'phonedHome' | 'faulty' | 'unavailable' | 'other';

/** Where an unallocated machine sits in its size's breakdown. */
function classify(plan: SizePlan, index: number): Classification {
  const offset = index - plan.allocated;
  let bound = plan.allocatable;
  if (offset < bound) return 'allocatable';
  bound += plan.waiting;
  if (offset < bound) return 'waiting';
  bound += plan.phonedHome;
  if (offset < bound) return 'phonedHome';
  bound += plan.faulty;
  if (offset < bound) return 'faulty';
  bound += plan.unavailable;
  if (offset < bound) return 'unavailable';
  return 'other';
}

function stateOf(classification: Classification, seq: number): MachineState {
  if (classification === 'faulty') return 'locked';
  if (classification === 'unavailable' || classification === 'other') return 'other';
  if (classification === 'allocated' && seq === 13) return 'tainted';
  if (classification === 'allocated' && seq === 28) return 'locked';
  return 'available';
}

function powerOf(classification: Classification, seq: number, noBmc: boolean): number | null {
  if (noBmc) return null;
  if (classification === 'allocated') return 300 + ((seq * 37) % 200);
  if (classification === 'allocatable') return 60 + ((seq * 13) % 60);
  if (classification === 'waiting' || classification === 'phonedHome') {
    return 50 + ((seq * 17) % 40);
  }
  return null;
}

function powerStateOf(seq: number, noBmc: boolean): PowerState | null {
  if (noBmc) return null;
  if (seq === 6 || seq === 20) return 'off';
  if (seq === 2) return 'unknown';
  return 'on';
}

function shortSize(size: string): string {
  return size.replace(/-x86$/, '');
}

function roleOf(plan: SizePlan, allocated: boolean): MachineRole {
  if (!allocated) return 'none';
  return plan.size.startsWith('n1-') ? 'firewall' : 'machine';
}

function imageOf(role: MachineRole): string {
  return role === 'firewall' ? 'firewall-ubuntu-24.04' : 'ubuntu-24.04';
}

function psuHealthyOf(sequence: number, noBmc: boolean): number {
  if (noBmc) return 0;
  return sequence === 5 ? 1 : 2;
}

const machines: Machine[] = [];
let sequence = 0;
PLANS.forEach((plan) => {
  for (let index = 0; index < plan.total; index += 1) {
    sequence += 1;
    const classification = index < plan.allocated ? 'allocated' : classify(plan, index);
    const allocated = classification === 'allocated';
    const noBmc = sequence === 9 || classification === 'faulty' || classification === 'unavailable';
    const role = roleOf(plan, allocated);
    const issueIds = ISSUE_BY_SEQ[sequence] ?? [];

    machines.push({
      id: id('machine', sequence),
      name: allocated
        ? `${plan.partition}-${shortSize(plan.size)}-${String(index + 1).padStart(2, '0')}`
        : '',
      partition: plan.partition,
      size: plan.size,
      state: stateOf(classification, sequence),
      role,
      clusterTag: allocated ? CLUSTERS[(sequence - 1) % CLUSTERS.length] : '',
      imageId: allocated ? imageOf(role) : '',
      powerWatts: powerOf(classification, sequence, noBmc),
      powerState: powerStateOf(sequence, noBmc),
      psuTotal: noBmc ? 0 : 2,
      psuHealthy: psuHealthyOf(sequence, noBmc),
      issues: issueIds.map((issueId) => ({ id: issueId, ...ISSUES[issueId] })),
    });
  }
});

const networks: Network[] = [
  {
    id: id('network', 1),
    name: 'internet-ams1',
    partition: 'ams1',
    ipsUsed: 210,
    ipsAvailable: 44,
    prefixesUsed: 3,
    prefixesAvailable: 2,
  },
  {
    id: id('network', 2),
    name: 'internet-grn1',
    partition: 'grn1',
    ipsUsed: 180,
    ipsAvailable: 74,
    prefixesUsed: 2,
    prefixesAvailable: 3,
  },
  {
    id: id('network', 3),
    name: 'tenant-utrecht-prod',
    partition: 'ams1',
    ipsUsed: 96,
    ipsAvailable: 30,
    prefixesUsed: 1,
    prefixesAvailable: 1,
  },
  {
    id: id('network', 4),
    name: 'tenant-zwolle-prod',
    partition: 'grn1',
    ipsUsed: 70,
    ipsAvailable: 58,
    prefixesUsed: 1,
    prefixesAvailable: 2,
  },
  {
    id: id('network', 5),
    name: 'tenant-rvo-prod',
    partition: 'grn1',
    ipsUsed: 120,
    ipsAvailable: 8,
    prefixesUsed: 2,
    prefixesAvailable: 0,
  },
];

const switches: Switch[] = [
  {
    name: 'leaf-ams1-01',
    partition: 'ams1',
    rack: 'R01-1',
    metalCoreUp: true,
    syncFailing: false,
    os: 'SONiC 4.2',
    metalCoreVersion: '0.9.1',
    bgpEstablished: 8,
    bgpTotal: 8,
  },
  {
    name: 'leaf-ams1-02',
    partition: 'ams1',
    rack: 'R01-2',
    metalCoreUp: true,
    syncFailing: true,
    os: 'SONiC 4.2',
    metalCoreVersion: '0.9.1',
    bgpEstablished: 7,
    bgpTotal: 8,
  },
  {
    name: 'leaf-ams1-03',
    partition: 'ams1',
    rack: 'R02-1',
    metalCoreUp: false,
    syncFailing: true,
    os: 'SONiC 4.1',
    metalCoreVersion: '0.8.4',
    bgpEstablished: 0,
    bgpTotal: 8,
  },
  {
    name: 'leaf-grn1-01',
    partition: 'grn1',
    rack: 'R02-2',
    metalCoreUp: true,
    syncFailing: false,
    os: 'SONiC 4.2',
    metalCoreVersion: '0.9.1',
    bgpEstablished: 8,
    bgpTotal: 8,
  },
  {
    name: 'leaf-grn1-02',
    partition: 'grn1',
    rack: 'R03-1',
    metalCoreUp: true,
    syncFailing: false,
    os: 'SONiC 4.2',
    metalCoreVersion: '0.9.1',
    bgpEstablished: 6,
    bgpTotal: 6,
  },
  {
    name: 'leaf-grn1-03',
    partition: 'grn1',
    rack: 'R03-2',
    metalCoreUp: true,
    syncFailing: false,
    os: 'SONiC 4.2',
    metalCoreVersion: '0.9.1',
    bgpEstablished: 4,
    bgpTotal: 8,
  },
];

const components: ComponentStatus[] = [
  {
    id: id('comp', 1),
    type: 'api',
    identifier: 'metal-api',
    up: true,
    tokenLifetimeSeconds: 30 * 86400,
  },
  {
    id: id('comp', 2),
    type: 'core',
    identifier: 'metal-core-ams1',
    up: true,
    tokenLifetimeSeconds: 8 * 86400,
  },
  {
    id: id('comp', 3),
    type: 'core',
    identifier: 'metal-core-grn1',
    up: false,
    tokenLifetimeSeconds: 2 * 86400,
  },
  {
    id: id('comp', 4),
    type: 'exporter',
    identifier: 'metal-metrics-exporter',
    up: true,
    tokenLifetimeSeconds: 14 * 86400,
  },
  {
    id: id('comp', 5),
    type: 'exporter',
    identifier: 'sonic-exporter',
    up: true,
    tokenLifetimeSeconds: 5 * 86400,
  },
  {
    id: id('comp', 6),
    type: 'db',
    identifier: 'rethinkdb',
    up: true,
    tokenLifetimeSeconds: null,
  },
];

const powerLabels = timeLabels(5 * MINUTE, 288);
const basePower = machines.reduce((total, machine) => total + (machine.powerWatts ?? 0), 0);
const powerValues = powerLabels.map((_, index) => {
  const phase = (index / powerLabels.length) * Math.PI * 2;
  return Math.round(basePower * (0.94 + 0.06 * Math.sin(phase)) + random() * 40);
});

const allocationLabels = timeLabels(HOUR, 168);
const allocationAllocated = allocationLabels.map((_, index) =>
  Math.round(30 + (index / (allocationLabels.length - 1)) * 4 + (random() - 0.5) * 0.8),
);

const syncSeries: ChartSeries[] = switches
  .filter((item) => item.metalCoreUp)
  .slice(0, 3)
  .map((item, switchIndex) => ({
    label: item.name,
    values: powerLabels.map((_, index) =>
      Math.round(
        900 +
          switchIndex * 150 +
          Math.sin(index / 18 + switchIndex) * 120 +
          random() * 90 +
          (switchIndex === 1 && index > 240 ? 600 : 0),
      ),
    ),
  }));

const requestSeries: ChartSeries[] = [
  {
    label: '2xx',
    values: powerLabels.map((_, index) =>
      Math.round(230 + Math.sin(index / 24) * 40 + random() * 20),
    ),
  },
  {
    label: '4xx',
    values: powerLabels.map(() => Math.round(12 + random() * 10)),
  },
  {
    label: '5xx',
    values: powerLabels.map(() => Math.round(1 + random() * 4)),
  },
];

const restLatency: Histogram = {
  buckets: [
    { le: 0.25, count: 6100 },
    { le: 0.5, count: 8200 },
    { le: 1, count: 9100 },
    { le: 2.5, count: 9300 },
    { le: 5, count: 9340 },
    { le: 10, count: 9350 },
    { le: Infinity, count: 9350 },
  ],
  sum: 4120,
  count: 9350,
};

const grpcLatency: Histogram = {
  buckets: [
    { le: 0.25, count: 18000 },
    { le: 0.5, count: 19200 },
    { le: 1, count: 19500 },
    { le: 2.5, count: 19560 },
    { le: 5, count: 19570 },
    { le: 10, count: 19572 },
    { le: Infinity, count: 19572 },
  ],
  sum: 9100,
  count: 19572,
};

/** The whole section's snapshot, as the demo's MetalMetricsService returns it. */
const metalMetrics: MetalMetrics = {
  collectedAt: NOW,
  partitions,
  machines,
  networks,
  switches,
  components,
  power: { labels: powerLabels, values: powerValues },
  allocation: {
    labels: allocationLabels,
    allocated: allocationAllocated,
    total: allocationLabels.map(() => machines.length),
  },
  syncDurations: { labels: powerLabels, switches: syncSeries },
  coreErrors: {
    labels: ['switch-sync', 'machine-reclaim', 'event-receive', 'bgp-update'],
    values: [42, 13, 7, 3],
  },
  requestRate: { labels: powerLabels, series: requestSeries },
  restLatency,
  grpcLatency,
};

export default metalMetrics;
