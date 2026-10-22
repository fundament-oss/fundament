import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { ProjectMemberRole } from '../support/api/project-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { authenticateWithPassword } from './common.steps.ts';

// Seeded users' ids, resolved once per run from the `sub` claim of their JWT.
const userIdsByEmail = new Map<string, string>();

async function userIdByEmail(email: string): Promise<string> {
  const cached = userIdsByEmail.get(email);
  if (cached) {
    return cached;
  }
  const token = await authenticateWithPassword(email);
  const payload = JSON.parse(
    Buffer.from(token.split('.')[1], 'base64url').toString(),
  );
  const sub: string | undefined = payload.sub;
  if (!sub) {
    throw new Error(`JWT for ${email} has no sub claim`);
  }
  userIdsByEmail.set(email, sub);
  return sub;
}

function roleFromName(role: string): ProjectMemberRole {
  switch (role) {
    case 'admin':
      return ProjectMemberRole.ADMIN;
    case 'viewer':
      return ProjectMemberRole.VIEWER;
    default:
      throw new Error(`unknown project member role ${role}`);
  }
}

Given(
  'I have created a project named {string} on cluster {string}',
  async function (this: ICustomWorld, name: string, clusterName: string) {
    const cluster = await this.clusterService!.getClusterByName(clusterName);
    if (!cluster) {
      throw new Error(`cluster ${clusterName} not found in the organization`);
    }
    // Project names are unique per cluster: a suffix keeps a leftover from an
    // aborted run from failing the next one (30 chars max, see the proto).
    const uniqueName = `${name}-${crypto.randomUUID().slice(0, 8)}`;
    const response = await this.projectService!.createProject(
      cluster.id,
      uniqueName,
    );
    this.currentProjectId = response.projectId;
    this.createdProjects.push({
      id: response.projectId,
      ownerEmail: this.currentUserEmail!,
    });
  },
);

When(
  'I add the user {string} to the project with role {string}',
  async function (this: ICustomWorld, email: string, role: string) {
    const userId = await userIdByEmail(email);
    try {
      this.lastApiResponse = await this.projectService!.addProjectMember(
        this.currentProjectId!,
        userId,
        roleFromName(role),
      );
      this.lastApiError = undefined;
    } catch (error) {
      this.lastApiError = error as Error;
      this.lastApiResponse = undefined;
    }
  },
);

Then(
  'I should receive a failed precondition error',
  async function (this: ICustomWorld) {
    expect(this.lastApiError).toBeDefined();
    expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
    expect((this.lastApiError as ConnectRpcError).code).toBe(
      'failed_precondition',
    );
  },
);

Then(
  'the project should have the member {string}',
  async function (this: ICustomWorld, email: string) {
    const userId = await userIdByEmail(email);
    const members = await this.projectService!.listProjectMembers(
      this.currentProjectId!,
    );
    expect(members.members.map((m) => m.userId)).toContain(userId);
  },
);

Then(
  'the project should not have the member {string}',
  async function (this: ICustomWorld, email: string) {
    const userId = await userIdByEmail(email);
    const members = await this.projectService!.listProjectMembers(
      this.currentProjectId!,
    );
    expect(members.members.map((m) => m.userId)).not.toContain(userId);
  },
);
