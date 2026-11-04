import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { create } from '@bufbuild/protobuf';
import { ConnectError, Code } from '@connectrpc/connect';
import { ICustomWorld } from '../support/world.ts';
import { LoginPage } from '../support/pages/login.page.ts';
import { createServiceClient } from '../support/api/client.ts';
import { AUTHZ_READY_TIMEOUT_MS } from './common.steps.ts';
import { Action } from '../support/generated/v1/actions_pb.ts';
import { AuthzService, PermissionCheckSchema, PermissionDeniedDetailSchema, RequiredRole } from '../support/generated/v1/authz_pb.ts';
import { ClusterService } from '../support/generated/v1/cluster_pb.ts';
import { ProjectService } from '../support/generated/v1/project_pb.ts';
import { LogsService } from '../support/generated/v1/logs_pb.ts';
import { NamespaceService } from '../support/generated/v1/namespace_pb.ts';

// Seeded in db/testdata (001: acme-cluster, 003: acme-project).
const SEEDED_IDS: Record<string, string> = {
  cluster: '019b4000-2000-7000-8000-000000000001',
  project: '019b4000-9000-7000-8000-000000000001',
};

const CHECK_PERMISSIONS_PATH = '/authz.v1.AuthzService/CheckPermissions';
// The console asks again 1 s and 4 s after the first answer while any is false
// (console-frontend/src/app/utils/permission-checks.ts), a longest gap of 3 s,
// and the shell and each page run their own watchers. No answer for longer
// than that means every re-ask is done.
const QUIET_AFTER_ANSWERS_MS = 3500;
const PAGE_TIMEOUT_MS = 30_000;

interface Denial {
  error?: ConnectError;
  elapsedMs: number;
}

const denials = new WeakMap<object, Denial>();
// Times of the CheckPermissions answers the console received since the page's last navigation.
const answerTimes = new WeakMap<object, number[]>();

function seededId(object: string): string {
  const id = SEEDED_IDS[object];
  if (!id) {
    throw new Error(`Unknown seeded object: ${object}`);
  }
  return id;
}

function actionByName(name: string): Action {
  const action = Action[name as keyof typeof Action];
  if (action === undefined) {
    throw new Error(`Unknown action: ${name}`);
  }
  return action;
}

function roleByName(name: string): RequiredRole {
  const role = RequiredRole[name as keyof typeof RequiredRole];
  if (role === undefined) {
    throw new Error(`Unknown role: ${name}`);
  }
  return role;
}

async function pollAllowed(
  world: ICustomWorld,
  checks: { action: Action; object: string }[],
  expected: boolean,
) {
  const client = createServiceClient(AuthzService, world.authzApiUrl!, world.authToken);
  const request = checks.map((c) =>
    create(PermissionCheckSchema, { action: c.action, objectId: seededId(c.object) }),
  );
  await expect
    .poll(
      async () => {
        const res = await client.checkPermissions({ checks: request });
        return res.allowed.every((answer) => answer === expected);
      },
      { timeout: AUTHZ_READY_TIMEOUT_MS, intervals: [2000] },
    )
    .toBe(true);
}

// Every seeded user may view both objects, so true here means the user's
// tuples for them are in OpenFGA and no seed outbox row is still pending.
Given(
  'my access to the seeded cluster and project has synced',
  { timeout: AUTHZ_READY_TIMEOUT_MS + 10_000 },
  async function (this: ICustomWorld) {
    await pollAllowed(
      this,
      [
        { action: Action.CLUSTER_CAN_VIEW, object: 'cluster' },
        { action: Action.PROJECT_CAN_VIEW, object: 'project' },
      ],
      true,
    );
  },
);

// --- API ---

When(
  'I call {string} on the seeded {word}',
  async function (this: ICustomWorld, rpc: string, object: string) {
    const url = this.organizationApiUrl!;
    const token = this.authToken!;
    const org = this.organizationId;
    const id = seededId(object);

    const call = (): Promise<unknown> => {
      switch (rpc) {
        case 'DeleteCluster':
          return createServiceClient(ClusterService, url, token, org).deleteCluster({
            clusterId: id,
          });
        case 'CreateProject':
          return createServiceClient(ProjectService, url, token, org).createProject({
            clusterId: id,
            name: 'denial-probe',
          });
        case 'QueryLogs':
          return createServiceClient(LogsService, url, token, org).queryLogs({
            clusterId: id,
          });
        case 'CreateNamespace':
          return createServiceClient(NamespaceService, url, token, org).createNamespace({
            projectId: id,
            name: 'denial-probe',
          });
        default:
          throw new Error(`Unknown rpc: ${rpc}`);
      }
    };

    const started = Date.now();
    let error: ConnectError | undefined;
    try {
      await call();
    } catch (err) {
      error = ConnectError.from(err);
    }
    denials.set(this, { error, elapsedMs: Date.now() - started });
  },
);

Then(
  'the call should be denied in under {int} seconds',
  async function (this: ICustomWorld, seconds: number) {
    const denial = denials.get(this);
    expect(denial?.error, 'the call should have failed').toBeDefined();
    expect(denial!.error!.code).toBe(Code.PermissionDenied);
    expect(denial!.elapsedMs).toBeLessThan(seconds * 1000);
  },
);

