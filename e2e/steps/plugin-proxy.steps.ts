import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import {
  type InstallationFixture,
  fixtureInstallation,
  INSTALL_TIMEOUT_MS,
  realGardener,
} from '../support/fixtures.ts';

// For paths that are rejected before the cluster is looked up.
const SEEDED_CLUSTER_ID = '019b4000-2000-7000-8000-000000000001';

// Per-scenario state for plugin-proxy tests. Reset by the @api Before hook
// via fresh `lastApiResponse` / `lastApiError`, but the response itself needs
// its own slot so we can read headers + body without re-reading the stream.
interface PluginProxyState {
  response?: Response;
  body?: string;
  pluginToken?: string;
}
const scenarioState = new WeakMap<ICustomWorld, PluginProxyState>();

function state(world: ICustomWorld): PluginProxyState {
  let s = scenarioState.get(world);
  if (!s) {
    s = {};
    scenarioState.set(world, s);
  }
  return s;
}

async function captureResponse(
  world: ICustomWorld,
  response: Response,
): Promise<void> {
  const s = state(world);
  s.response = response;
  s.body = await response.text();
}

// Expand ${PLUGIN_PROXY_URL} / ${CONSOLE_URL} so feature strings work in both
// local dev (fundament.localhost) and PR envs (pr${N}.${DOMAIN}), and
// ${CLUSTER_ID} / ${INSTALLATION} / ${VERSION} / ${INSTALLATION_ID} to the e2e
// installation once a step has resolved it. Asset paths name the installation,
// as the console builds them (plugin-console-url.utils.ts).
function resolveUrls(world: ICustomWorld, text: string): string {
  const fixture = world.testData.installation as InstallationFixture | undefined;
  return text
    .replace(/\$\{PLUGIN_PROXY_URL\}/g, world.pluginProxyUrl ?? '')
    .replace(/\$\{CONSOLE_URL\}/g, world.consoleUrl ?? '')
    .replace(/\$\{CLUSTER_ID\}/g, fixture?.clusterId ?? SEEDED_CLUSTER_ID)
    .replace(/\$\{INSTALLATION\}/g, fixture?.installationName ?? '')
    .replace(/\$\{VERSION\}/g, fixture?.pluginVersion ?? '')
    .replace(/\$\{INSTALLATION_ID\}/g, fixture?.installationId ?? '');
}

// --- Given ---

Given(
  'the e2e installation is running',
  { timeout: INSTALL_TIMEOUT_MS + 60_000 },
  async function (this: ICustomWorld) {
    this.testData.installation = await fixtureInstallation(this);
  },
);

Given(
  'I have a plugin token for the e2e installation',
  { timeout: INSTALL_TIMEOUT_MS + 60_000 },
  async function (this: ICustomWorld) {
    const fixture = await fixtureInstallation(this);
    this.testData.installation = fixture;
    const mint = await this.tokenService!.mintPluginToken(
      this.authToken!,
      fixture.clusterId,
      fixture.installationId,
    );
    state(this).pluginToken = mint.accessToken;
  },
);

// --- When ---

When(
  'I GET the asset {string}',
  async function (this: ICustomWorld, path: string) {
    // The asset route is /clusters/{clusterID}/plugins/{name}/{version}/console/{path}
    // and is gated by the console UserToken cookie + OpenFGA can_view. Send the
    // authenticated user's token as the fundament_auth cookie (matches
    // common/auth/auth.go ConsoleAuthCookieName). Malformed-path negative cases
    // are rejected by parsePath before auth, so the cookie is harmless there.
    const headers: Record<string, string> = {};
    if (this.authToken) {
      headers.Cookie = `fundament_auth=${this.authToken}`;
    }
    const response = await fetch(
      `${this.pluginProxyUrl}${resolveUrls(this, path)}`,
      {
        redirect: 'manual',
        headers,
      },
    );
    await captureResponse(this, response);
  },
);

When(
  'I send a GET to the installation route {string} with no token',
  async function (this: ICustomWorld, path: string) {
    const response = await fetch(`${this.pluginProxyUrl}${path}`, {
      redirect: 'manual',
    });
    await captureResponse(this, response);
  },
);

When(
  'I send a GET to the installation route {string} with the plugin token',
  async function (this: ICustomWorld, path: string) {
    const tok = state(this).pluginToken;
    if (!tok) {
      throw new Error('plugin token was not minted — Background step missing?');
    }
    const response = await fetch(`${this.pluginProxyUrl}${resolveUrls(this, path)}`, {
      redirect: 'manual',
      headers: { Authorization: `Bearer ${tok}` },
    });
    await captureResponse(this, response);
  },
);

// --- Then ---

Then(
  'the response status should be {int}',
  function (this: ICustomWorld, want: number) {
    const r = state(this).response;
    expect(r, 'no response captured').toBeDefined();
    expect(r!.status).toBe(want);
  },
);

Then(
  'the response status should not be in the 2xx range',
  function (this: ICustomWorld) {
    const r = state(this).response;
    expect(r, 'no response captured').toBeDefined();
    expect(r!.status).toBeGreaterThanOrEqual(300);
  },
);

// The mock plugin-proxy answers every runtime call itself; a real plugin pod
// answers /livez (plugin-sdk/pluginruntime/health).
Then(
  "the body is the installation runtime's liveness reply",
  function (this: ICustomWorld) {
    expect(state(this).body).toBe(realGardener() ? 'ok' : 'mock backend');
  },
);

Then(
  'the {string} header should be {string}',
  function (this: ICustomWorld, name: string, want: string) {
    const got = state(this).response!.headers.get(name);
    expect(got, `header ${name}`).toBe(resolveUrls(this, want));
  },
);

Then(
  'the {string} header should start with {string}',
  function (this: ICustomWorld, name: string, prefix: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    const expanded = resolveUrls(this, prefix);
    expect(got.startsWith(expanded), `header ${name} = ${got}`).toBe(true);
  },
);

Then(
  'the {string} header should contain {string}',
  function (this: ICustomWorld, name: string, substr: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    expect(got).toContain(resolveUrls(this, substr));
  },
);

Then(
  'the {string} header should not contain {string}',
  function (this: ICustomWorld, name: string, substr: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    expect(got).not.toContain(resolveUrls(this, substr));
  },
);
