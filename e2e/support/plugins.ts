/**
 * Plugin installations on a real cluster, made the way the console and the
 * OpenTofu provider make them: a PluginInstallation created through the
 * kube-api-proxy with the user's kubeconfig, pinned to the latest definition
 * the catalog publishes.
 */

import { createServiceClient } from './api/client.ts';
import { PluginService } from './generated/v1/plugin_pb.ts';
import type { Kubectl, UserSession } from './kube.ts';
import { pollUntil } from './poll.ts';

export interface PluginInstallation {
  metadata: { name: string; uid: string };
  spec: { definitionRef: { organizationName: string; pluginName: string; pluginVersion: string } };
  status?: { phase?: string; message?: string };
}

export function installationName(organizationName: string, pluginName: string): string {
  return `${organizationName}--${pluginName}`;
}

// The namespace plugin-controller runs an installation in (common/kubename),
// for names of up to 56 characters, as the suite's are; longer ones get a hash.
export function pluginNamespace(name: string): string {
  return `plugin-${name}`;
}

export function getInstallation(kubectl: Kubectl, name: string): PluginInstallation | undefined {
  const out = kubectl(['get', 'plugininstallations.plugins.fundament.io', name, '-o', 'json', '--ignore-not-found']);
  return out.trim() ? (JSON.parse(out) as PluginInstallation) : undefined;
}

export async function installPlugin(
  session: UserSession,
  kubectl: Kubectl,
  organizationName: string,
  pluginName: string,
): Promise<void> {
  const plugins = createServiceClient(PluginService, session.organizationApiUrl, session.authToken, session.organizationId);
  const { plugins: catalog } = await plugins.listPlugins({});
  const plugin = catalog.find((p) => p.name === pluginName && p.organizationName === organizationName);
  if (!plugin?.pluginVersion || !plugin.definitionHash) {
    throw new Error(`the catalog publishes no installable ${organizationName}/${pluginName}; run: just test-env plugin-publish ${pluginName}`);
  }
  const manifest = {
    apiVersion: 'plugins.fundament.io/v1',
    kind: 'PluginInstallation',
    metadata: { name: installationName(organizationName, pluginName) },
    spec: {
      definitionRef: {
        organizationName,
        pluginName,
        pluginVersion: plugin.pluginVersion,
        definitionHash: plugin.definitionHash,
      },
    },
  };
  kubectl(['create', '-f', '-'], JSON.stringify(manifest));
}

export async function waitForPhase(
  kubectl: Kubectl,
  name: string,
  phase: string,
  deadlineMs: number,
): Promise<PluginInstallation> {
  return pollUntil(deadlineMs, `plugin installation ${name} to be ${phase}`, async () => {
    const installation = getInstallation(kubectl, name);
    return installation?.status?.phase === phase ? installation : undefined;
  });
}

export async function waitForGone(kubectl: Kubectl, name: string, deadlineMs: number): Promise<void> {
  await pollUntil(deadlineMs, `plugin installation ${name} and its namespace to be gone`, async () => {
    const left = getInstallation(kubectl, name) ?? kubectl(['get', 'namespace', pluginNamespace(name), '--ignore-not-found']).trim();
    return left ? undefined : true;
  });
}
