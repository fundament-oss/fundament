import { When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { createHmac, randomUUID } from 'node:crypto';
import { ICustomWorld } from '../support/world.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { type ExchangeWorkloadTokenResponse } from '../support/api/token-service.ts';

// Seed and mock fixtures. Must stay in sync with
// db/testdata/001_0101-content.sql (acme-cluster, acme-corp) and authn-api's
// mock shoot verifier defaults (MOCK_SHOOT_SECRET, LOCAL_CLUSTER_ID).
const SEEDED_CLUSTER_ID = '019b4000-2000-7000-8000-000000000001';
const SEEDED_ORGANIZATION_ID = '019b4000-0000-7000-8000-000000000001';
// Matches authn-api's MOCK_SHOOT_SECRET (default in its main.go); a deployment
// that sets authnApi.mockShootSecret exports the same value to the suite.
const MOCK_SHOOT_SECRET = process.env.MOCK_SHOOT_SECRET ?? 'mock-shoot-secret';
const CREDENTIAL_AUDIENCE = 'fundament-authn-api';
const PLUGIN_CONTROLLER_SUBJECT =
  'system:serviceaccount:fundament-system:plugin-controller';

let exchangeResponse: ExchangeWorkloadTokenResponse | undefined;

interface WorkloadClaims {
  iss: string;
  sub: string;
  aud: string | string[];
  exp: number;
  iat?: number;
  jti?: string;
  organization_id: string;
  workload: string;
}

function base64url(input: Buffer | string): string {
  return Buffer.from(input).toString('base64url');
}

// mockProjectedToken mints what a shoot's kube-apiserver would project into
// the plugin-controller pod, minus the cluster: an HS256 JWT the mock
// verifier accepts. Real deployments verify by TokenReview instead.
function mockProjectedToken(opts: {
  subject?: string;
  audience?: string;
  expiresInSeconds?: number;
}): string {
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const payload = base64url(
    JSON.stringify({
      iss: 'https://kubernetes.default.svc.cluster.local',
      sub: opts.subject ?? PLUGIN_CONTROLLER_SUBJECT,
      aud: [opts.audience ?? CREDENTIAL_AUDIENCE],
      iat: now,
      exp: now + (opts.expiresInSeconds ?? 3600),
      jti: randomUUID(),
    }),
  );
  const signature = createHmac('sha256', MOCK_SHOOT_SECRET)
    .update(`${header}.${payload}`)
    .digest('base64url');
  return `${header}.${payload}.${signature}`;
}

function decodeWorkloadClaims(token: string): WorkloadClaims {
  const parts = token.split('.');
  if (parts.length !== 3) {
    throw new Error(`expected JWT with 3 parts, got ${parts.length}`);
  }
  return JSON.parse(
    Buffer.from(parts[1], 'base64url').toString(),
  ) as WorkloadClaims;
}

async function exchange(
  world: ICustomWorld,
  credential: string,
  clusterId: string,
): Promise<void> {
  try {
    exchangeResponse = await world.tokenService!.exchangeWorkloadToken(
      credential,
      clusterId,
    );
    world.lastApiResponse = exchangeResponse;
    world.lastApiError = undefined;
  } catch (error) {
    world.lastApiError = error as Error;
    world.lastApiResponse = undefined;
    exchangeResponse = undefined;
  }
}

// --- When steps ---

When(
  'I exchange a mock projected token for the seeded cluster',
  async function (this: ICustomWorld) {
    await exchange(this, mockProjectedToken({}), SEEDED_CLUSTER_ID);
  },
);

When(
  'I exchange a mock projected token that expires in {int} seconds',
  async function (this: ICustomWorld, seconds: number) {
    await exchange(
      this,
      mockProjectedToken({ expiresInSeconds: seconds }),
      SEEDED_CLUSTER_ID,
    );
  },
);

When(
  'I exchange a mock projected token with audience {string}',
  async function (this: ICustomWorld, audience: string) {
    await exchange(this, mockProjectedToken({ audience }), SEEDED_CLUSTER_ID);
  },
);

When(
  'I exchange a mock projected token for subject {string}',
  async function (this: ICustomWorld, subject: string) {
    await exchange(this, mockProjectedToken({ subject }), SEEDED_CLUSTER_ID);
  },
);

When(
  'I exchange a mock projected token for an unknown cluster',
  async function (this: ICustomWorld) {
    await exchange(
      this,
      mockProjectedToken({}),
      '00000000-0000-0000-0000-0000000000ff',
    );
  },
);

When(
  'I exchange a mock projected token for cluster id {string}',
  async function (this: ICustomWorld, clusterId: string) {
    await exchange(this, mockProjectedToken({}), clusterId);
  },
);

When(
  'I exchange my user token as a workload credential',
  async function (this: ICustomWorld) {
    await exchange(this, this.authToken!, SEEDED_CLUSTER_ID);
  },
);

When(
  'I exchange the workload token as a workload credential',
  async function (this: ICustomWorld) {
    await exchange(this, exchangeResponse!.accessToken, SEEDED_CLUSTER_ID);
  },
);

When(
  'I exchange a workload token without an Authorization header',
  async function (this: ICustomWorld) {
    // Bypass the typed client so we can omit Authorization entirely.
    try {
      const response = await fetch(
        `${this.authnApiUrl}/authn.v1.TokenService/ExchangeWorkloadToken`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Connect-Protocol-Version': '1',
          },
          body: JSON.stringify({ clusterId: SEEDED_CLUSTER_ID }),
        },
      );

      if (!response.ok) {
        const body = await response.json();
        throw new ConnectRpcError(
          body.code || 'unknown',
          body.message || 'Unknown error',
        );
      }

      this.lastApiResponse = await response.json();
      this.lastApiError = undefined;
    } catch (error) {
      this.lastApiError = error as Error;
      this.lastApiResponse = undefined;
    }
  },
);

