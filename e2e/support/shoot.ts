/**
 * The Gardener side of a cluster, for scenarios that check what fundament did
 * there or act as a workload on the shoot. Reaches the garden through
 * E2E_GARDEN_CONTEXT; only the test-env VM has one.
 */

import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Kubectl } from './kube.ts';

const gardenContext = process.env.E2E_GARDEN_CONTEXT ?? 'kind-gardener-local-garden';

export interface Shoot {
  metadata: { name: string; namespace: string };
  status?: { lastOperation?: { type: string; state: string } };
}

function garden(args: string[], input?: string): string {
  return execFileSync('kubectl', ['--context', gardenContext, ...args], {
    encoding: 'utf8',
    input,
    stdio: ['pipe', 'pipe', 'pipe'],
  });
}

// The shoot cluster-worker made for a cluster, found by its cluster-id label.
export function shootForCluster(clusterId: string): Shoot | undefined {
  const out = garden(['get', 'shoots', '-A', '-l', `fundament.io/cluster-id=${clusterId}`, '-o', 'json']);
  return (JSON.parse(out) as { items: Shoot[] }).items[0];
}

// Runs use with kubectl as cluster-admin of the cluster's shoot, through a
// short-lived admin kubeconfig from the garden.
export async function withShootAdmin<T>(clusterId: string, use: (kubectl: Kubectl) => Promise<T>): Promise<T> {
  const shoot = shootForCluster(clusterId);
  if (!shoot) {
    throw new Error(`no shoot carries the cluster-id label ${clusterId}`);
  }
  const { namespace, name } = shoot.metadata;
  const request = JSON.stringify({
    apiVersion: 'authentication.gardener.cloud/v1alpha1',
    kind: 'AdminKubeconfigRequest',
    spec: { expirationSeconds: 600 },
  });
  const response = garden(
    ['create', '--raw', `/apis/core.gardener.cloud/v1beta1/namespaces/${namespace}/shoots/${name}/adminkubeconfig`, '-f', '-'],
    request,
  );
  const dir = mkdtempSync(join(tmpdir(), 'e2e-shoot-'));
  try {
    const kubeconfig = join(dir, 'kubeconfig');
    const encoded = (JSON.parse(response) as { status: { kubeconfig: string } }).status.kubeconfig;
    writeFileSync(kubeconfig, Buffer.from(encoded, 'base64'), { mode: 0o600 });
    return await use((args, input) => {
      try {
        return execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { input, encoding: 'utf8', stdio: 'pipe' });
      } catch (err) {
        const stderr = (err as { stderr?: Buffer | string }).stderr?.toString().trim();
        throw new Error(`kubectl ${args.join(' ')}: ${stderr || err}`);
      }
    });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
