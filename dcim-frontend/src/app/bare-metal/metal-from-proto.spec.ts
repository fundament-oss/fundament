import { create, type MessageInitShape } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  GetMetalSnapshotResponseSchema,
  MetalQueryResultSchema,
} from '../../generated/v1/metal_pb';
import metalFromProto, { machineStateOf } from './metal-from-proto';

const COLLECTED = new Date('2026-11-25T10:07:00Z');

type Labels = Record<string, string>;

type Result = MessageInitShape<typeof MetalQueryResultSchema>;

const instant = (...samples: [Labels, number][]): Result => ({
  samples: samples.map(([labels, value]) => ({ labels, value })),
});

const range = (...series: [Labels, [string, number][]][]): Result => ({
  series: series.map(([labels, points]) => ({
    labels,
    points: points.map(([at, value]) => ({ time: timestampFromDate(new Date(at)), value })),
  })),
});

/** One partition, one allocated and one free machine, one switch: the labels
 *  as the exporter publishes them, the results named as dcim-api names them. */
function response(extra: Record<string, Result> = {}) {
  return create(GetMetalSnapshotResponseSchema, {
    collectedAt: timestampFromDate(COLLECTED),
    results: {
      capacity: instant(
        [
          { __name__: 'metal_partition_capacity_total', partition: 'fire', size: 'c1-large-x86' },
          2,
        ],
        [
          {
            __name__: 'metal_partition_capacity_allocated',
            partition: 'fire',
            size: 'c1-large-x86',
          },
          1,
        ],
        [
          {
            __name__: 'metal_partition_capacity_allocatable',
            partition: 'fire',
            size: 'c1-large-x86',
          },
          1,
        ],
        [
          {
            __name__: 'metal_partition_capacity_phoned_home',
            partition: 'fire',
            size: 'c1-large-x86',
          },
          0,
        ],
        [
          {
            __name__: 'metal_partition_capacity_reservations_total',
            partition: 'fire',
            size: 'c1-large-x86',
          },
          3,
        ],
      ),
      machine_allocation: instant(
        [
          {
            machineid: 'm-2',
            partition: 'fire',
            machinename: 'NOTALLOCATED',
            clusterTag: 'NOTALLOCATED',
            role: '',
            state: 'AVAILABLE',
            imageId: 'NOTALLOCATED',
          },
          1,
        ],
        [
          {
            machineid: 'm-1',
            partition: 'fire',
            machinename: 'worker-1',
            clusterTag: 'cl-utrecht-prod',
            role: 'machine',
            state: 'TAINTED',
            imageId: 'ubuntu-24.04',
          },
          1,
        ],
      ),
      machine_size: instant([{ machineid: 'm-1', size: 'c1-large-x86' }, 1]),
      machine_power: instant(
        [{ __name__: 'metal_machine_power_usage', machineid: 'm-1' }, 320],
        [{ __name__: 'metal_machine_power_state', machineid: 'm-1' }, 1],
        [{ __name__: 'metal_machine_power_state', machineid: 'm-2' }, -1],
        [{ __name__: 'metal_machine_power_supplies_total', machineid: 'm-1' }, 2],
        [{ __name__: 'metal_machine_power_supplies_healthy', machineid: 'm-1' }, 1],
      ),
      machine_issues: instant([{ machineid: 'm-1', issueid: 'crashloop' }, 1]),
      machine_issue_info: instant(
        [
          {
            issueid: 'crashloop',
            severity: 'major',
            description: 'machine is in a provisioning crash loop',
            refurl: 'https://docs.metal-stack.io/issues#crashloop',
          },
          1,
        ],
        [
          { issueid: 'failed-machine-reclaim', severity: 'critical', description: '', refurl: '' },
          1,
        ],
      ),
      network_info: instant([{ networkId: 'internet', name: '', partition: '' }, 1]),
      network_usage: instant(
        [{ __name__: 'metal_network_ip_used', networkId: 'internet' }, 5],
        [{ __name__: 'metal_network_ip_available', networkId: 'internet' }, 250],
      ),
      switch_info: instant([
        {
          switchname: 'leaf01',
          partition: 'fire',
          rackid: 'rack-1',
          metalCoreVersion: 'v0.14.0',
          osVendor: 'SONiC',
          osVersion: '202311',
        },
        1,
      ]),
      switch_state: instant(
        [{ __name__: 'metal_switch_metal_core_up', switchname: 'leaf01' }, 1],
        [{ __name__: 'metal_switch_sync_failed', switchname: 'leaf01' }, 0],
      ),
      switch_bgp_total: instant([{ switchname: 'leaf01' }, 4]),
      switch_bgp_established: instant([{ switchname: 'leaf01' }, 3]),
      component_up: instant(
        [{ uuid: 'c-1', type: 'metal-core', identifier: 'leaf01' }, 1],
        [{ uuid: 'c-2', type: 'metal-bmc', identifier: 'mgmt01' }, 0],
      ),
      component_token: instant([{ uuid: 'c-1' }, 86400]),
      rest_latency_buckets: instant([{ le: '+Inf' }, 10], [{ le: '0.5' }, 8], [{ le: '0.1' }, 4]),
      rest_latency_sum: instant([{}, 2.5]),
      rest_latency_count: instant([{}, 10]),
      power: range([
        {},
        [
          ['2026-11-25T09:55:00Z', 400],
          ['2026-11-25T10:00:00Z', 410],
        ],
      ]),
      sync_durations: range(
        [{ switchname: 'leaf02' }, [['2026-11-25T10:00:00Z', 900]]],
        [
          { switchname: 'leaf01' },
          [
            ['2026-11-25T09:55:00Z', 800],
            ['2026-11-25T10:00:00Z', 850],
          ],
        ],
      ),
      allocation_allocated: range([{}, [['2026-11-25T10:00:00Z', 1]]]),
      allocation_total: range([{}, [['2026-11-25T10:00:00Z', 2]]]),
      ...extra,
    },
  });
}

