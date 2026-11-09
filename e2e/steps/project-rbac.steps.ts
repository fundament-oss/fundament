/**
 * Steps for the @project-rbac and @project-namespaces features: project
 * namespaces and roles become namespaces and RoleBindings on a real cluster,
 * so kubectl through the kube-api-proxy allows exactly what the role grants.
 * Runs in the test-env VM (`just test-env test`).
 */

import { Given, When, Then, After } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { clusterName } from '../support/fixtures.ts';
import { ClusterService } from '../support/api/cluster-service.ts';
import { ProjectMemberRole, ProjectService } from '../support/api/project-service.ts';
import { authenticateWithPassword, extractOrganizationId } from '../support/auth.ts';
import { type Kubectl, type UserSession, withClusterAccess } from '../support/kube.ts';
import { pollUntil } from '../support/poll.ts';
import { withShootAdmin } from '../support/shoot.ts';


interface RbacState {
  owner: UserSession;
  clusterId: string;
  projectId: string;
  projectName: string;
  // By project namespace name, while it exists.
  namespaceIds: Map<string, string>;
  // By email.
  memberIds: Map<string, string>;
}

function state(world: ICustomWorld): RbacState {
  return world.testData.rbac as RbacState;
}

function projects(session: UserSession): ProjectService {
  return new ProjectService(session.organizationApiUrl, session.authToken, session.organizationId);
}

// The cluster-side name of a project namespace (kubename.GenerateNamespace).
function clusterNamespace(world: ICustomWorld, name: string): string {
  return `tnt-${state(world).projectName}--${name}`;
}

async function asUser<T>(world: ICustomWorld, email: string, use: (kubectl: Kubectl) => Promise<T>): Promise<T> {
  const authToken = await authenticateWithPassword(email);
  const session = { organizationApiUrl: world.organizationApiUrl!, authToken, organizationId: extractOrganizationId(authToken) };
  return withClusterAccess(session, state(world).clusterId, use);
}

// A project goes only once its namespaces are gone; deleting them removes the
// namespaces and their bindings from the cluster.
After({ timeout: 30_000 }, async function (this: ICustomWorld) {
  const rbac = this.testData.rbac as RbacState | undefined;
  if (!rbac) {
    return;
  }
  const service = projects(rbac.owner);
  for (const id of rbac.namespaceIds.values()) {
    await service.deleteNamespace(id).catch((err) => console.error(`cleanup: failed to delete namespace ${id}`, err));
  }
  await service
    .deleteProject(rbac.projectId)
    .catch((err) => console.error(`cleanup: failed to delete project ${rbac.projectName}`, err));
});

Given(
  'a new project on the baseline cluster with a namespace {string}',
  { timeout: 60_000 },
  async function (this: ICustomWorld, namespace: string) {
    const owner = { organizationApiUrl: this.organizationApiUrl!, authToken: this.authToken!, organizationId: this.organizationId };
    const cluster = await new ClusterService(owner.organizationApiUrl, owner.authToken, owner.organizationId).getClusterByName(clusterName());
    expect(cluster, `no cluster named ${clusterName()}`).toBeDefined();
    const projectName = `e2e-rb-${Date.now().toString(36)}`;
    const projectId = await projects(owner).createProject(cluster!.id, projectName);
    const rbac: RbacState = {
      owner,
      clusterId: cluster!.id,
      projectId,
      projectName,
      namespaceIds: new Map(),
      memberIds: new Map(),
    };
    this.testData.rbac = rbac;
    rbac.namespaceIds.set(namespace, await projects(owner).createNamespace(projectId, namespace));
  },
);

When(
  'I add {string} to the project as a(n) {word}',
  { timeout: 30_000 },
  async function (this: ICustomWorld, email: string, role: string) {
    const roles: Record<string, ProjectMemberRole> = { viewer: ProjectMemberRole.VIEWER, admin: ProjectMemberRole.ADMIN };
    expect(roles[role], `unknown project role ${role}`).toBeDefined();
    const service = projects(state(this).owner);
    const memberId = await service.addProjectMember(state(this).projectId, await service.userIdByEmail(email), roles[role]);
    state(this).memberIds.set(email, memberId);
  },
);

