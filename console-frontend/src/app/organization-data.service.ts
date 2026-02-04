import { Injectable, inject, signal, computed } from '@angular/core';
import { AUTHN, ORGANIZATION, PROJECT } from '../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { GetOrganizationRequestSchema } from '../generated/v1/organization_pb';
import {
  ListProjectsRequestSchema,
  ListProjectNamespacesRequestSchema,
} from '../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';

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
  private authnClient = inject(AUTHN);
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
    for (const org of this.organizations()) {
      for (const project of org.projects) {
        for (const namespace of project.namespaces) {
          map.set(namespace.id, { namespace, project, organization: org });
        }
      }
    }
    return map;
  });

  private projectMap = computed(() => {
    const map = new Map<string, { project: ProjectData; organization: OrganizationData }>();
    for (const org of this.organizations()) {
      for (const project of org.projects) {
        map.set(project.id, { project, organization: org });
      }
    }
    return map;
  });

  async loadOrganizationData() {
    this.loading.set(true);
    try {
      // Get current user to retrieve organization ID
      const userResponse = await firstValueFrom(this.authnClient.getUserInfo({}));
      if (!userResponse.user?.organizationId) {
        return;
      }

      // Get organization details
      const orgRequest = create(GetOrganizationRequestSchema, {
        id: userResponse.user.organizationId,
      });
      const orgResponse = await firstValueFrom(this.organizationClient.getOrganization(orgRequest));

      if (!orgResponse.organization) {
        return;
      }

      // Get all projects for the organization
      const projectsRequest = create(ListProjectsRequestSchema, {});
      const projectsResponse = await firstValueFrom(
        this.projectClient.listProjects(projectsRequest),
      );

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
      console.error('Error loading organization data:', error);
    } finally {
      this.loading.set(false);
    }
  }

  async reloadOrganizationData() {
    await this.loadOrganizationData();
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
}
