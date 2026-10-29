import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';
import { OrganizationDataService } from './organization-data.service';
import { CLUSTER, ORGANIZATION, PROJECT, MEMBER } from '../connect/tokens';
import { GetOrganizationResponseSchema, OrganizationSchema } from '../generated/v1/organization_pb';
import {
  ListClustersResponseSchema,
  ListClustersResponse_ClusterSummarySchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../generated/v1/cluster_pb';
import { ClusterStatus } from '../generated/v1/common_pb';
import {
  ListMembersResponseSchema,
  MemberSchema,
  type ListMembersResponse,
  type Member,
} from '../generated/v1/member_pb';

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

function member(id: string, status: 'accepted' | 'pending'): Member {
  return create(MemberSchema, { id, userId: `user-${id}`, name: id, permission: 'viewer', status });
}

/** A ListMembers the server has not answered yet: `answer()` makes it, in
 *  whichever order the test wants the answers to land. */
class PendingMembers {
  readonly response = new Subject<ListMembersResponse>();

  answer(members: Member[]) {
    this.response.next(create(ListMembersResponseSchema, { members }));
    this.response.complete();
  }
}

/** The service behind a ListClusters the test drives: `listed` is what the
 *  server answers with, and `fail` makes it answer with an error instead.
 *  `members` is what ListMembers answers, `membersFail` makes it an error, and
 *  with `holdMembers` the answers wait in `pendingMembers` for the test. */
function build(listed: ClusterSummary[]) {
  const server = {
    listed,
    fail: false,
    members: [
      member('ann', 'accepted'),
      member('bob', 'accepted'),
      member('cas', 'accepted'),
      member('dee', 'pending'),
    ],
    membersFail: false,
    holdMembers: false,
    pendingMembers: [] as PendingMembers[],
  };

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
      {
        provide: MEMBER,
        useValue: {
          listMembers: () => {
            if (server.membersFail) return throwError(() => new Error('unreachable'));
            if (!server.holdMembers) {
              return of(create(ListMembersResponseSchema, { members: server.members }));
            }
            const pending = new PendingMembers();
            server.pendingMembers.push(pending);
            return pending.response.asObservable();
          },
        },
      },
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

  it('reloads the list when node pools change, for the pool counts it carries', async () => {
    const { service, server } = build([alpha]);
    await service.loadOrganizationData(ORGANIZATION_ID);

    server.listed = [
      create(ListClustersResponse_ClusterSummarySchema, { ...alpha, nodePoolCount: 3 }),
    ];
    service.nodePoolsChanged.update((revision) => revision + 1);
    TestBed.tick();

    await vi.waitFor(() => expect(service.clusterSummaries()[0].nodePoolCount).toBe(3));
  });

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

describe('OrganizationDataService members', () => {
  const alpha = summary('cluster-a', 'alpha', ClusterStatus.RUNNING);

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  /** Load the organization and wait for its member list too, which the load
   *  itself does not wait for. */
  async function loaded(service: OrganizationDataService) {
    await service.loadOrganizationData(ORGANIZATION_ID);
    await vi.waitFor(() => expect(service.membersLoaded()).toBe(true));
  }

  it('counts the accepted members once the list is in, leaving invitations out', async () => {
    const { service } = build([alpha]);
    expect(service.memberCount()).toBeNull();

    await service.loadOrganizationData(ORGANIZATION_ID);
    await vi.waitFor(() => expect(service.membersLoaded()).toBe(true));

    expect(service.memberCount()).toBe(3);
  });

  it('asks again for a list that never came in, so the role does not stay unknown', async () => {
    vi.useFakeTimers();
    const reported = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { service, server } = build([alpha]);

    server.membersFail = true;
    await service.loadOrganizationData(ORGANIZATION_ID);
    await vi.waitFor(() => expect(reported).toHaveBeenCalledTimes(1));
    expect(service.membersLoaded()).toBe(false);

    server.membersFail = false;
    await vi.advanceTimersByTimeAsync(1000);

    expect(service.membersLoaded()).toBe(true);
    expect(service.memberCount()).toBe(3);
  });

  it('stops asking once the organization it was for is gone', async () => {
    vi.useFakeTimers();
    const reported = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { service, server } = build([alpha]);

    server.membersFail = true;
    await service.loadOrganizationData(ORGANIZATION_ID);
    await vi.waitFor(() => expect(reported).toHaveBeenCalledTimes(1));
    const listed = vi.spyOn(TestBed.inject(MEMBER), 'listMembers');
    service.clearAll();
    await vi.advanceTimersByTimeAsync(60000);

    expect(listed).not.toHaveBeenCalled();
  });

  it('follows the list a page fetched through it', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);

    server.members = [member('ann', 'accepted')];
    const listed = await service.reloadMembers();

    expect(listed).toHaveLength(1);
    expect(service.memberCount()).toBe(1);
  });

  it('keeps the list it had when a fetch fails, and lets the page know', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);

    server.membersFail = true;
    await expect(service.reloadMembers()).rejects.toThrow('unreachable');

    expect(service.members().map((m) => m.id)).toEqual(['ann', 'bob', 'cas', 'dee']);
    expect(service.memberCount()).toBe(3);
  });

  it('leaves a list behind that was overtaken by one asked for later', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);

    // The page asks for a list, then removes someone and asks again; the
    // server answers the second before the first.
    server.holdMembers = true;
    const before = service.reloadMembers();
    const after = service.reloadMembers();
    const [first, second] = server.pendingMembers;
    second.answer([member('ann', 'accepted')]);
    await after;
    first.answer(server.members);

    // The earlier list resolves to what the cache holds, not to its own
    // out-of-date response.
    expect((await before).map((m) => m.id)).toEqual(['ann']);
    expect(service.memberCount()).toBe(1);
  });

  it('drops a list that comes back after the organization is gone', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);

    server.holdMembers = true;
    const listing = service.reloadMembers();
    service.clearAll();
    server.pendingMembers[0].answer(server.members);

    await expect(listing).rejects.toThrow('organization changed');
    expect(service.members()).toEqual([]);
    expect(service.memberCount()).toBeNull();
  });

  it('fetches again when a sheet reports a change, whatever page is open', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);

    server.members = [...server.members, member('eve', 'accepted')];
    service.membersChanged.update((count) => count + 1);
    TestBed.tick();

    await vi.waitFor(() => expect(service.memberCount()).toBe(4));
  });

  it('does not report a change-driven fetch that crossed an organization switch', async () => {
    const { service, server } = build([alpha]);
    await loaded(service);
    const reported = vi.spyOn(console, 'error').mockImplementation(() => {});

    server.holdMembers = true;
    service.membersChanged.update((count) => count + 1);
    TestBed.tick();
    service.clearAll();
    server.pendingMembers[0].answer(server.members);
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });

    expect(reported).not.toHaveBeenCalled();
    expect(service.memberCount()).toBeNull();
  });
});