When('I remove {string} from the project', { timeout: 30_000 }, async function (this: ICustomWorld, email: string) {
  const memberId = state(this).memberIds.get(email);
  expect(memberId, `${email} was not added in this scenario`).toBeDefined();
  await projects(state(this).owner).removeProjectMember(memberId!);
});

When('I add the namespace {string} to the project', { timeout: 30_000 }, async function (this: ICustomWorld, namespace: string) {
  const rbac = state(this);
  rbac.namespaceIds.set(namespace, await projects(rbac.owner).createNamespace(rbac.projectId, namespace));
});

When('I delete the namespace {string} from the project', { timeout: 30_000 }, async function (this: ICustomWorld, namespace: string) {
  const rbac = state(this);
  const id = rbac.namespaceIds.get(namespace);
  expect(id, `namespace ${namespace} was not created in this scenario`).toBeDefined();
  await projects(rbac.owner).deleteNamespace(id!);
  rbac.namespaceIds.delete(namespace);
});

// What the shoot itself holds, seen as its admin rather than through a user's
// RBAC.
Then(
  'within {int} minutes the cluster has the namespace {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await withShootAdmin(state(this).clusterId, (kubectl) =>
      pollUntil(minutes * 60_000, `namespace ${ns} on the cluster`, async () =>
        kubectl(['get', 'namespace', ns, '--ignore-not-found']).trim() || undefined,
      ),
    );
  },
);

Then(
  'within {int} minutes the cluster no longer has the namespace {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await withShootAdmin(state(this).clusterId, (kubectl) =>
      pollUntil(minutes * 60_000, `namespace ${ns} to leave the cluster`, async () =>
        kubectl(['get', 'namespace', ns, '--ignore-not-found']).trim() ? undefined : true,
      ),
    );
  },
);

// No cucumber timeout: pollUntil enforces the minutes the feature names. The
// namespace and the binding reach the cluster through cluster-worker.
Then(
  'within {int} minutes {string} can list config maps in {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, email: string, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await asUser(this, email, (kubectl) =>
      pollUntil(minutes * 60_000, `${email} lists config maps in ${ns}`, async () => {
        try {
          return kubectl(['get', 'configmaps', '-n', ns]);
        } catch {
          return undefined;
        }
      }),
    );
  },
);

Then(
  'within {int} minutes {string} can no longer list config maps in {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, email: string, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    // Lost access is a refusal (docs/user/clusters.md, "What the kubeconfig
    // grants"); any other answer keeps the wait going and is reported.
    let last = 'no answer';
    await asUser(this, email, (kubectl) =>
      pollUntil(minutes * 60_000, `${email} to lose access to ${ns}`, async () => {
        try {
          kubectl(['get', 'configmaps', '-n', ns]);
          last = 'the list succeeded';
          return undefined;
        } catch (err) {
          last = (err as Error).message.split('\n').at(-1) ?? '';
          return /forbidden/i.test(last) ? true : undefined;
        }
      }).catch((err) => {
        throw new Error(`${(err as Error).message} (last: ${last})`);
      }),
    );
  },
);

Then(
  '{string} cannot list config maps in {string}',
  { timeout: 120_000 },
  async function (this: ICustomWorld, email: string, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await asUser(this, email, async (kubectl) => {
      expect(() => kubectl(['get', 'configmaps', '-n', ns])).toThrow(/forbidden/i);
    });
  },
);

Then(
  '{string} cannot create a config map in {string}',
  { timeout: 120_000 },
  async function (this: ICustomWorld, email: string, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await asUser(this, email, async (kubectl) => {
      expect(() => kubectl(['create', 'configmap', 'e2e-viewer', '-n', ns])).toThrow(/forbidden/i);
    });
  },
);

Then(
  'within {int} minutes {string} can create a config map in {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, email: string, namespace: string) {
    const ns = clusterNamespace(this, namespace);
    await asUser(this, email, (kubectl) =>
      pollUntil(minutes * 60_000, `${email} creates a config map in ${ns}`, async () => {
        try {
          return kubectl(['create', 'configmap', 'e2e-admin', '-n', ns]);
        } catch {
          return undefined;
        }
      }),
    );
  },
);