describe('metalFromProto', () => {
  it('reads the collection time', () => {
    expect(metalFromProto(response()).collectedAt).toEqual(COLLECTED);
  });

  it('folds the capacity gauges into one row per partition and size', () => {
    const [row] = metalFromProto(response()).partitions;
    expect(row).toEqual({
      partition: 'fire',
      size: 'c1-large-x86',
      total: 2,
      allocated: 1,
      allocatable: 1,
      waiting: 0,
      phonedHome: 0,
      faulty: 0,
      unavailable: 0,
      other: 0,
      reservationsTotal: 3,
      reservationsUsed: 0,
    });
  });

  it('joins machines with their size, power and issues', () => {
    const machines = metalFromProto(response()).machines;
    expect(machines.map((m) => m.id)).toEqual(['m-2', 'm-1']);
    const allocated = machines[1];
    expect(allocated).toEqual({
      id: 'm-1',
      name: 'worker-1',
      partition: 'fire',
      size: 'c1-large-x86',
      state: 'tainted',
      role: 'machine',
      clusterTag: 'cl-utrecht-prod',
      imageId: 'ubuntu-24.04',
      powerWatts: 320,
      powerState: 'on',
      psuTotal: 2,
      psuHealthy: 1,
      issues: [
        {
          id: 'crashloop',
          severity: 'major',
          description: 'machine is in a provisioning crash loop',
          refUrl: 'https://docs.metal-stack.io/issues#crashloop',
        },
      ],
    });
  });

  it('empties what a machine without allocation does not have', () => {
    const free = metalFromProto(response()).machines[0];
    expect(free.name).toBe('');
    expect(free.clusterTag).toBe('');
    expect(free.imageId).toBe('');
    expect(free.role).toBe('none');
    expect(free.size).toBe('');
    expect(free.powerWatts).toBeNull();
    expect(free.powerState).toBe('unknown');
    expect(free.psuTotal).toBe(0);
    expect(free.issues).toEqual([]);
  });

  it('normalizes the v2 machine states', () => {
    expect(machineStateOf('AVAILABLE')).toBe('available');
    expect(machineStateOf('TAINTED')).toBe('tainted');
    expect(machineStateOf('LOCKED')).toBe('locked');
    expect(machineStateOf('')).toBe('other');
    expect(machineStateOf(undefined)).toBe('other');
  });

  it('joins networks with their consumption, named by id when unnamed', () => {
    expect(metalFromProto(response()).networks).toEqual([
      {
        id: 'internet',
        name: 'internet',
        partition: '',
        ipsUsed: 5,
        ipsAvailable: 250,
        prefixesUsed: 0,
        prefixesAvailable: 0,
      },
    ]);
  });

  it('joins switches with their state and BGP counts', () => {
    expect(metalFromProto(response()).switches).toEqual([
      {
        name: 'leaf01',
        partition: 'fire',
        rack: 'rack-1',
        metalCoreUp: true,
        syncFailing: false,
        os: 'SONiC 202311',
        metalCoreVersion: 'v0.14.0',
        bgpEstablished: 3,
        bgpTotal: 4,
      },
    ]);
  });

  it('counts no established sessions as zero', () => {
    const [item] = metalFromProto(response({ switch_bgp_established: {} })).switches;
    expect(item.bgpEstablished).toBe(0);
  });

  it('joins components with their token lifetime', () => {
    expect(metalFromProto(response()).components).toEqual([
      { id: 'c-2', type: 'metal-bmc', identifier: 'mgmt01', up: false, tokenLifetimeSeconds: null },
      {
        id: 'c-1',
        type: 'metal-core',
        identifier: 'leaf01',
        up: true,
        tokenLifetimeSeconds: 86400,
      },
    ]);
  });

  it('sorts histogram buckets and reads +Inf as the last bound', () => {
    expect(metalFromProto(response()).restLatency).toEqual({
      buckets: [
        { le: 0.1, count: 4 },
        { le: 0.5, count: 8 },
        { le: Infinity, count: 10 },
      ],
      sum: 2.5,
      count: 10,
    });
  });

  it('reads an empty histogram when metal-api has not been scraped', () => {
    expect(metalFromProto(response()).grpcLatency).toEqual({ buckets: [], sum: 0, count: 0 });
  });

  it('labels the time axis and leaves gaps where a series has no point', () => {
    const metrics = metalFromProto(response());
    expect(metrics.power).toEqual({ labels: ['09:55', '10:00'], values: [400, 410] });
    expect(metrics.syncDurations.labels).toEqual(['09:55', '10:00']);
    expect(metrics.syncDurations.switches.map((s) => s.label)).toEqual(['leaf01', 'leaf02']);
    expect(metrics.syncDurations.switches[1].values[0]).toBeNaN();
    expect(metrics.syncDurations.switches[1].values[1]).toBe(900);
    expect(metrics.allocation).toEqual({
      labels: ['11-25 10:00'],
      allocated: [1],
      total: [2],
    });
  });

  it('leaves metal-core errors absent while nothing collects them', () => {
    expect(metalFromProto(response()).coreErrors).toBeUndefined();
  });

  it('reads metal-core errors by operation, most first', () => {
    const metrics = metalFromProto(
      response({
        core_errors: instant([{ operation: 'bgp-update' }, 2], [{ operation: 'switch-sync' }, 7]),
      }),
    );
    expect(metrics.coreErrors).toEqual({ labels: ['switch-sync', 'bgp-update'], values: [7, 2] });
  });
});
