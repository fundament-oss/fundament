import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';

// Mock fixtures the plugin-proxy serves in PLUGIN_PROXY_MODE=mock. Must stay
// in sync with plugin-proxy/pkg/service/mock.go (MockClusterID,
// MockInstallationUID) and db/testdata/001_0101-content.sql (acme-cluster).
const SEEDED_CLUSTER_ID = '019b4000-2000-7000-8000-000000000001';
const MOCK_INSTALLATION_ID = '00000000-0000-0000-0000-000000000001';

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

async function captureResponse(world: ICustomWorld, response: Response): Promise<void> {
  const s = state(world);
  s.response = response;
  s.body = await response.text();
}

// --- Given ---

Given('I have a plugin token for the seeded installation', async function (this: ICustomWorld) {
  const mint = await this.tokenService!.mintPluginToken(
    this.authToken!,
    SEEDED_CLUSTER_ID,
    MOCK_INSTALLATION_ID,
  );
  state(this).pluginToken = mint.accessToken;
});

// --- When ---

When('I GET the asset {string}', async function (this: ICustomWorld, path: string) {
  const response = await fetch(`${this.pluginProxyUrl}${path}`, { redirect: 'manual' });
  await captureResponse(this, response);
});

When(
  'I send a GET to the installation route {string} with no token',
  async function (this: ICustomWorld, path: string) {
    const response = await fetch(`${this.pluginProxyUrl}${path}`, { redirect: 'manual' });
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
    const response = await fetch(`${this.pluginProxyUrl}${path}`, {
      redirect: 'manual',
      headers: { Authorization: `Bearer ${tok}` },
    });
    await captureResponse(this, response);
  },
);

When(
  'I send a CORS preflight to {string} from origin {string}',
  async function (this: ICustomWorld, path: string, origin: string) {
    const response = await fetch(`${this.pluginProxyUrl}${path}`, {
      method: 'OPTIONS',
      redirect: 'manual',
      headers: {
        Origin: origin,
        'Access-Control-Request-Method': 'POST',
        'Access-Control-Request-Headers': 'Authorization,Content-Type',
      },
    });
    await captureResponse(this, response);
  },
);

// --- Then ---

Then('the response status should be {int}', function (this: ICustomWorld, want: number) {
  const r = state(this).response;
  expect(r, 'no response captured').toBeDefined();
  expect(r!.status).toBe(want);
});

Then('the response status should not be in the 2xx range', function (this: ICustomWorld) {
  const r = state(this).response;
  expect(r, 'no response captured').toBeDefined();
  expect(r!.status).toBeGreaterThanOrEqual(300);
});

Then('the response body should equal {string}', function (this: ICustomWorld, want: string) {
  expect(state(this).body).toBe(want);
});

Then(
  'the {string} header should be {string}',
  function (this: ICustomWorld, name: string, want: string) {
    const got = state(this).response!.headers.get(name);
    expect(got, `header ${name}`).toBe(want);
  },
);

Then(
  'the {string} header should start with {string}',
  function (this: ICustomWorld, name: string, prefix: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    expect(got.startsWith(prefix), `header ${name} = ${got}`).toBe(true);
  },
);

Then(
  'the {string} header should contain {string}',
  function (this: ICustomWorld, name: string, substr: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    expect(got).toContain(substr);
  },
);

Then(
  'the {string} header should not contain {string}',
  function (this: ICustomWorld, name: string, substr: string) {
    const got = state(this).response!.headers.get(name) ?? '';
    expect(got).not.toContain(substr);
  },
);

Then('the {string} header should be absent', function (this: ICustomWorld, name: string) {
  expect(state(this).response!.headers.get(name)).toBeNull();
});