Then(
  'the denial should name the action {string} and the role {string}',
  async function (this: ICustomWorld, action: string, role: string) {
    const error = denials.get(this)?.error;
    expect(error, 'the call should have failed').toBeDefined();
    const details = error!.findDetails(PermissionDeniedDetailSchema);
    expect(details).toHaveLength(1);
    expect(details[0].action).toBe(actionByName(action));
    expect(details[0].requiredRole).toBe(roleByName(role));
  },
);

Then(
  'asking whether I may {string} on the seeded {word} should answer {word}',
  { timeout: AUTHZ_READY_TIMEOUT_MS + 10_000 },
  async function (this: ICustomWorld, action: string, object: string, expected: string) {
    expect(['true', 'false']).toContain(expected);
    const client = createServiceClient(AuthzService, this.authzApiUrl!, this.authToken);
    const check = create(PermissionCheckSchema, {
      action: actionByName(action),
      objectId: seededId(object),
    });
    // A true answer may lag the seed reaching OpenFGA, so poll for it.
    await expect
      .poll(
        async () => {
          const res = await client.checkPermissions({ checks: [check] });
          return String(res.allowed[0]);
        },
        { timeout: AUTHZ_READY_TIMEOUT_MS, intervals: [2000] },
      )
      .toBe(expected);
  },
);

// --- Console ---

Given(
  'I am signed in to the console as {string}',
  async function (this: ICustomWorld, email: string) {
    const page = this.page!;
    const times: number[] = [];
    answerTimes.set(this, times);
    // A new document drops what the last one asked; in-app router navigations do not.
    page.on('domcontentloaded', () => {
      times.length = 0;
    });
    page.on('response', (response) => {
      if (response.url().endsWith(CHECK_PERMISSIONS_PATH)) {
        times.push(Date.now());
      }
    });
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(email, 'password');
    await loginPage.waitForLoginSuccess();
    await expect(page).toHaveURL(/\/organizations\/[^/]+\/clusters$/);
  },
);

async function openInOrganization(world: ICustomWorld, path: string) {
  const page = world.page!;
  const organization = new URL(page.url()).pathname.match(/^\/organizations\/[^/]+/);
  expect(organization, 'signed in inside an organization').not.toBeNull();
  await page.goto(`${organization![0]}${path}`);
}

async function waitForPageToRender(world: ICustomWorld) {
  // Proves the page itself rendered, not only the shell.
  const page = world.page!;
  await expect(page.locator('h1').first()).toBeVisible({ timeout: PAGE_TIMEOUT_MS });
  await expect(page.locator('[text^="Loading"]')).toHaveCount(0, {
    timeout: PAGE_TIMEOUT_MS,
  });
}

When('I open the seeded cluster in the console', async function (this: ICustomWorld) {
  await openInOrganization(this, `/clusters/${SEEDED_IDS.cluster}`);
  await waitForPageToRender(this);
});

When(
  'I open the namespaces of the seeded project in the console',
  async function (this: ICustomWorld) {
    await openInOrganization(this, `/projects/${SEEDED_IDS.project}/namespaces`);
    await waitForPageToRender(this);
  },
);

// The page shows a sentence, not a heading, when logs are denied; the step
// that follows waits for it.
When('I open the logs in the console', async function (this: ICustomWorld) {
  await openInOrganization(this, '/logs');
});

// A missing action means something only once every re-ask is done: waits for
// an answer, then for a quiet period with none. Fails if no answer comes.
When(
  'the console has had its permission answers and its re-asks',
  { timeout: 25_000 },
  async function (this: ICustomWorld) {
    const times = answerTimes.get(this)!;
    await expect
      .poll(() => (times.length > 0 ? Date.now() - times[times.length - 1] : 0), {
        timeout: 20_000,
        intervals: [250],
      })
      .toBeGreaterThanOrEqual(QUIET_AFTER_ANSWERS_MS);
  },
);

// Buttons inside a modal dialog are excluded: the confirmation dialog on the
// cluster page always renders its own "Delete cluster" button.
function actionLocator(world: ICustomWorld, text: string) {
  return world.page!.locator(
    `nldd-button[text="${text}"]:not(nldd-modal-dialog *), nldd-menu-item[text="${text}"]`,
  );
}

Then('I should not see the action {string}', async function (this: ICustomWorld, text: string) {
  await expect(actionLocator(this, text)).toHaveCount(0);
});

Then('I should see the action {string}', async function (this: ICustomWorld, text: string) {
  // Menu items sit in a closed menu, so they are attached rather than visible.
  await expect(actionLocator(this, text).first()).toBeAttached({
    timeout: PAGE_TIMEOUT_MS,
  });
});

Then('I should see the message {string}', async function (this: ICustomWorld, message: string) {
  await expect(this.page!.locator(`nldd-inline-dialog[text="${message}"]`)).toBeVisible({
    timeout: PAGE_TIMEOUT_MS,
  });
});
