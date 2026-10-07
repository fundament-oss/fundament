import { Injectable, inject, signal, computed } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { type Timestamp } from '@bufbuild/protobuf/wkt';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION, CLUSTER, PROJECT } from '../connect/tokens';
import OrganizationContextService from './organization-context.service';
import type { RoleBinding } from './utils/mock-role-bindings';
import { GetOrganizationRequestSchema, type Organization } from '../generated/v1/organization_pb';
import {
  ListClustersRequestSchema,
  ListClustersResponse_ClusterSummarySchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../generated/v1/cluster_pb';
import { ClusterStatus } from '../generated/v1/common_pb';
import { ListProjectsRequestSchema } from '../generated/v1/project_pb';
import sortByName from './utils/sort-by-name';

export interface ProjectData {
  id: string;
  name: string;
  alias: string;
  namespaceCount: number;
  memberCount: number;
}

export interface ClusterData {
  id: string;
  name: string;
  projects: ProjectData[];
}

export interface OrganizationData {
  id: string;
  name: string;
  alias: string;
  created?: Timestamp;
  clusters: ClusterData[];
}

/**
 * Minted before a cluster list is fetched and handed back to `setClusters()`
 * with the response. It carries the organization the request went out under and
 * its place in the order the requests were made, so a list that belongs to an
 * organization that is gone — or that a later one has already overtaken — is
 * dropped rather than applied.
 */
export interface ClusterListTicket {
  readonly generation: number;
  readonly seq: number;
}

@Injectable({
  providedIn: 'root',
})
export class OrganizationDataService {
  /** Bumped when a namespace is created or deleted somewhere other than the
   *  list that shows them: the create sheet belongs to the shell, and the
   *  namespace sheet is a child route that leaves the list mounted behind it. */
  readonly namespacesChanged = signal(0);

  /** The same, for members: inviting someone and adding someone to a project
   *  both happen in a sheet the shell owns. */
  readonly membersChanged = signal(0);

  /** And for node pools, which are edited in a sheet over the cluster page that
   *  lists them: a child route, so that page is never unmounted and would go on
   *  showing the pools it fetched when it was opened. */
  readonly nodePoolsChanged = signal(0);

  /** TEMPORARY, dev only: roles have no API, so a fresh grant waits here for the
   *  member id the list assigns. The sheet that hands it out belongs to the
   *  shell, so it cannot park it on the list itself. Delete with the mock
   *  bindings. */
  readonly pendingProjectGrant = signal<{ userId: string; bindings: RoleBinding[] } | null>(null);

  private organizationContext = inject(OrganizationContextService);

  private organizationClient = inject(ORGANIZATION);

  private clusterClient = inject(CLUSTER);

  private projectClient = inject(PROJECT);

  /** Full ClusterSummary list (with status, region, etc.) for the current organization. */
  clusterSummaries = signal<ClusterSummary[]>([]);

  /** All organizations the user belongs to. Lightweight, without nested projects and namespaces. */
  userOrganizations = signal<Organization[]>([]);

  /** Organization data (with clusters) for the currently selected organization. Projects are populated lazily via loadProjects(). */
  organizations = signal<OrganizationData[]>([]);

  loading = signal(false);

  /**
   * What the current organization is called on screen.
   *
   * The alias is the name a person gave it; `name` is the one the address uses
   * and is always there, so it is the fallback. One computed because everything
   * that names the organization to a human has to agree: the sidebar selector,
   * the picker, the mobile back button and now the invitation email.
   *
   * Reads `organizations()`, not `userOrganizations()`. The former is replaced
   * by loadOrganizationData with a single entry for the organization actually
   * selected; the latter is the full membership list used by the picker.
   */
  readonly currentOrganizationDisplayName = computed(() => {
    const id = this.organizationContext.currentOrganizationId();
    const org = id ? this.getOrganizationById(id) : undefined;
    return org?.alias || org?.name || '';
  });

  // Lookup maps for O(1) access
  private projectMap = computed(() => {
    const map = new Map<
      string,
      { project: ProjectData; cluster: ClusterData; organization: OrganizationData }
    >();
    this.organizations().forEach((org) => {
      org.clusters.forEach((cluster) => {
        cluster.projects.forEach((project) => {
          map.set(project.id, { project, cluster, organization: org });
        });
      });
    });
    return map;
  });

  private clusterMap = computed(() => {
    const map = new Map<string, { cluster: ClusterData; organization: OrganizationData }>();
    this.organizations().forEach((org) => {
      org.clusters.forEach((cluster) => {
        map.set(cluster.id, { cluster, organization: org });
      });
    });
    return map;
  });

  private cachedOrganizationId: string | null = null;

  /** Bumped on every organization load and on logout. A response that comes
   *  back under another generation than it left with belongs to a cache that
   *  is gone by now and is dropped. Comparing organization ids alone would let
   *  an A → B → A switch through, and would let a response land after logout. */
  private generation = 0;

  /** Every cluster list ever asked for, and the newest one that made it into
   *  the cache. The clusters page polls on a timer that does not wait for the
   *  response before asking again, and a delete asks in the middle of all that,
   *  so the responses do not necessarily come back in the order they were
   *  asked for. */
  private clusterListRequests = 0;

  private appliedClusterList = 0;

  /** Mint a ticket for a cluster list about to be fetched. Read before asking,
   *  handed back to setClusters() with the response. */
  beginClusterListFetch(): ClusterListTicket {
    this.clusterListRequests += 1;
    return { generation: this.generation, seq: this.clusterListRequests };
  }

  private loadProjectsPromise: Promise<void> | null = null;

  /** True once loadProjectsAndNamespaces() has completed successfully for the current org. */
  projectsLoaded = signal(false);

  /** True once the cluster list has come back for the current org. Anything that
   *  reads "no clusters" as a state rather than as "not fetched yet" waits on this. */
  clustersLoaded = signal(false);

  async loadOrganizationData(organizationId?: string) {
    const orgId = organizationId ?? this.cachedOrganizationId;
    if (!orgId) return;
    this.cachedOrganizationId = orgId;
    this.generation += 1;
    const { generation } = this;

    // Reset project cache so the next loadProjectsAndNamespaces() fetches fresh data.
    this.loadProjectsPromise = null;
    this.projectsLoaded.set(false);
    this.clustersLoaded.set(false);

    this.loading.set(true);
    try {
      const orgRequest = create(GetOrganizationRequestSchema, { id: orgId });

      const [orgResponse, clustersResponse] = await Promise.all([
        firstValueFrom(this.organizationClient.getOrganization(orgRequest)),
        firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
      ]);

      if (!orgResponse.organization || generation !== this.generation) {
        return;
      }

      const clusters = sortByName(clustersResponse.clusters);
      this.clusterSummaries.set(clusters);
      this.clustersLoaded.set(true);

      const clustersData: ClusterData[] = clusters.map((cluster) => ({
        id: cluster.id,
        name: cluster.name,
        projects: [],
      }));

      this.organizations.set([
        {
          id: orgResponse.organization.id,
          name: orgResponse.organization.name,
          alias: orgResponse.organization.alias,
          created: orgResponse.organization.created,
          clusters: clustersData,
        },
      ]);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Error loading organization data:', error);
    } finally {
      this.loading.set(false);
    }
  }

  /**
   * Load projects for all clusters in the current organization.
   * Deduplicates concurrent calls — simultaneous callers share the same in-flight request.
   * Use reloadProjectsAndNamespaces() to force a fresh fetch (e.g. after mutations).
   */
  loadProjectsAndNamespaces(): Promise<void> {
    if (this.projectsLoaded()) {
      return Promise.resolve();
    }
    if (!this.loadProjectsPromise) {
      this.loadProjectsPromise = this.doLoadProjects()
        .then((loaded) => {
          if (loaded) this.projectsLoaded.set(true);
        })
        .finally(() => {
          this.loadProjectsPromise = null;
        });
    }
    return this.loadProjectsPromise;
  }

  /** Force a fresh fetch of projects, bypassing the in-flight deduplication. */
  reloadProjectsAndNamespaces(): Promise<void> {
    this.loadProjectsPromise = null;
    this.projectsLoaded.set(false);
    return this.loadProjectsAndNamespaces();
  }

  /** Resolves false when there is nothing to count as loaded: the organization
   *  itself is not in yet, or a cluster's list failed. A cluster that failed
   *  keeps whatever it had and must be asked again, so a partial result must
   *  not latch `projectsLoaded` — a project on that cluster would stay
   *  unresolvable for the rest of the session. */
  private async doLoadProjects(): Promise<boolean> {
    const orgData = this.organizations()[0];
    if (!orgData) return false;
    const { generation } = this;

    this.loading.set(true);
    try {
      // Settled per cluster: one cluster the user cannot list yet (its authz
      // tuple may still be syncing right after it was created) must not hide
      // the projects of all the others. That cluster keeps what it had.
      const requested = orgData.clusters;
      const results = await Promise.allSettled(
        requested.map((cluster) =>
          firstValueFrom(
            this.projectClient.listProjects(
              create(ListProjectsRequestSchema, { clusterId: cluster.id }),
            ),
          ),
        ),
      );

      if (generation !== this.generation) return false;

      const failures = results.filter((result) => result.status === 'rejected');
      if (failures.length > 0) {
        // eslint-disable-next-line no-console
        console.error(
          'Error loading project data:',
          failures.map((failure) => failure.reason),
        );
        if (failures.length === results.length) throw failures[0].reason;
      }

      const loaded = new Map<string, ProjectData[]>();
      requested.forEach((cluster, i) => {
        const result = results[i];
        if (result.status === 'rejected') return;

        loaded.set(
          cluster.id,
          result.value.projects.map((project) => ({
            id: project.id,
            name: project.name,
            alias: project.alias,
            namespaceCount: project.namespaceCount,
            memberCount: project.memberCount,
          })),
        );
      });

      // Merged into the list as it is now, not the one the requests went out
      // with: reloadClusters() may have put a cluster in while they were in
      // flight, and replacing the list wholesale would take it out again. A
      // cluster that was not asked keeps what it has, which for a cluster that
      // new is the empty list it was created with.
      this.organizations.update((orgs) =>
        orgs.map((org) => {
          if (org.id !== orgData.id) return org;

          return {
            ...org,
            clusters: org.clusters.map((cluster) => {
              const projects = loaded.get(cluster.id);
              return projects ? { ...cluster, projects } : cluster;
            }),
          };
        }),
      );
      return failures.length === 0;
    } finally {
      this.loading.set(false);
    }
  }

  /**
   * Get project by ID with its parent cluster and organization (O(1) lookup)
   */
  getProjectById(projectId: string) {
    return this.projectMap().get(projectId);
  }

  /**
   * Get cluster by ID with its parent organization (O(1) lookup)
   */
  getClusterById(clusterId: string) {
    return this.clusterMap().get(clusterId);
  }

  /**
   * Get organization by ID (O(n) lookup, but typically only one organization)
   */
  getOrganizationById(organizationId: string) {
    return this.organizations().find((org) => org.id === organizationId);
  }

  /**
   * Refresh the cluster list from the server after a cluster was created or
   * deleted, so every view built on the cache (Projects, plugins, the sidebar)
   * shows it in its place. Projects already loaded for the other clusters stay
   * in place.
   *
   * The server is the one that says what the list holds, in both directions. A
   * created cluster exists once the create came back, and the cache has to know
   * it either way: the page it lands on takes its title and breadcrumb from
   * here, the sidebar counts it, and the new-cluster form checks names against
   * it. A delete is a soft one, so the opposite holds — ListClusters goes on
   * returning the cluster as DELETING until it is really gone, and taking the
   * entry out by hand makes the list disagree with the server: the cluster is
   * back on the next page load, and in the meantime nothing in the list is
   * transitional, so the clusters page never starts the poll that would have
   * reported the deletion finishing.
   *
   * `fallback` is the cluster the call that just succeeded was about, and the
   * status it put it in. It is used only if the list cannot be fetched, so that
   * the cache still reflects what happened; the next poll fills in the rest.
   */
  async reloadClusters(fallback?: { id: string; name: string; status: ClusterStatus }) {
    if (!this.cachedOrganizationId) return;
    const ticket = this.beginClusterListFetch();

    let clusters: ClusterSummary[];
    try {
      const response = await firstValueFrom(
        this.clusterClient.listClusters(create(ListClustersRequestSchema, {})),
      );
      clusters = response.clusters;
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Error reloading clusters:', error);
      clusters = this.clustersWithFallback(fallback);
    }

    this.applyClusters(clusters, ticket);
  }

  /**
   * Take a cluster list someone else has just fetched as the cache's own. The
   * clusters page polls for one every few seconds while a cluster is still
   * moving, and it is the only thing watching: without this the cache would go
   * on holding a cluster that has finished deleting until the next full
   * organization load, and the sidebar counts clusters and groups the projects
   * under them.
   *
   * `ticket` is the one `beginClusterListFetch()` handed out before the list was
   * asked for. Returns whether the list was applied: false means a newer one
   * already was, or the organization it belongs to is gone, and the cache holds
   * something the caller should prefer to its own response.
   */
  setClusters(clusters: readonly ClusterSummary[], ticket: ClusterListTicket): boolean {
    return this.applyClusters([...clusters], ticket);
  }

  private applyClusters(clusters: ClusterSummary[], ticket: ClusterListTicket): boolean {
    const activeOrgId = this.cachedOrganizationId;
    // The user switched organization or logged out while the request was in
    // flight, so this list belongs to a cache that is gone; or a list asked for
    // later has landed first, which leaves this one behind the cache rather
    // than ahead of it.
    if (!activeOrgId || ticket.generation !== this.generation) return false;
    if (ticket.seq <= this.appliedClusterList) return false;
    this.appliedClusterList = ticket.seq;

    const sorted = sortByName(clusters);
    this.clusterSummaries.set(sorted);
    this.organizations.update((orgs) =>
      orgs.map((org) => {
        if (org.id !== activeOrgId) return org;

        const known = new Map(org.clusters.map((c) => [c.id, c]));
        return {
          ...org,
          clusters: sorted.map(
            (cluster) =>
              known.get(cluster.id) ?? { id: cluster.id, name: cluster.name, projects: [] },
          ),
        };
      }),
    );
    return true;
  }

  /** The list as it stands with `fallback` applied, for when ListClusters could
   *  not be reached. A cluster that is already in the list keeps the region and
   *  the counts it was listed with and only takes the new status; one that is
   *  not there yet — a cluster just created — goes in with the little the
   *  caller knows about it. */
  private clustersWithFallback(
    fallback: { id: string; name: string; status: ClusterStatus } | undefined,
  ): ClusterSummary[] {
    const current = this.clusterSummaries();
    if (!fallback) return current;

    if (current.some((cluster) => cluster.id === fallback.id)) {
      return current.map((cluster) =>
        cluster.id === fallback.id ? { ...cluster, status: fallback.status } : cluster,
      );
    }

    return [
      ...current,
      create(ListClustersResponse_ClusterSummarySchema, {
        id: fallback.id,
        name: fallback.name,
        status: fallback.status,
      }),
    ];
  }

  /**
   * Update the cached organization name without a full reload
   */
  updateOrganizationAlias(organizationId: string, alias: string) {
    this.organizations.update((orgs) =>
      orgs.map((org) => (org.id === organizationId ? { ...org, alias } : org)),
    );
    this.userOrganizations.update((orgs) =>
      orgs.map((org) => (org.id === organizationId ? { ...org, alias } : org)),
    );
  }

  /**
   * Update the cached project alias without a full reload
   */
  updateProjectAlias(projectId: string, alias: string) {
    this.organizations.update((orgs) =>
      orgs.map((org) => ({
        ...org,
        clusters: org.clusters.map((c) => ({
          ...c,
          projects: c.projects.map((p) => (p.id === projectId ? { ...p, alias } : p)),
        })),
      })),
    );
  }

  /**
   * Set the list of all organizations the user belongs to, without nested projects and namespaces.
   */
  setUserOrganizations(orgs: Organization[]) {
    this.userOrganizations.set(orgs);
  }

  /**
   * Clear all organization data (used on logout).
   */
  clearAll() {
    this.generation += 1;
    this.organizations.set([]);
    this.userOrganizations.set([]);
    this.clusterSummaries.set([]);
    this.clustersLoaded.set(false);
    this.loadProjectsPromise = null;
    this.projectsLoaded.set(false);
  }
}
