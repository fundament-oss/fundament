import { Injectable, inject, signal, computed } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION, CLUSTER, NAMESPACE, PROJECT } from '../connect/tokens';
import { GetOrganizationRequestSchema, type Organization } from '../generated/v1/organization_pb';
import { ListClustersRequestSchema } from '../generated/v1/cluster_pb';
import { ListProjectsRequestSchema } from '../generated/v1/project_pb';
import { ListProjectNamespacesRequestSchema } from '../generated/v1/namespace_pb';

export interface NamespaceData {
  id: string;
  name: string;
}

export interface ProjectData {
  id: string;
  name: string;
  namespaces: NamespaceData[];
}

export interface ClusterData {
  id: string;
  name: string;
  projects: ProjectData[];
}

export interface OrganizationData {
  id: string;
  name: string;
  clusters: ClusterData[];
}

@Injectable({
  providedIn: 'root',
})
export class OrganizationDataService {
  private organizationClient = inject(ORGANIZATION);

  private clusterClient = inject(CLUSTER);

  private projectClient = inject(PROJECT);

  private namespaceClient = inject(NAMESPACE);

  /** All organizations the user belongs to. Lightweight, without nested projects and namespaces. */
  userOrganizations = signal<Organization[]>([]);

  /** Full data (with clusters, projects and namespaces) for the currently selected organization. */
  organizations = signal<OrganizationData[]>([]);

  loading = signal(false);

  // Lookup maps for O(1) access
  private namespaceMap = computed(() => {
    const map = new Map<
      string,
      {
        namespace: NamespaceData;
        project: ProjectData;
        cluster: ClusterData;
        organization: OrganizationData;
      }
    >();
    this.organizations().forEach((org) => {
      org.clusters.forEach((cluster) => {
        cluster.projects.forEach((project) => {
          project.namespaces.forEach((namespace) => {
            map.set(namespace.id, { namespace, project, cluster, organization: org });
          });
        });
      });
    });
    return map;
  });

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

  async loadOrganizationData(organizationId?: string) {
    const orgId = organizationId ?? this.cachedOrganizationId;
    if (!orgId) return;
    this.cachedOrganizationId = orgId;

    this.loading.set(true);
    try {
      // Fetch organization and clusters in parallel
      const orgRequest = create(GetOrganizationRequestSchema, { id: orgId });
      const clustersRequest = create(ListClustersRequestSchema, {});

      const [orgResponse, clustersResponse] = await Promise.all([
        firstValueFrom(this.organizationClient.getOrganization(orgRequest)),
        firstValueFrom(this.clusterClient.listClusters(clustersRequest)),
      ]);

      if (!orgResponse.organization) {
        return;
      }

      // For each cluster, fetch its projects, then for each project fetch namespaces
      const clustersData: ClusterData[] = await Promise.all(
        clustersResponse.clusters.map(async (cluster) => {
          const projectsRequest = create(ListProjectsRequestSchema, {
            clusterId: cluster.id,
          });
          const projectsResponse = await firstValueFrom(
            this.projectClient.listProjects(projectsRequest),
          );

          const projects: ProjectData[] = await Promise.all(
            projectsResponse.projects.map(async (project) => {
              const namespacesRequest = create(ListProjectNamespacesRequestSchema, {
                projectId: project.id,
              });
              const namespacesResponse = await firstValueFrom(
                this.namespaceClient.listProjectNamespaces(namespacesRequest),
              );

              return {
                id: project.id,
                name: project.name,
                namespaces: namespacesResponse.namespaces.map((ns) => ({
                  id: ns.id,
                  name: ns.name,
                })),
              };
            }),
          );

          return {
            id: cluster.id,
            name: cluster.name,
            projects,
          };
        }),
      );

      // Build the organization data
      const organizationData: OrganizationData = {
        id: orgResponse.organization.id,
        name: orgResponse.organization.name,
        clusters: clustersData,
      };

      this.organizations.set([organizationData]);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Error loading organization data:', error);
    } finally {
      this.loading.set(false);
    }
  }

  /**
   * Get namespace by ID with its parent project, cluster and organization (O(1) lookup)
   */
  getNamespaceById(namespaceId: string) {
    return this.namespaceMap().get(namespaceId);
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
  updateOrganizationName(organizationId: string, name: string) {
    this.organizations.update((orgs) =>
      orgs.map((org) => (org.id === organizationId ? { ...org, name } : org)),
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
  }
}
