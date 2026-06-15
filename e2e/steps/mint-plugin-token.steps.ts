import { When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { type MintPluginTokenResponse } from '../support/api/token-service.ts';

// Mock fixtures the plugin-proxy serves in PLUGIN_PROXY_MODE=mock. Must stay
// in sync with plugin-proxy/pkg/service/mock.go (MockClusterID,
// MockInstallationUID) and db/testdata/001_0101-content.sql (acme-cluster).
const SEEDED_CLUSTER_ID = '019b4000-2000-7000-8000-000000000001';
const MOCK_INSTALLATION_ID = '00000000-0000-0000-0000-000000000001';

// Track state for mint plugin token tests (matches token-exchange.steps.ts pattern).
let mintResponse: MintPluginTokenResponse | undefined;

interface PluginClaims {
  iss: string;
  sub: string;
  aud: string | string[];
  exp: number;
  iat?: number;
  cluster_id: string;
  installation_id: string;
  plugin_name: string;
  plugin_version: string;
  definition_hash: string;
}

function decodePluginClaims(token: string): PluginClaims {
  const parts = token.split('.');
  if (parts.length !== 3) {
    throw new Error(`expected JWT with 3 parts, got ${parts.length}`);
  }
  return JSON.parse(Buffer.from(parts[1], 'base64url').toString()) as PluginClaims;
}

function extractUserId(token: string): string {
  const parts = token.split('.');
  const payload = JSON.parse(Buffer.from(parts[1], 'base64url').toString());
  return payload.sub as string;
}

// --- When steps ---

When('I mint a plugin token for the seeded cluster and installation', async function (this: ICustomWorld) {
  try {
    mintResponse = await this.tokenService!.mintPluginToken(
      this.authToken!,
      SEEDED_CLUSTER_ID,
      MOCK_INSTALLATION_ID,
    );
    this.lastApiResponse = mintResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    mintResponse = undefined;
  }
});

When('I mint a plugin token without an Authorization header', async function (this: ICustomWorld) {
  // Bypass the typed client so we can omit Authorization entirely.
  try {
    const response = await fetch(`${this.authnApiUrl}/authn.v1.TokenService/MintPluginToken`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Connect-Protocol-Version': '1',
      },
      body: JSON.stringify({
        clusterId: SEEDED_CLUSTER_ID,
        installationId: MOCK_INSTALLATION_ID,
      }),
    });

    if (!response.ok) {
      const body = await response.json();
      throw new ConnectRpcError(body.code || 'unknown', body.message || 'Unknown error');
    }

    this.lastApiResponse = await response.json();
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I mint a plugin token with cluster id {string}', async function (this: ICustomWorld, clusterId: string) {
  try {
    mintResponse = await this.tokenService!.mintPluginToken(
      this.authToken!,
      clusterId,
      MOCK_INSTALLATION_ID,
    );
    this.lastApiResponse = mintResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    mintResponse = undefined;
  }
});

When('I mint a plugin token with installation id {string}', async function (this: ICustomWorld, installationId: string) {
  try {
    mintResponse = await this.tokenService!.mintPluginToken(
      this.authToken!,
      SEEDED_CLUSTER_ID,
      installationId,
    );
    this.lastApiResponse = mintResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    mintResponse = undefined;
  }
});

When('I mint a plugin token for an unknown cluster', async function (this: ICustomWorld) {
  // A valid-shape UUID that has no can_view tuple in OpenFGA; the handler
  // collapses unauthorized + missing into NotFound.
  const unknownClusterId = '00000000-0000-0000-0000-0000000000ff';
  try {
    mintResponse = await this.tokenService!.mintPluginToken(
      this.authToken!,
      unknownClusterId,
      MOCK_INSTALLATION_ID,
    );
    this.lastApiResponse = mintResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    mintResponse = undefined;
  }
});

When('I use the minted plugin token to call GetUserInfo', async function (this: ICustomWorld) {
  // GetUserInfo's validator is pinned to aud=fundament-user; a plugin token
  // (aud=fundament-plugin) must be rejected on the audience pin even though
  // it is signed with the same HMAC secret.
  try {
    const response = await fetch(`${this.authnApiUrl}/authn.v1.AuthnService/GetUserInfo`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Connect-Protocol-Version': '1',
        Authorization: `Bearer ${mintResponse!.accessToken}`,
      },
      body: JSON.stringify({}),
    });

    if (!response.ok) {
      const body = await response.json();
      throw new ConnectRpcError(body.code || 'unknown', body.message || 'Unknown error');
    }

    this.lastApiResponse = await response.json();
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

// --- Then steps ---

Then('I should receive a valid JWT for plugin use', async function (this: ICustomWorld) {
  expect(mintResponse).toBeDefined();
  expect(mintResponse!.accessToken).toBeDefined();
  expect(mintResponse!.accessToken.length).toBeGreaterThan(0);
  expect(mintResponse!.accessToken.split('.').length).toBe(3);
});

Then('the plugin token audience should be {string}', async function (this: ICustomWorld, want: string) {
  const claims = decodePluginClaims(mintResponse!.accessToken);
  const aud = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  expect(aud).toContain(want);
});

Then('the plugin token subject should be the authenticated user', async function (this: ICustomWorld) {
  const claims = decodePluginClaims(mintResponse!.accessToken);
  const expectedSub = extractUserId(this.authToken!);
  expect(claims.sub).toBe(expectedSub);
});

Then('the plugin token should bind the cluster and installation', async function (this: ICustomWorld) {
  const claims = decodePluginClaims(mintResponse!.accessToken);
  expect(claims.cluster_id).toBe(SEEDED_CLUSTER_ID);
  expect(claims.installation_id).toBe(MOCK_INSTALLATION_ID);
});

Then('the plugin token should carry the plugin name {string}', async function (this: ICustomWorld, want: string) {
  const claims = decodePluginClaims(mintResponse!.accessToken);
  expect(claims.plugin_name).toBe(want);
});

Then('I should receive an invalid argument error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
  expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
  expect((this.lastApiError as ConnectRpcError).code).toBe('invalid_argument');
});
