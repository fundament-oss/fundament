import { Injectable, inject, signal, computed } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION, PROJECT } from '../connect/tokens';
import { GetOrganizationRequestSchema } from '../generated/v1/organization_pb';
import {
  ListProjectsRequestSchema,
  ListProjectNamespacesRequestSchema,
} from '../generated/v1/project_pb';

export interface NamespaceData {
  id: string;
  name: string;
}

export interface ProjectData {
  id: string;
  name: string;
  namespaces: NamespaceData[];
}

export interface OrganizationData {
  id: string;
  name: string;
  projects: ProjectData[];
}

@Injectable({
  providedIn: 'root',
})
export class OrganizationDataService {
  private organizationClient = inject(ORGANIZATION);

  private projectClient = inject(PROJECT);

  organizations = signal<OrganizationData[]>([]);

  loading = signal(false);

  // Lookup maps for O(1) access
  private namespaceMap = computed(() => {
    const map = new Map<
      string,
      { namespace: NamespaceData; project: ProjectData; organization: OrganizationData }
    >();
    this.organizations().forEach((org) => {
      org.projects.forEach((project) => {
        project.namespaces.forEach((namespace) => {
          map.set(namespace.id, { namespace, project, organization: org });
        });
      });
    });
    return map;
  });

  private projectMap = computed(() => {
    const map = new Map<string, { project: ProjectData; organization: OrganizationData }>();
    this.organizations().forEach((org) => {
      org.projects.forEach((project) => {
        map.set(project.id, { project, organization: org });
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
      // Parallelize organization and projects requests (they don't depend on each other)
      const orgRequest = create(GetOrganizationRequestSchema, {
        id: orgId,
      });
      const projectsRequest = create(ListProjectsRequestSchema, {});

      const [orgResponse, projectsResponse] = await Promise.all([
        firstValueFrom(this.organizationClient.getOrganization(orgRequest)),
        firstValueFrom(this.projectClient.listProjects(projectsRequest)),
      ]);

      if (!orgResponse.organization) {
        return;
      }

      // For each project, get its namespaces
      const projectsData: ProjectData[] = await Promise.all(
        projectsResponse.projects.map(async (project) => {
          const namespacesRequest = create(ListProjectNamespacesRequestSchema, {
            projectId: project.id,
          });
          const namespacesResponse = await firstValueFrom(
            this.projectClient.listProjectNamespaces(namespacesRequest),
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

      // Build the organization data
      const organizationData: OrganizationData = {
        id: orgResponse.organization.id,
        name: orgResponse.organization.name,
        projects: projectsData,
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
   * Get namespace by ID with its parent project and organization (O(1) lookup)
   */
  getNamespaceById(namespaceId: string) {
    return this.namespaceMap().get(namespaceId);
  }

  /**
   * Get project by ID with its parent organization (O(1) lookup)
   */
  getProjectById(projectId: string) {
    return this.projectMap().get(projectId);
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
        projects: org.projects.map((p) => (p.id === projectId ? { ...p, name } : p)),
      })),
    );
  }
}
