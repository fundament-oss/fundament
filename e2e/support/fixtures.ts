/**
 * The cluster and plugin installation that scenarios address, in either kind of
 * environment:
 *   - mock Gardener (PR previews, local dev): the seeded acme-cluster and the
 *     one installation the mock plugin-proxy serves for it;
 *   - real Gardener (the test-env VM sets E2E_GARDENER=real): the baseline
 *     cluster `just test-env up` creates, with cert-manager installed on it.
 */

import { ClusterService } from './api/cluster-service.ts';
import { type UserSession, withClusterAccess } from './kube.ts';
import { getInstallation, installPlugin, installationName, waitForPhase } from './plugins.ts';
import type { ICustomWorld } from './world.ts';

// Read when called: hooks.ts loads .env.local after this module is imported.
export function realGardener(): boolean {
  return process.env.E2E_GARDENER === 'real';
}

export const clusterName = process.env.E2E_CLUSTER_NAME ?? 'e2e-base';

// The plugin the real-mode installation fixture installs, published by the
// system organization during `just test-env up`. Scenarios load pages of its
// console bundle.
export const fixturePlugin = { organizationName: 'system', pluginName: 'cert-manager' };

// Seed and mock fixtures: db/testdata/001_0101-content.sql (acme-cluster) and
// plugin-proxy/pkg/service/mock.go (MockClusterID, MockInstallationID).
const SEEDED_CLUSTER_ID = '019b4000-2000-7000-8000-000000000001';
const MOCK_INSTALLATION = {
  installationId: '00000000-0000-0000-0000-000000000001',
  pluginName: 'test-plugin',
  installationName: 'system--test-plugin',
  pluginVersion: 'v1.17.2',
};

// A first install pulls the plugin's images, so steps that may make one need
// this much time on top of their own.
export const INSTALL_TIMEOUT_MS = 10 * 60_000;

export interface InstallationFixture {
  clusterId: string;
  installationId: string;
  pluginName: string;
  installationName: string;
  pluginVersion: string;
}

export function session(world: ICustomWorld): UserSession {
  if (!world.authToken) {
    throw new Error('not authenticated: add "Given I am authenticated as ..." first');
  }
  return { organizationApiUrl: world.organizationApiUrl!, authToken: world.authToken, organizationId: world.organizationId };
}

export async function fixtureClusterId(world: ICustomWorld): Promise<string> {
  if (!realGardener()) {
    return SEEDED_CLUSTER_ID;
  }
  const s = session(world);
  const cluster = await new ClusterService(s.organizationApiUrl, s.authToken, s.organizationId).getClusterByName(clusterName);
  if (!cluster) {
    throw new Error(`no cluster ${clusterName}; just test-env up creates it`);
  }
  return cluster.id;
}

// The fixture installation, installed first when it is missing (an earlier
// scenario may have uninstalled it).
export async function fixtureInstallation(world: ICustomWorld): Promise<InstallationFixture> {
  if (!realGardener()) {
    return { clusterId: SEEDED_CLUSTER_ID, ...MOCK_INSTALLATION };
  }
  const clusterId = await fixtureClusterId(world);
  const { organizationName, pluginName } = fixturePlugin;
  const name = installationName(organizationName, pluginName);
  const s = session(world);
  const installation = await withClusterAccess(s, clusterId, async (kubectl) => {
    if (!getInstallation(kubectl, name)) {
      await installPlugin(s, kubectl, organizationName, pluginName);
    }
    return waitForPhase(kubectl, name, 'Running', INSTALL_TIMEOUT_MS);
  });
  return {
    clusterId,
    installationId: installation.metadata.uid,
    pluginName,
    installationName: name,
    pluginVersion: installation.spec.definitionRef.pluginVersion,
  };
}
