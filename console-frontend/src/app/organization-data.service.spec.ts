import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { OrganizationDataService } from './organization-data.service';
import { CLUSTER, ORGANIZATION, PROJECT } from '../connect/tokens';
import { GetOrganizationResponseSchema, OrganizationSchema } from '../generated/v1/organization_pb';
import {
  ListClustersResponseSchema,
  ListClustersResponse_ClusterSummarySchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../generated/v1/cluster_pb';
import { ClusterStatus } from '../generated/v1/common_pb';

const ORGANIZATION_ID = '019b4000-1000-7000-8000-000000000001';

function summary(id: string, name: string, status: ClusterStatus): ClusterSummary {
  return create(ListClustersResponse_ClusterSummarySchema, {
    id,
    name,
    status,
    region: 'eu-nl-1',
    projectCount: 2,
  });
}

/** The service behind a ListClusters the test drives: `listed` is what the
 *  server answers with, and `fail` makes it answer with an error instead. */
function build(listed: ClusterSummary[]) {
  const server = { listed, fail: false };

  TestBed.configureTestingModule({
    providers: [
      {
        provide: ORGANIZATION,
        useValue: {
          getOrganization: () =>
            of(
              create(GetOrganizationResponseSchema, {
                organization: create(OrganizationSchema, {
                  id: ORGANIZATION_ID,
                  name: 'acme',
                  alias: 'Acme',
                }),
              }),
            ),
        },
      },
      {
        provide: CLUSTER,
        useValue: {
          listClusters: () =>
            server.fail
              ? throwError(() => new Error('unreachable'))
              : of(create(ListClustersResponseSchema, { clusters: server.listed })),
        },
      },
      { provide: PROJECT, useValue: { listProjects: () => of({ projects: [] }) } },
    ],
  });

  return { service: TestBed.inject(OrganizationDataService), server };
}

function statuses(service: OrganizationDataService): [string, ClusterStatus][] {
  return service.clusterSummaries().map((cluster) => [cluster.name, cluster.status]);
}

describe('OrganizationDataService reloadClusters', () => {
  const alpha = summary('cluster-a', 'alpha', ClusterStatus.RUNNING);
  const bravo = summary('cluster-b', 'bravo', ClusterStatus.RUNNING);

  // The failure paths log, and a restore at the end of the test body is not
  // reached when an assertion fails: the stub would then go on hiding the
  // service's errors through the tests that follow the one being debugged.
  afterEach(() => vi.restoreAllMocks());

  it('keeps a soft-deleted cluster in the list, as the server reports it', async () => {
    const { service, server } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);

    // DeleteCluster only sets `deleted`, so the cluster is still listed.
    server.listed = [alpha, summary('cluster-b', 'bravo', ClusterStatus.DELETING)];
    await service.reloadClusters({
      id: 'cluster-b',
      name: 'bravo',
      status: ClusterStatus.DELETING,
    });

    expect(statuses(service)).toEqual([
      ['alpha', ClusterStatus.RUNNING],
      ['bravo', ClusterStatus.DELETING],
    ]);
    // Still in the nested cache too: the sidebar counts it and the projects
    // view resolves against it until it is really gone.
    expect(service.organizations()[0].clusters.map((cluster) => cluster.name)).toEqual([
      'alpha',
      'bravo',
    ]);
  });

  it('marks the cluster itself when the list cannot be fetched', async () => {
    const { service, server } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    server.fail = true;
    await service.reloadClusters({
      id: 'cluster-b',
      name: 'bravo',
      status: ClusterStatus.DELETING,
    });

    expect(statuses(service)).toEqual([
      ['alpha', ClusterStatus.RUNNING],
      ['bravo', ClusterStatus.DELETING],
    ]);
    // What the cluster was listed with is kept: only the status is news.
    expect(service.clusterSummaries()[1].region).toBe('eu-nl-1');
    expect(service.clusterSummaries()[1].projectCount).toBe(2);
  });

  it('puts a created cluster in by hand when the list cannot be fetched', async () => {
    const { service, server } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);
    vi.spyOn(console, 'error').mockImplementation(() => {});

    server.fail = true;
    await service.reloadClusters({
      id: 'cluster-c',
      name: 'aardvark',
      status: ClusterStatus.PROVISIONING,
    });

    // By name, not appended: the list is sorted, so the new cluster lands where
    // the next poll will find it rather than jumping once the list comes back.
    expect(statuses(service)).toEqual([
      ['aardvark', ClusterStatus.PROVISIONING],
      ['alpha', ClusterStatus.RUNNING],
      ['bravo', ClusterStatus.RUNNING],
    ]);
    expect(service.organizations()[0].clusters.map((cluster) => cluster.name)).toEqual([
      'aardvark',
      'alpha',
      'bravo',
    ]);
  });
});

describe('OrganizationDataService setClusters', () => {
  const alpha = summary('cluster-a', 'alpha', ClusterStatus.RUNNING);
  const bravo = summary('cluster-b', 'bravo', ClusterStatus.RUNNING);

  it('lets go of a cluster once the server stops listing it', async () => {
    const { service } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);

    // What the clusters page sees once the deletion has finished. Nothing else
    // watches, so this is where the cache hears about it.
    service.setClusters([alpha], service.beginClusterListFetch());

    expect(statuses(service)).toEqual([['alpha', ClusterStatus.RUNNING]]);
    expect(service.organizations()[0].clusters.map((cluster) => cluster.name)).toEqual(['alpha']);
  });

  it('drops a list that a later one has already overtaken', async () => {
    const { service } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);

    // Two polls in flight at once: the page asks again every five seconds
    // without waiting for the answer, so they can come back the other way
    // round. The older one must not put the deleted cluster back.
    const first = service.beginClusterListFetch();
    const second = service.beginClusterListFetch();

    expect(service.setClusters([alpha], second)).toBe(true);
    expect(service.setClusters([alpha, bravo], first)).toBe(false);

    expect(statuses(service)).toEqual([['alpha', ClusterStatus.RUNNING]]);
  });

  it('drops a list that outlived the organization it was asked about', async () => {
    const { service, server } = build([alpha, bravo]);
    await service.loadOrganizationData(ORGANIZATION_ID);
    const stale = service.beginClusterListFetch();

    // A second organization is loaded while that poll is in flight.
    server.listed = [summary('cluster-z', 'zulu', ClusterStatus.RUNNING)];
    await service.loadOrganizationData('019b4000-1000-7000-8000-000000000002');

    service.setClusters([alpha, bravo], stale);

    expect(statuses(service)).toEqual([['zulu', ClusterStatus.RUNNING]]);
  });
});
