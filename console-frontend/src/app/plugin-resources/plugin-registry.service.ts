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
        singularLabel: e.singularLabel || undefined,
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

  // The cluster the poll loop runs for, and its latest sync; a second
  // loadPlugins for the same cluster waits on that instead of starting over.
  private activeClusterId: string | null = null;

  private latestLoad: Promise<void> | null = null;

  // Whether the latest sync left something unread: the list, or a running
  // plugin's definition. A loadPlugins then retries at once rather than hand
  // back that result: it comes from a user opening a screen that needs it.
  private lastSyncFailed = false;

  // List reads that failed in a row, to back off from a failure that is not
  // clearing (no RBAC to list installations, no plugins CRD on the cluster).
  private failures = 0;

  // Definition fetches that failed in a row, and when to try them again. Kept
  // apart from the list reads, which go on at the fast pace while an install
  // is on its way: a running plugin whose definition is gone from the catalog
  // must not be refetched on every one of those.
  private definitionFailures = 0;

  private definitionRetryAt: number | null = null;

  // Bumped by every load and reset, so a read that a cluster switch overtook
  // can tell its result is stale and drop it.
  private generation = 0;

  // The running installations the menu was last built from, by installationKey;
  // null when it has not been built for the current cluster yet.
  private runningKeys: string[] | null = null;

  private pollTimer: ReturnType<typeof setTimeout> | null = null;

  /** How often to re-read while an installation on the cluster is still coming
   *  up or going away, and the first retry after a read that failed. */
  private static readonly POLL_MS = 5000;

  /** The longest wait between retries of a read that keeps failing. */
  private static readonly MAX_BACKOFF_MS = 120000;

  /** How often to re-read while nothing is changing, so an install started
   *  after the menu was built, or a Degraded plugin that recovers, still shows
   *  up without a reload. */
  private static readonly IDLE_POLL_MS = 30000;

  // Parsed CRDs indexed by plural; key: "${pluginName}/${clusterId}/${plural}"
  private parsedCrdByPlural = new Map<string, ParsedCrd>();

  private configService = inject(ConfigService);

  private pluginClient = inject(PLUGIN);

  loadPlugins(clusterId: string): Promise<void> {
    if (clusterId === this.activeClusterId && this.latestLoad) {
      // A retry already waiting on the backoff timer runs now instead; one
      // that is under way is what the caller waits on.
      if (this.lastSyncFailed && this.pollTimer) {
        this.stopPolling();
        if (this.definitionRetryAt !== null) this.definitionRetryAt = 0;
        this.startRun(clusterId, this.generation);
      }
      return this.latestLoad;
    }

    this.stopPolling();
    this.generation += 1;
    this.runningKeys = null;
    this.failures = 0;
    this.definitionFailures = 0;
    this.definitionRetryAt = null;
    this.lastSyncFailed = false;
    this.activeClusterId = clusterId;
    return this.startRun(clusterId, this.generation);
  }

  private startRun(clusterId: string, generation: number): Promise<void> {
    const load = this.run(clusterId, generation).catch((err: unknown) => {
      // A load that threw scheduled no next read, so let the next call for
      // this cluster start over rather than hand back the same failure.
      if (generation === this.generation && this.latestLoad === load) this.latestLoad = null;
      throw err;
    });
    this.latestLoad = load;
    return load;
  }

  /**
   * Syncs the menu and schedules the next sync. The menu is built when a
   * project is opened, which is often while a plugin just installed is still
   * deploying, so while any installation is on its way up or down, or a read
   * failed, this re-reads quickly: the plugin's screens then appear in the
   * sidebar the moment it runs, without a reload. Otherwise it keeps reading
   * at a slower pace to notice installs started later. A read that keeps
   * failing is retried with exponential backoff. So is a definition that keeps
   * failing to load, on a clock of its own and never slower than the idle
   * pace: the list itself still works, and an install started meanwhile
   * should still show up.
   */
  private async run(clusterId: string, generation: number): Promise<void> {
    const result = await this.sync(clusterId, generation);
    if (result === 'stale' || generation !== this.generation) return;

    this.failures = result === 'failed' ? this.failures + 1 : 0;
    this.lastSyncFailed = result === 'failed' || this.definitionRetryAt !== null;

    let delay: number;
    switch (result) {
      case 'idle':
        delay = PluginRegistryService.IDLE_POLL_MS;
        break;
      case 'inProgress':
        delay = PluginRegistryService.POLL_MS;
        break;
      case 'incomplete':
        delay = Math.max(0, (this.definitionRetryAt ?? 0) - Date.now());
        break;
      case 'failed':
        delay = Math.min(
          PluginRegistryService.POLL_MS * 2 ** (this.failures - 1),
          PluginRegistryService.MAX_BACKOFF_MS,
        );
        break;
      default:
        throw new Error(`unexpected sync result: ${result satisfies never}`);
    }
    this.pollTimer = setTimeout(() => {
      this.pollTimer = null;
      this.startRun(clusterId, generation).catch(() => {});
    }, delay);
  }

  /** Reads the cluster's installations and rebuilds the menu from the running
   *  ones. 'failed' is a list that could not be read, 'incomplete' a list that
   *  was read with a running plugin whose definition could not be, and whose
   *  retry is what to wait for next. */
  private async sync(
    clusterId: string,
    generation: number,
  ): Promise<'idle' | 'inProgress' | 'incomplete' | 'failed' | 'stale'> {
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

    // Nothing started or stopped since the last read, and no failed
    // definition is due another try: keep the menu as it is rather than
    // fetching every definition again and redrawing the sidebar.
    const previous = this.runningKeys;
    const unchanged =
      previous !== null &&
      keys.length === previous.length &&
      keys.every((k, i) => k === previous[i]);
    const retryDue = this.definitionRetryAt !== null && Date.now() >= this.definitionRetryAt;

    if (!unchanged || retryDue) {
      const fetched = await this.fetchDefinitions(runningPlugins);
      if (generation !== this.generation) return 'stale';
      this.plugins.set(fetched.definitions);
      this.runningKeys = keys;
      if (fetched.complete) {
        this.definitionFailures = 0;
        this.definitionRetryAt = null;
      } else {
        this.definitionFailures += 1;
        this.definitionRetryAt =
          Date.now() +
          Math.min(
            PluginRegistryService.POLL_MS * 2 ** (this.definitionFailures - 1),
            PluginRegistryService.IDLE_POLL_MS,
          );
      }
    }

    // An installation that has no phase yet is one the controller has not
    // picked up, which is as much on its way as a Pending one.
    if (items.some((item) => isInstallInProgress(installPhase(item.status?.phase)))) {
      return 'inProgress';
    }
    return this.definitionRetryAt === null ? 'idle' : 'incomplete';
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

    // A definition that fails to load keeps the menu entry it had, so one
    // flaky fetch does not drop a working plugin from the sidebar.
    const previous = this.plugins();
    const definitions = results.flatMap((r, i): PluginDefinition[] => {
      if (r.status === 'rejected') {
        const ref = items[i];
        const kept = previous.find(
          (p) =>
            p.installationId === ref.metadata.uid &&
            p.installationVersion === ref.spec.definitionRef.pluginVersion,
        );
        return kept ? [kept] : [];
      }
      if (r.value.def === undefined) return [];
      return [
        mapDefinition(r.value.def, {
          installationId: r.value.installationId,
          installationName: r.value.installationName,
          installationVersion: r.value.installationVersion,
          organizationName: r.value.organizationName,
        }),
      ];
    });
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
    this.activeClusterId = null;
    this.latestLoad = null;
    this.lastSyncFailed = false;
    this.failures = 0;
    this.definitionFailures = 0;
    this.definitionRetryAt = null;
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
