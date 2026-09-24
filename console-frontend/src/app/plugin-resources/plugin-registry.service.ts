import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type {
  PluginDefinition,
  ParsedCrd,
  RawCrdYaml,
  PluginInstallationItem,
  PluginInstallationListResponse,
} from './types';
import type { PluginDefinition as ProtoPluginDefinition } from '../../generated/v1/plugin_pb';
import { PLUGIN } from '../../connect/tokens';
import { parseObjectSchema } from './crd-schema.utils';
import { ConfigService } from '../config.service';
import { installPhase, isInstallInProgress } from '../utils/plugin-install-status';

function parseCrd(raw: RawCrdYaml): ParsedCrd {
  const version = raw.spec.versions.find((v) => v.storage) ?? raw.spec.versions[0];

  const specRaw = version.schema.openAPIV3Schema.properties?.['spec'] as
    Record<string, unknown> | undefined;

  const statusRaw = version.schema.openAPIV3Schema.properties?.['status'] as
    Record<string, unknown> | undefined;

  const specSchema = specRaw?.['properties']
    ? parseObjectSchema(
        specRaw['properties'] as Record<string, unknown>,
        specRaw['required'] as string[] | undefined,
      )
    : { properties: {} };

  const statusSchema = statusRaw?.['properties']
    ? parseObjectSchema(
        statusRaw['properties'] as Record<string, unknown>,
        statusRaw['required'] as string[] | undefined,
      )
    : undefined;

  return {
    group: raw.spec.group,
    kind: raw.spec.names.kind,
    plural: raw.spec.names.plural,
    singular: raw.spec.names.singular,
    scope: raw.spec.scope as 'Namespaced' | 'Cluster',
    version: version.name,
    additionalPrinterColumns: version.additionalPrinterColumns ?? [],
    specSchema,
    statusSchema,
  };
}

function mapDefinition(
  def: ProtoPluginDefinition,
  installation: {
    installationId: string;
    installationName: string;
    installationVersion: string;
    organizationName: string;
  },
): PluginDefinition {
  return {
    name: def.metadata?.name ?? '',
    label: def.metadata?.displayName ?? '',
    version: def.metadata?.version ?? '',
    description: def.metadata?.description ?? '',
    author: def.metadata?.author || undefined,
    menu: {
      project: def.menu?.project?.map((e) => ({
        crd: e.crd,
        label: e.label || undefined,
        icon: e.icon || undefined,
      })),
    },
    crds: def.crds ?? [],
    customComponents:
      Object.keys(def.customComponents).length > 0
        ? Object.fromEntries(
            Object.entries(def.customComponents).map(([k, v]) => [
              k,
              {
                list: v.list || undefined,
                detail: v.detail || undefined,
                create: v.create || undefined,
              },
            ]),
          )
        : undefined,
    allowedResources: (def.allowedResources ?? []).map((r) => ({
      group: r.group,
      version: r.version,
      resource: r.resource,
      verbs: r.verbs,
    })),
    installationId: installation.installationId,
    installationName: installation.installationName,
    installationVersion: installation.installationVersion,
    organizationName: installation.organizationName,
  };
}

interface RunningInstallation {
  def: ProtoPluginDefinition | undefined;
  installationId: string;
  installationName: string;
  installationVersion: string;
  organizationName: string;
}

/** Identifies one running installation at one version: a reinstall or an
 *  upgrade changes it, a status poll that finds nothing new does not. */
