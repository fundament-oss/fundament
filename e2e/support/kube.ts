/**
 * kubectl against a cluster as a fundament user, the way a user reaches it: the
 * cluster's kubeconfig from the API, which authenticates through
 * `functl cluster token` and the kube-api-proxy.
 */

import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { APIKeyService } from './api/apikey-service.ts';
import { ClusterService } from './api/cluster-service.ts';

const functl = process.env.E2E_FUNCTL ?? 'functl';

export interface UserSession {
  organizationApiUrl: string;
  authToken: string;
  organizationId?: string;
}

// Runs kubectl, with input on stdin when given, and returns its stdout; a
// failure throws with kubectl's stderr.
export type Kubectl = (args: string[], input?: string) => string;

export async function withClusterAccess<T>(
  session: UserSession,
  clusterId: string,
  use: (kubectl: Kubectl) => Promise<T>,
): Promise<T> {
  const { organizationApiUrl, authToken, organizationId } = session;
  const clusters = new ClusterService(organizationApiUrl, authToken, organizationId);
  const apiKeys = new APIKeyService(organizationApiUrl, authToken, organizationId);
  const dir = mkdtempSync(join(tmpdir(), 'e2e-kube-'));
  try {
    const kubeconfig = join(dir, 'kubeconfig');
    writeFileSync(kubeconfig, await clusters.getKubeconfig(clusterId));

    // functl needs a login: an API key of the same user, in a config dir of its own.
    const key = await apiKeys.createAPIKey({ name: `e2e-kube-${Date.now().toString(36)}`, expiresIn: '1h' });
    // Pointed at the same installation as the session, so the key never goes
    // to functl's built-in default endpoint.
    const env = {
      ...process.env,
      FUNCTL_CONFIG_DIR: join(dir, 'functl'),
      FUNCTL_API_ENDPOINT: organizationApiUrl,
      FUNCTL_AUTHN_URL: process.env.AUTHN_API_URL ?? 'https://authn.fundament.localhost:8443',
      PATH: `${dirname(functl)}:${process.env.PATH}`,
    };
    try {
      // On stdin, so the key does not show in the process list.
      execFileSync(functl, ['auth', 'login'], { env, input: `${key.token}\n`, stdio: 'pipe' });
      return await use((args, input) => {
        try {
          return execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { env, input, encoding: 'utf8', stdio: 'pipe' });
        } catch (err) {
          const stderr = (err as { stderr?: Buffer | string }).stderr?.toString().trim();
          throw new Error(`kubectl ${args.join(' ')}: ${stderr || err}`);
        }
      });
    } finally {
      // A failed delete must not hide the error use() threw.
      await apiKeys.deleteAPIKey(key.id).catch((err) => console.error(`cleanup: failed to delete API key ${key.id}`, err));
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
