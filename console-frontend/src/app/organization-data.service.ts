import { Injectable, inject, signal, computed } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { type Timestamp } from '@bufbuild/protobuf/wkt';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION, CLUSTER, PROJECT } from '../connect/tokens';
import { GetOrganizationRequestSchema, type Organization } from '../generated/v1/organization_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../generated/v1/cluster_pb';
import { ListProjectsRequestSchema } from '../generated/v1/project_pb';

export interface ProjectData {
  id: string;
  name: string;
}

export interface ClusterData {
  id: string;
  name: string;
  projects: ProjectData[];
}

export interface OrganizationData {
  id: string;
  name: string;
  displayName: string;
  created?: Timestamp;
  clusters: ClusterData[];
}

@Injectable({
  providedIn: 'root',
})
export class OrganizationDataService {
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

  private loadProjectsPromise: Promise<void> | null = null;

  /** True once loadProjectsAndNamespaces() has completed successfully for the current org. */
  projectsLoaded = signal(false);

  async loadOrganizationData(organizationId?: string) {
    const orgId = organizationId ?? this.cachedOrganizationId;
    if (!orgId) return;
    this.cachedOrganizationId = orgId;

    // Reset project cache so the next loadProjectsAndNamespaces() fetches fresh data.
    this.loadProjectsPromise = null;
    this.projectsLoaded.set(false);

    this.loading.set(true);
    try {
      const orgRequest = create(GetOrganizationRequestSchema, { id: orgId });

      const [orgResponse, clustersResponse] = await Promise.all([
        firstValueFrom(this.organizationClient.getOrganization(orgRequest)),
        firstValueFrom(this.clusterClient.listClusters(create(ListClustersRequestSchema, {}))),
      ]);

      if (!orgResponse.organization) {
        return;
      }

      this.clusterSummaries.set(clustersResponse.clusters);

      const clustersData: ClusterData[] = clustersResponse.clusters.map((cluster) => ({
        id: cluster.id,
        name: cluster.name,
        projects: [],
      }));

      this.organizations.set([
        {
          id: orgResponse.organization.id,
          name: orgResponse.organization.name,
          displayName: orgResponse.organization.displayName,
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
        .then(() => {
          this.projectsLoaded.set(true);
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

  private async doLoadProjects() {
    const orgData = this.organizations()[0];
    if (!orgData) return;

    this.loading.set(true);
    try {
      const clustersData: ClusterData[] = await Promise.all(
        orgData.clusters.map(async (cluster) => {
          const projectsResponse = await firstValueFrom(
            this.projectClient.listProjects(
              create(ListProjectsRequestSchema, { clusterId: cluster.id }),
            ),
          );

          return {
            id: cluster.id,
            name: cluster.name,
            projects: projectsResponse.projects.map((project) => ({
              id: project.id,
              name: project.name,
            })),
          };
        }),
      );

      this.organizations.update((orgs) =>
        orgs.map((org) => (org.id === orgData.id ? { ...org, clusters: clustersData } : org)),
      );
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Error loading project data:', error);
      throw error;
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
   * Update the cached organization name without a full reload
   */
  updateOrganizationDisplayName(organizationId: string, displayName: string) {
    this.organizations.update((orgs) =>
      orgs.map((org) => (org.id === organizationId ? { ...org, displayName } : org)),
    );
    this.userOrganizations.update((orgs) =>
      orgs.map((org) => (org.id === organizationId ? { ...org, displayName } : org)),
    );
  }

  /**
   * Update the cached project name without a full reload
   */
  updateProjectName(projectId: string, name: string) {
    this.organizations.update((orgs) =>
      orgs.map((org) => ({
        ...org,
        clusters: org.clusters.map((c) => ({
          ...c,
          projects: c.projects.map((p) => (p.id === projectId ? { ...p, name } : p)),
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
    this.organizations.set([]);
    this.userOrganizations.set([]);
    this.clusterSummaries.set([]);
    this.loadProjectsPromise = null;
    this.projectsLoaded.set(false);
  }
}