When(
  'I use the workload token to call GetUserInfo',
  async function (this: ICustomWorld) {
    // GetUserInfo's validator is pinned to aud=fundament-user; a
    // WorkloadToken (aud=fundament-workload) must be rejected on the
    // audience pin even though it is signed with the same HMAC secret.
    try {
      const response = await fetch(
        `${this.authnApiUrl}/authn.v1.AuthnService/GetUserInfo`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Connect-Protocol-Version': '1',
            Authorization: `Bearer ${exchangeResponse!.accessToken}`,
          },
          body: JSON.stringify({}),
        },
      );

      if (!response.ok) {
        const body = await response.json();
        throw new ConnectRpcError(
          body.code || 'unknown',
          body.message || 'Unknown error',
        );
      }

      this.lastApiResponse = await response.json();
      this.lastApiError = undefined;
    } catch (error) {
      this.lastApiError = error as Error;
      this.lastApiResponse = undefined;
    }
  },
);

// --- Then steps ---

Then(
  'I should receive a valid WorkloadToken',
  async function (this: ICustomWorld) {
    expect(this.lastApiError).toBeUndefined();
    expect(exchangeResponse).toBeDefined();
    expect(exchangeResponse!.accessToken.split('.').length).toBe(3);
  },
);

Then(
  'the workload token type should be {string}',
  async function (this: ICustomWorld, want: string) {
    expect(exchangeResponse!.tokenType).toBe(want);
  },
);

Then(
  'the workload token should expire in at most {int} seconds',
  async function (this: ICustomWorld, seconds: number) {
    expect(Number(exchangeResponse!.expiresIn)).toBeLessThanOrEqual(seconds);
    expect(Number(exchangeResponse!.expiresIn)).toBeGreaterThan(seconds - 30);
    const claims = decodeWorkloadClaims(exchangeResponse!.accessToken);
    const now = Math.floor(Date.now() / 1000);
    expect(claims.exp).toBeLessThanOrEqual(now + seconds + 5);
  },
);

Then(
  'the workload token audience should be {string}',
  async function (this: ICustomWorld, want: string) {
    const claims = decodeWorkloadClaims(exchangeResponse!.accessToken);
    const aud = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
    expect(aud).toContain(want);
  },
);

Then(
  'the workload token subject should be the seeded cluster',
  async function (this: ICustomWorld) {
    const claims = decodeWorkloadClaims(exchangeResponse!.accessToken);
    expect(claims.sub).toBe(SEEDED_CLUSTER_ID);
    expect(claims.iss).toBe('fundament-authn-api');
    expect(claims.jti).toBeDefined();
  },
);

Then(
  "the workload token organization should be the seeded cluster's owner",
  async function (this: ICustomWorld) {
    const claims = decodeWorkloadClaims(exchangeResponse!.accessToken);
    expect(claims.organization_id).toBe(SEEDED_ORGANIZATION_ID);
  },
);

Then(
  'the workload token workload should be {string}',
  async function (this: ICustomWorld, want: string) {
    const claims = decodeWorkloadClaims(exchangeResponse!.accessToken);
    expect(claims.workload).toBe(want);
  },
);
