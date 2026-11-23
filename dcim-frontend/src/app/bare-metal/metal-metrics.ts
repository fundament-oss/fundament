// The shapes metal-stack's Prometheus series arrive in, as the page reads
// them. Field names follow the labels of the exporters, so the Prometheus read
// that replaces the mock maps onto these types without a second vocabulary.
//
// Nothing here is generated: there is no metal-stack API in the DCIM frontend
// yet. The seam that will fill it is metal-metrics.service.ts.

/** One series over time. Labels are preformatted for the axis, because the
 *  charts use a category scale rather than Chart.js's time adapter. */
export interface TimeSeries {
  labels: string[];
  values: number[];
}

/** A named series, the unit the chart component draws. */
export interface ChartSeries {
  label: string;
  values: number[];
}

/** One bucket of a classic Prometheus histogram. `count` is cumulative, as
 *  the exporter publishes it. The last bucket has an infinite upper bound. */
export interface HistogramBucket {
  le: number;
  count: number;
}

export interface Histogram {
  buckets: HistogramBucket[];
  sum: number;
  count: number;
}

/** `metal_partition_capacity_*`, one row per partition and machine size. */
export interface PartitionCapacity {
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

/** The machine states metal-stack's condition reports, normalized. */
export type MachineState = 'available' | 'reserved' | 'locked' | 'other';

export type MachineRole = 'machine' | 'firewall' | 'none';

export type PowerState = 'on' | 'off' | 'unknown';

export type IssueSeverity = 'minor' | 'major' | 'critical';

export interface MachineIssue {
  id: string;
  severity: IssueSeverity;
  description: string;
  refUrl: string;
}

/** One machine, joined from `metal_machine_allocation_info`, the power series
 *  and the issue series. Only issues that are present are listed: the exporter
 *  publishes a 0/1 series per known issue, and zero is not an issue. */
export interface Machine {
  id: string;
  /** Empty while the machine has no allocation. */
  name: string;
  partition: string;
  size: string;
  state: MachineState;
  role: MachineRole;
  /** Empty while the machine has no allocation. */
  clusterTag: string;
  /** Empty while the machine has no allocation. */
  imageId: string;
  /** null when the BMC reported nothing for this machine. */
  powerWatts: number | null;
  powerState: PowerState | null;
  psuTotal: number;
  psuHealthy: number;
  issues: MachineIssue[];
}

/** `metal_network_*`, one row per network. */
export interface Network {
  id: string;
  name: string;
  partition: string;
  ipsUsed: number;
  ipsAvailable: number;
  prefixesUsed: number;
  prefixesAvailable: number;
}

/** `metal_switch_*`, one row per switch. */
export interface Switch {
  name: string;
  partition: string;
  rack: string;
  metalCoreUp: boolean;
  syncFailing: boolean;
  os: string;
  metalCoreVersion: string;
  bgpEstablished: number;
  bgpTotal: number;
}

/** `metal_component_*`, one row per control-plane component. */
export interface ComponentStatus {
  id: string;
  type: string;
  identifier: string;
  up: boolean;
  /** null when the component reports no token. */
  tokenLifetimeSeconds: number | null;
}

/** 7 days of allocated machines against the estate's total. */
export interface AllocationHistory {
  labels: string[];
  allocated: number[];
  total: number[];
}

/** 24 h of sync duration per switch, in milliseconds. */
export interface SyncHistory {
  labels: string[];
  switches: ChartSeries[];
}

/** Everything the section shows, as one snapshot. */
export interface MetalMetrics {
  /** When the snapshot was read. */
  collectedAt: Date;
  partitions: PartitionCapacity[];
  machines: Machine[];
  networks: Network[];
  switches: Switch[];
  components: ComponentStatus[];
  /** 24 h of total machine power, in watts. */
  power: TimeSeries;
  allocation: AllocationHistory;
  syncDurations: SyncHistory;
  /** Total metal-core errors over the window, by operation. */
  coreErrors: { labels: string[]; values: number[] };
  /** 24 h of request rate per status class, in requests per minute. */
  requestRate: { labels: string[]; series: ChartSeries[] };
  restLatency: Histogram;
  grpcLatency: Histogram;
}
