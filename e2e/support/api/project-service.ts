/**
 * Project, namespace and member clients for organization-api: what a scenario
 * needs to give a user a role in a project namespace.
 * Uses generated proto types with Connect RPC.
 */

import { type Client, ConnectError } from '@connectrpc/connect';
import {
  createServiceClient,
  createWithIdempotency,
  ConnectRpcError,
  IDEMPOTENCY_KEY_HEADER,
  IDEMPOTENCY_STATUS_HEADER,
} from './client.ts';
import {
  ProjectService as ProjectServiceDesc,
  ProjectMemberRole,
} from '../generated/v1/project_pb.ts';
import { NamespaceService as NamespaceServiceDesc } from '../generated/v1/namespace_pb.ts';
import { MemberService as MemberServiceDesc } from '../generated/v1/member_pb.ts';

export { ProjectMemberRole };

async function rpc<T>(call: () => Promise<T>): Promise<T> {
  try {
    return await call();
  } catch (err) {
    if (err instanceof ConnectError) {
      throw ConnectRpcError.fromConnectError(err);
    }
    throw err;
  }
}

// Calls a create RPC with an Idempotency-Key until the server reports it done.
function idempotent<Resp>(
  call: (opts: { headers: Record<string, string>; onHeader: (h: Headers) => void }) => Promise<Resp>,
): Promise<Resp> {
  return rpc(() =>
    createWithIdempotency(async (idempotencyKey) => {
      let status = '';
      const response = await call({
        headers: { [IDEMPOTENCY_KEY_HEADER]: idempotencyKey },
        onHeader(headers) {
          status = headers.get(IDEMPOTENCY_STATUS_HEADER) ?? '';
        },
      });
      return { response, status };
    }),
  );
}

export class ProjectService {
  private projects: Client<typeof ProjectServiceDesc>;
  private namespaces: Client<typeof NamespaceServiceDesc>;
  private members: Client<typeof MemberServiceDesc>;

  constructor(baseUrl: string, authToken: string, organizationId?: string) {
    this.projects = createServiceClient(ProjectServiceDesc, baseUrl, authToken, organizationId);
    this.namespaces = createServiceClient(NamespaceServiceDesc, baseUrl, authToken, organizationId);
    this.members = createServiceClient(MemberServiceDesc, baseUrl, authToken, organizationId);
  }

  async createProject(clusterId: string, name: string): Promise<string> {
    const response = await idempotent((opts) => this.projects.createProject({ clusterId, name }, opts));
    return response.projectId;
  }

  async deleteProject(projectId: string): Promise<void> {
    await rpc(() => this.projects.deleteProject({ projectId }));
  }

  async createNamespace(projectId: string, name: string): Promise<string> {
    const response = await idempotent((opts) => this.namespaces.createNamespace({ projectId, name }, opts));
    return response.namespaceId;
  }

  async deleteNamespace(namespaceId: string): Promise<void> {
    await rpc(() => this.namespaces.deleteNamespace({ namespaceId }));
  }

  async addProjectMember(projectId: string, userId: string, role: ProjectMemberRole): Promise<string> {
    const response = await idempotent((opts) => this.projects.addProjectMember({ projectId, userId, role }, opts));
    return response.memberId;
  }

  async removeProjectMember(memberId: string): Promise<void> {
    await rpc(() => this.projects.removeProjectMember({ memberId }));
  }

  // The user id of the organization member with this email.
  async userIdByEmail(email: string): Promise<string> {
    const { members } = await rpc(() => this.members.listMembers({}));
    const member = members.find((m) => m.email === email);
    if (!member) {
      throw new Error(`no organization member with email ${email}`);
    }
    return member.userId;
  }
}
