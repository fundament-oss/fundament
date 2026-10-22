/**
 * ProjectService client for organization-api.
 * Uses generated proto types with Connect RPC.
 */

import { type Client, ConnectError } from '@connectrpc/connect';
import { createServiceClient, ConnectRpcError } from './client.ts';
import {
  ProjectService as ProjectServiceDesc,
  ProjectMemberRole,
  type CreateProjectResponse,
  type ListProjectMembersResponse,
} from '../generated/v1/project_pb.ts';

export { ProjectMemberRole };
export type { CreateProjectResponse, ListProjectMembersResponse };

export class ProjectService {
  private client: Client<typeof ProjectServiceDesc>;

  constructor(baseUrl: string, authToken: string, organizationId?: string) {
    this.client = createServiceClient(
      ProjectServiceDesc,
      baseUrl,
      authToken,
      organizationId,
    );
  }

  async createProject(
    clusterId: string,
    name: string,
  ): Promise<CreateProjectResponse> {
    return this.wrap(() => this.client.createProject({ clusterId, name }));
  }

  async deleteProject(projectId: string): Promise<void> {
    await this.wrap(() => this.client.deleteProject({ projectId }));
  }

  async addProjectMember(
    projectId: string,
    userId: string,
    role: ProjectMemberRole,
  ): Promise<string> {
    const response = await this.wrap(() =>
      this.client.addProjectMember({ projectId, userId, role }),
    );
    return response.memberId;
  }

  async listProjectMembers(
    projectId: string,
  ): Promise<ListProjectMembersResponse> {
    return this.wrap(() => this.client.listProjectMembers({ projectId }));
  }

  private async wrap<T>(call: () => Promise<T>): Promise<T> {
    try {
      return await call();
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }
}
