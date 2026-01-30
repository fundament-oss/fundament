import { Injectable, inject, signal } from '@angular/core';
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
      const projectsResponse = await firstValueFrom(this.projectClient.listProjects(projectsRequest));

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
}