function installationKey(item: PluginInstallationItem): string {
  return `${item.metadata.uid}/${item.spec.definitionRef.pluginVersion}`;
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  private loadedForClusterId: string | null = null;

  // Bumped by every load and reset, so a read that a cluster switch overtook
  // can tell its result is stale and drop it.
  private generation = 0;

  // The running installations the menu was last built from, by installationKey;
  // null when it has not been built for the current cluster yet.
  private runningKeys: string[] | null = null;

  private pollTimer: ReturnType<typeof setTimeout> | null = null;

  /** How often to re-read while an installation on the cluster is still coming
   *  up or going away, or after a read that failed. */
  private static readonly POLL_MS = 5000;

  /** How often to re-read while nothing is changing, so an install started
   *  after the menu was built, or a Degraded plugin that recovers, still shows
   *  up without a reload. */
  private static readonly IDLE_POLL_MS = 30000;

  // Parsed CRDs indexed by plural; key: "${pluginName}/${clusterId}/${plural}"
  private parsedCrdByPlural = new Map<string, ParsedCrd>();

  private configService = inject(ConfigService);

  private pluginClient = inject(PLUGIN);

  async loadPlugins(clusterId: string): Promise<void> {
    if (clusterId === this.loadedForClusterId) return;

    this.stopPolling();
    this.generation += 1;
    this.runningKeys = null;
    await this.run(clusterId, this.generation);
  }

  /**
   * Syncs the menu and schedules the next sync. The menu is built when a
   * project is opened, which is often while a plugin just installed is still
   * deploying, so while any installation is on its way up or down, or a read
   * failed, this re-reads quickly: the plugin's screens then appear in the
   * sidebar the moment it runs, without a reload. Otherwise it keeps reading
   * at a slower pace to notice installs started later.
   */
  private async run(clusterId: string, generation: number): Promise<void> {
    const result = await this.sync(clusterId, generation);
    if (result === 'stale' || generation !== this.generation) return;

    if (result !== 'failed') this.loadedForClusterId = clusterId;

    const delay =
      result === 'idle' ? PluginRegistryService.IDLE_POLL_MS : PluginRegistryService.POLL_MS;
    this.pollTimer = setTimeout(() => {
      this.pollTimer = null;
      this.run(clusterId, generation).catch(() => {});
    }, delay);
  }

  /** Reads the cluster's installations and rebuilds the menu from the running
   *  ones. */
  private async sync(
    clusterId: string,
    generation: number,
  ): Promise<'idle' | 'inProgress' | 'failed' | 'stale'> {
    const { kubeApiProxyUrl } = this.configService.getConfig();

    let listData: PluginInstallationListResponse;

    try {
      const listRes = await fetch(
        `${kubeApiProxyUrl}/clusters/${clusterId}/apis/plugins.fundament.io/v1/plugininstallations`,
        { credentials: 'include' },
      );
      if (!listRes.ok) return generation === this.generation ? 'failed' : 'stale';

      listData = (await listRes.json()) as PluginInstallationListResponse;
    } catch {
      return generation === this.generation ? 'failed' : 'stale';
    }
    if (generation !== this.generation) return 'stale';

    const items = listData.items ?? [];
    const runningPlugins = items.filter(
      (item) => item.status?.phase === 'Running' && item.status?.ready,
    );
    const keys = runningPlugins.map(installationKey);

    // Nothing started or stopped since the last read: keep the menu as it is
    // rather than fetching every definition again and redrawing the sidebar.
    const previous = this.runningKeys;
    const unchanged =
      previous !== null &&
      keys.length === previous.length &&
      keys.every((k, i) => k === previous[i]);

    if (!unchanged) {
      const { definitions, complete } = await this.fetchDefinitions(runningPlugins);
      if (generation !== this.generation) return 'stale';
      this.plugins.set(definitions);
      // A definition that failed to load must be fetched again on the next
      // read, so only remember the key set once every fetch went through.
      this.runningKeys = complete ? keys : null;
      if (!complete) return 'failed';
    }

    // An installation that has no phase yet is one the controller has not
    // picked up, which is as much on its way as a Pending one.
    return items.some((item) => isInstallInProgress(installPhase(item.status?.phase)))
      ? 'inProgress'
      : 'idle';
  }

  private async fetchDefinitions(
    items: PluginInstallationItem[],
  ): Promise<{ definitions: PluginDefinition[]; complete: boolean }> {
    const results = await Promise.allSettled(
      items.map(async (item): Promise<RunningInstallation> => {
        const ref = item.spec.definitionRef;
        const res = await firstValueFrom(
          // A definition is keyed on (organization, plugin, version): the
          // publisher is required, not optional context.
          this.pluginClient.getPluginDefinition({
            organizationName: ref.organizationName,
            pluginName: ref.pluginName,
            pluginVersion: ref.pluginVersion,
          }),
        );
        return {
          def: res.definition,
          installationId: item.metadata.uid,
          // installationName is the CR metadata.name; the plugin's namespace is
          // derived from installationName, so this — not the definition's
          // display name — drives the iframe asset URL.
          installationName: item.metadata.name,
          installationVersion: ref.pluginVersion,
          organizationName: ref.organizationName,
        };
      }),
    );

    const definitions = results
      .filter((r): r is PromiseFulfilledResult<RunningInstallation> => r.status === 'fulfilled')
      .filter((r) => r.value.def !== undefined)
      .map((r) =>
        mapDefinition(r.value.def as ProtoPluginDefinition, {
          installationId: r.value.installationId,
          installationName: r.value.installationName,
          installationVersion: r.value.installationVersion,
          organizationName: r.value.organizationName,
        }),
      );
    return { definitions, complete: results.every((r) => r.status === 'fulfilled') };
  }

  private stopPolling(): void {
    if (this.pollTimer) {
      clearTimeout(this.pollTimer);
      this.pollTimer = null;
    }
  }

  async loadCrdsForPlugin(
    pluginName: string,
    clusterId: string,
    kubeApiProxyUrl: string,
  ): Promise<void> {
    const plugin = this.getPlugin(pluginName);
    if (!plugin) return;

    const base = kubeApiProxyUrl.replace(/\/$/, '');

    await Promise.allSettled(
      plugin.crds.map(async (crdName) => {
        const url = `${base}/clusters/${clusterId}/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${crdName}`;
        const response = await fetch(url, {
          credentials: 'include',
        });

        if (!response.ok) {
          // eslint-disable-next-line no-console
          console.error(`[PluginRegistry] Failed to fetch CRD ${crdName}: ${response.status}`);
          return;
        }

        const raw = (await response.json()) as RawCrdYaml;
        const parsed = parseCrd(raw);
        const fullName = `${parsed.plural}.${parsed.group}`;
        this.parsedCrdByPlural.set(`${pluginName}/${clusterId}/${parsed.plural}`, parsed);
        this.parsedCrdByPlural.set(`${pluginName}/${clusterId}/${parsed.kind}`, parsed);
        this.parsedCrdByPlural.set(`${pluginName}/${clusterId}/${fullName}`, parsed);
      }),
    );
  }

  reset(): void {
    this.stopPolling();
    this.generation += 1;
    this.loadedForClusterId = null;
    this.runningKeys = null;
    this.plugins.set([]);
    this.parsedCrdByPlural.clear();
  }

  // Keyed on the installation name, not the definition's `name`: two
  // organizations may publish the same plugin name, and only the installation
  // name tells their installations apart.
  getPlugin(installationName: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.installationName === installationName);
  }

  getCrd(pluginName: string, plural: string, clusterId: string): ParsedCrd | undefined {
    return this.parsedCrdByPlural.get(`${pluginName}/${clusterId}/${plural}`);
  }

  allPlugins = this.plugins.asReadonly();
}
