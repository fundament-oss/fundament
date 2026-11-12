import { create } from '@bufbuild/protobuf';
import {
  ClusterStatus,
  ClusterSummarySchema,
  DriftFindingSchema,
  DriftKind,
  OrganizationSummarySchema,
} from '../../generated/fleet/v1/fleet_pb';
import {
  ALL,
  clusterStatusOptions,
  countText,
  driftKindOptions,
  filterClusters,
  filterFindings,
  filterOrganizations,
  matchesQuery,
} from './platform-filters';

describe('matchesQuery', () => {
  it('lets everything through on an empty query', () => {
    expect(matchesQuery('', 'anything')).toBe(true);
    expect(matchesQuery('   ', 'anything')).toBe(true);
  });

  it('needs every word somewhere in the fields, ignoring case', () => {
    expect(matchesQuery('acme PROD', 'Acme B.V.', 'prod-eu')).toBe(true);
    expect(matchesQuery('acme staging', 'Acme B.V.', 'prod-eu')).toBe(false);
  });

  it('skips fields that are not there', () => {
    expect(matchesQuery('acme', undefined, 'acme')).toBe(true);
  });
});

describe('filterOrganizations', () => {
  const organizations = [
    create(OrganizationSummarySchema, {
      id: '1',
      alias: 'acme',
      name: 'Acme',
      clustersUnhealthy: 0,
    }),
    create(OrganizationSummarySchema, {
      id: '2',
      alias: 'globex',
      name: 'Globex',
      clustersUnhealthy: 2,
    }),
  ];

  it('searches alias and name', () => {
    expect(filterOrganizations(organizations, 'glob', false).map((o) => o.id)).toEqual(['2']);
  });

  it('keeps only organizations with an unhealthy cluster when asked', () => {
    expect(filterOrganizations(organizations, '', true).map((o) => o.id)).toEqual(['2']);
  });
});

describe('filterClusters', () => {
  const clusters = [
    create(ClusterSummarySchema, {
      id: 'a',
      name: 'web',
      region: 'nl-ams',
      status: ClusterStatus.RUNNING,
    }),
    create(ClusterSummarySchema, {
      id: 'b',
      name: 'db',
      region: 'nl-ams',
      status: ClusterStatus.ERROR,
    }),
    create(ClusterSummarySchema, {
      id: 'c',
      name: 'ci',
      region: 'de-fra',
      status: ClusterStatus.RUNNING,
    }),
  ];

  it('narrows by status and search together', () => {
    expect(filterClusters(clusters, 'ams', ClusterStatus.RUNNING).map((c) => c.id)).toEqual(['a']);
    expect(filterClusters(clusters, '', ALL)).toHaveLength(3);
  });

  it('offers the statuses that occur, in enum order, with counts', () => {
    expect(clusterStatusOptions(clusters, ALL)).toEqual([
      { value: ClusterStatus.RUNNING, label: 'Running', count: 2 },
      { value: ClusterStatus.ERROR, label: 'Error', count: 1 },
    ]);
  });

  it('keeps the picked status in the menu after its last cluster went', () => {
    expect(clusterStatusOptions(clusters, ClusterStatus.LOST).at(-1)).toEqual({
      value: ClusterStatus.LOST,
      label: 'Lost',
      count: 0,
    });
  });
});

describe('filterFindings', () => {
  const findings = [
    create(DriftFindingSchema, {
      id: '1',
      kind: DriftKind.SHOOT_WITHOUT_CLUSTER,
      organizationName: 'Acme',
      shootNamespace: 'garden-acme',
      shootName: 'orphan',
    }),
    create(DriftFindingSchema, {
      id: '2',
      kind: DriftKind.KUBERNETES_VERSION_MISMATCH,
      organizationName: 'Globex',
      clusterName: 'web',
      detail: '1.31 in the database, 1.32 in Gardener',
    }),
  ];

  it('searches the organization, cluster, shoot and detail', () => {
    expect(filterFindings(findings, 'orphan', ALL).map((f) => f.id)).toEqual(['1']);
    expect(filterFindings(findings, '1.32', ALL).map((f) => f.id)).toEqual(['2']);
  });

  it('narrows by kind', () => {
    expect(
      filterFindings(findings, '', DriftKind.KUBERNETES_VERSION_MISMATCH).map((f) => f.id),
    ).toEqual(['2']);
  });

  it('offers the kinds that occur', () => {
    expect(driftKindOptions(findings, ALL).map((o) => o.label)).toEqual([
      'Shoot without cluster',
      'Version mismatch',
    ]);
  });
});

describe('countText', () => {
  it('says "x of y" only while something narrows the list', () => {
    expect(countText(12, 12, 'cluster', 'clusters')).toBe('12 clusters');
    expect(countText(3, 12, 'cluster', 'clusters')).toBe('3 of 12 clusters');
    expect(countText(1, 1, 'cluster', 'clusters')).toBe('1 cluster');
  });
});
