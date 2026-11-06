/**
 * Password login and the OpenFGA propagation wait, shared by the step
 * definitions and the scripts in e2e/scripts/.
 */

import { ConnectRpcError } from './api/client.ts';

/**
 * Decode a JWT and extract the first organization ID from the claims.
 */
export function extractOrganizationId(token: string): string {
  const payload = JSON.parse(
    Buffer.from(token.split('.')[1], 'base64url').toString(),
  );
  const orgIds: string[] = payload.organization_ids ?? [];
  if (orgIds.length === 0) {
    throw new Error('JWT contains no organization_ids');
  }
  return orgIds[0];
}

// Authz propagation tolerance: after a fresh deploy (DB reset + test-data
// seed), a user can authenticate before the authz-worker has written their
// org-membership tuples to OpenFGA, so permission checks transiently fail with
// permission_denied. Poll until they propagate.
export const AUTHZ_READY_TIMEOUT_MS = 60_000;
const AUTHZ_READY_INTERVAL_MS = 2_000;

/**
 * Wait until the authenticated user's org-membership tuples have propagated to
 * OpenFGA, retrying the given read-only permission-gated probe. Mirrors the
 * retry the terraform acceptance tests do around CreateAPIKey. Shared: the
 * node-pool steps probe with getClusterByName the same way.
 */
export async function waitForAuthzReady<T>(probe: () => Promise<T>): Promise<T> {
  const deadline = Date.now() + AUTHZ_READY_TIMEOUT_MS;
  for (;;) {
    try {
      return await probe();
    } catch (err) {
      if (
        err instanceof ConnectRpcError &&
        err.code === 'permission_denied' &&
        Date.now() < deadline
      ) {
        await new Promise((resolve) =>
          setTimeout(resolve, AUTHZ_READY_INTERVAL_MS),
        );
        continue;
      }
      throw err;
    }
  }
}

/**
 * Authenticate via password login and get JWT.
 */
export async function authenticateWithPassword(email: string): Promise<string> {
  const authnApiUrl =
    process.env.AUTHN_API_URL || 'https://authn.fundament.localhost:8443';
  const password = 'password';

  const response = await fetch(`${authnApiUrl}/login/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`Authentication failed: ${error}`);
  }

  const data = (await response.json()) as { access_token: string };
  if (!data.access_token) {
    throw new Error('No access_token in login response');
  }

  return data.access_token;
}
