import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type {
  PluginDefinition,
  ParsedCrd,
  RawCrdYaml,
  PluginInstallationHealth,
  PluginInstallationItem,
  PluginInstallationListResponse,
} from './types';
import type { PluginDefinition as ProtoPluginDefinition } from '../../generated/v1/plugin_pb';
import { PLUGIN } from '../../connect/tokens';
import { parseObjectSchema } from './crd-schema.utils';
import { ConfigService } from '../config.service';
import {
  installPhase,
  isInstallInProgress,
  isInstallTerminating,
  isInstallUnhealthy,
} from '../utils/plugin-install-status';

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

/** The names a resource kind can be looked up by: plural, kind, and
 *  "<plural>.<group>". */
export function crdKeys(crd: ParsedCrd): string[] {
  return [crd.plural, crd.kind, `${crd.plural}.${crd.group}`];
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
                edit: v.edit || undefined,
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

interface InstalledPlugin {
  def: ProtoPluginDefinition | undefined;
  installationId: string;
  installationName: string;
  installationVersion: string;
  organizationName: string;
}

/** Identifies one installation at one version: a reinstall or an upgrade
 *  changes it, a status poll that finds only a new phase does not. */
function installationKey(item: PluginInstallationItem): string {
  return `${item.metadata.uid}/${item.spec.definitionRef.pluginVersion}`;
}

function healthOf(item: PluginInstallationItem): PluginInstallationHealth {
  return {
    phase: installPhase(item.status?.phase),
    ready: item.status?.ready ?? false,
    message: item.status?.message ?? '',
  };
}

function sameHealth(
  a: ReadonlyMap<string, PluginInstallationHealth>,
  b: ReadonlyMap<string, PluginInstallationHealth>,
): boolean {
  if (a.size !== b.size) return false;
  return [...a].every(([name, health]) => {
    const other = b.get(name);
    return (
      other !== undefined &&
      other.phase === health.phase &&
      other.ready === health.ready &&
      other.message === health.message
    );
  });
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  // Each installation's phase and message by installation name, from the
  // latest list read. Kept apart from the definitions, which only change with
  // the installation: a phase change must not refetch them, and must still
  // redraw the sidebar's status marker.
  private health = signal<ReadonlyMap<string, PluginInstallationHealth>>(new Map());

  // The cluster the poll loop runs for, and its latest sync; a second
  // loadPlugins for the same cluster waits on that instead of starting over.
  private activeClusterId: string | null = null;

  private latestLoad: Promise<void> | null = null;

  // Whether the latest sync left something unread: the list, or an installed
  // plugin's definition. A loadPlugins then retries at once rather than hand
  // back that result: it comes from a user opening a screen that needs it.
  private lastSyncFailed = false;

  // List reads that failed in a row, to back off from a failure that is not
  // clearing (no RBAC to list installations, no plugins CRD on the cluster).
  private failures = 0;

  // Definition fetches that failed in a row, and when to try them again. Kept
  // apart from the list reads, which go on at the fast pace while an install
  // is on its way: a plugin whose definition is gone from the catalog
  // must not be refetched on every one of those.
  private definitionFailures = 0;

  private definitionRetryAt: number | null = null;

  // Bumped by every load and reset, so a read that a cluster switch overtook
  // can tell its result is stale and drop it.
  private generation = 0;

  // The installations the menu was last built from, by installationKey;
  // null when it has not been built for the current cluster yet.
  private installedKeys: string[] | null = null;

  private pollTimer: ReturnType<typeof setTimeout> | null = null;

  /** How often to re-read while an installation on the cluster is still coming
   *  up or going away, and the first retry after a read that failed. */
  private static readonly POLL_MS = 5000;

  /** The longest wait between retries of a read that keeps failing. */
  private static readonly MAX_BACKOFF_MS = 120000;

  /** How often to re-read while an installation is Degraded or Failed, so its
   *  recovery, or a new reason, shows soon after the controller notices it. */
  private static readonly UNHEALTHY_POLL_MS = 10000;

  /** How often to re-read while nothing is changing, so an install started
   *  after the menu was built still shows up without a reload. */
  private static readonly IDLE_POLL_MS = 30000;

  // Parsed CRDs indexed by plural; key: "${pluginName}/${clusterId}/${plural}"
  private parsedCrdByPlural = new Map<string, ParsedCrd>();

  // The kinds of the CRDs the menus point at, by CRD reference
  // ("dnsendpoints.externaldns.k8s.io" -> "DNSEndpoint"). A CRD's plural is
  // lowercase by Kubernetes' rules, so a label derived from the reference reads
  // "Dnsendpoints"; the kind is the only place the word boundaries and the
  // acronym's case survive. A signal, so the sidebar redraws when they arrive:
  // it is built from the definitions alone, which name the CRDs but do not say
  // how they are spelled.
  private crdKinds = signal<ReadonlyMap<string, string>>(new Map());

  private configService = inject(ConfigService);

  private pluginClient = inject(PLUGIN);

  /** Loads the cluster's plugins, or waits on the load under way for it.
   *  `fresh` reads again now instead of at the next poll, for a project just
   *  opened; a read already under way is fresh enough and is joined. */
  loadPlugins(clusterId: string, { fresh = false } = {}): Promise<void> {
    if (clusterId === this.activeClusterId) {
      // A load that threw left no next read: start one, keeping what the
      // cluster had rather than clearing it.
      if (!this.latestLoad) return this.startRun(clusterId, this.generation);
      // A retry already waiting on the backoff timer runs now instead; one
      // that is under way is what the caller waits on.
      if ((fresh || this.lastSyncFailed) && this.pollTimer) {
        this.stopPolling();
        if (this.definitionRetryAt !== null) this.definitionRetryAt = 0;
        this.startRun(clusterId, this.generation);
      }
      return this.latestLoad;
    }

    this.clearState();
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
   * failed, this re-reads quickly: the sidebar's status marker then clears the
   * moment the plugin runs, without a reload. While one is Degraded or Failed
   * it re-reads at a middle pace, to show a recovery or a new reason soon.
   * Otherwise it keeps reading at a slower pace to notice installs started
   * later. A read that keeps
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
      case 'unhealthy':
        delay = PluginRegistryService.UNHEALTHY_POLL_MS;
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

  /** Reads the cluster's installations and rebuilds the menu from them.
   *  'unhealthy' is a list with a Degraded or Failed installation in it,
   *  'failed' a list that could not be read, 'incomplete' a list that was read
   *  with a plugin whose definition could not be, and whose retry is what to
   *  wait for next. */
  private async sync(
    clusterId: string,
    generation: number,
  ): Promise<'idle' | 'inProgress' | 'unhealthy' | 'incomplete' | 'failed' | 'stale'> {
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
    // Every installation keeps its screens whatever its health: a plugin that
    // is Degraded, Failed or still coming up stays in the sidebar, which says
    // what is wrong, rather than vanishing just when someone looks for it. Only
    // one being removed is left out: its screens are about to go with it.
    const installed = items.filter(
      (item) => !isInstallTerminating(installPhase(item.status?.phase)),
    );
    const keys = installed.map(installationKey);

    const health = new Map(installed.map((item) => [item.metadata.name, healthOf(item)]));
    if (!sameHealth(health, this.health())) this.health.set(health);

    // Nothing installed or removed since the last read, and no failed
    // definition is due another try: keep the menu as it is rather than
    // fetching every definition again and redrawing the sidebar.
    const previous = this.installedKeys;
    const unchanged =
      previous !== null &&
      keys.length === previous.length &&
      keys.every((k, i) => k === previous[i]);
    const retryDue = this.definitionRetryAt !== null && Date.now() >= this.definitionRetryAt;

    if (!unchanged || retryDue) {
      const fetched = await this.fetchDefinitions(installed);
      if (generation !== this.generation) return 'stale';
      this.plugins.set(fetched.definitions);
      this.installedKeys = keys;
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

    // On every sync, not only the ones that rebuilt the menu: a CRD whose kind
    // could not be read keeps a reference-derived label, and the read is worth
    // another try — right after an install the CRD is often not served yet.
    // Resolved kinds are never refetched, so a settled menu reads nothing here.
    // Not awaited either: the sidebar must not wait on it.
    this.loadCrdKinds(clusterId, generation).catch(() => {});

    // An installation that has no phase yet is one the controller has not
    // picked up, which is as much on its way as a Pending one.
    if (items.some((item) => isInstallInProgress(installPhase(item.status?.phase)))) {
      return 'inProgress';
    }
    if (installed.some((item) => isInstallUnhealthy(installPhase(item.status?.phase)))) {
      return 'unhealthy';
    }
    return this.definitionRetryAt === null ? 'idle' : 'incomplete';
  }

  private async fetchDefinitions(
    items: PluginInstallationItem[],
  ): Promise<{ definitions: PluginDefinition[]; complete: boolean }> {
    const results = await Promise.allSettled(
      items.map(async (item): Promise<InstalledPlugin> => {
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
    // flaky fetch does not drop a plugin from the sidebar.
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

  /**
   * Reads the kind of every CRD the menus point at, so the sidebar can spell a
   * resource the way the CRD does. A read that fails is skipped silently: the
   * reference-derived label stands in, and a CRD the user may not read must not
   * take the menu with it.
   */
  private async loadCrdKinds(clusterId: string, generation: number): Promise<void> {
    const known = this.crdKinds();
    const refs = [
      ...new Set(
        this.plugins()
          .flatMap((plugin) => plugin.menu.project ?? [])
          .map((entry) => entry.crd)
          .filter((ref) => ref && !known.has(ref)),
      ),
    ];
    if (refs.length === 0) return;

    const base = this.configService.getConfig().kubeApiProxyUrl.replace(/\/$/, '');
    const found = await Promise.all(
      refs.map(async (ref): Promise<[string, string] | null> => {
        try {
          const response = await fetch(
            `${base}/clusters/${clusterId}/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${ref}`,
            { credentials: 'include' },
          );
          if (!response.ok) return null;
          const raw = (await response.json()) as RawCrdYaml;
          const kind = raw.spec?.names?.kind;
          return kind ? [ref, kind] : null;
        } catch {
          return null;
        }
      }),
    );
    if (generation !== this.generation) return;

    const resolved = found.filter((entry): entry is [string, string] => entry !== null);
    if (resolved.length === 0) return;
    this.crdKinds.set(new Map([...this.crdKinds(), ...resolved]));
  }

  /** The kind of the CRD a menu entry points at, once it has been read. */
  crdKind(crdRef: string): string | undefined {
    return this.crdKinds().get(crdRef);
  }

  private stopPolling(): void {
    if (this.pollTimer) {
      clearTimeout(this.pollTimer);
      this.pollTimer = null;
    }
  }

  /** Reads the plugin's CRDs and caches their schemas. Resolves with what was
   *  read, so a caller is not left without it when a reset or cluster switch
   *  since the read keeps it out of the cache. */
  async loadCrdsForPlugin(
    pluginName: string,
    clusterId: string,
    kubeApiProxyUrl: string,
  ): Promise<ParsedCrd[]> {
    const plugin = this.getPlugin(pluginName);
    if (!plugin) return [];

    const base = kubeApiProxyUrl.replace(/\/$/, '');
    const { generation } = this;

    const results = await Promise.allSettled(
      plugin.crds.map(async (crdName): Promise<ParsedCrd | null> => {
        const url = `${base}/clusters/${clusterId}/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${crdName}`;
        const response = await fetch(url, {
          credentials: 'include',
        });

        if (!response.ok) {
          // eslint-disable-next-line no-console
          console.error(`[PluginRegistry] Failed to fetch CRD ${crdName}: ${response.status}`);
          return null;
        }

        const parsed = parseCrd((await response.json()) as RawCrdYaml);
        // A reset or cluster switch since the read cleared the cache; refilling
        // it would hand a later caller a schema its own read may not get.
        if (generation === this.generation) {
          crdKeys(parsed).forEach((key) =>
            this.parsedCrdByPlural.set(`${pluginName}/${clusterId}/${key}`, parsed),
          );
        }
        return parsed;
      }),
    );
    return results.flatMap((r) => (r.status === 'fulfilled' && r.value ? [r.value] : []));
  }

  reset(): void {
    this.clearState();
    this.activeClusterId = null;
    this.latestLoad = null;
  }

  /** Drops everything read for the active cluster and stops its poll, so a
   *  read still under way finds its result stale. */
  private clearState(): void {
    this.stopPolling();
    this.generation += 1;
    this.lastSyncFailed = false;
    this.failures = 0;
    this.definitionFailures = 0;
    this.definitionRetryAt = null;
    this.installedKeys = null;
    this.plugins.set([]);
    this.health.set(new Map());
    this.parsedCrdByPlural.clear();
    this.crdKinds.set(new Map());
  }

  // Keyed on the installation name, not the definition's `name`: two
  // organizations may publish the same plugin name, and only the installation
  // name tells their installations apart.
  getPlugin(installationName: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.installationName === installationName);
  }

  /** The installation's phase and message from the latest list read; undefined
   *  for one the registry has not read. */
  installationHealth(installationName: string): PluginInstallationHealth | undefined {
    return this.health().get(installationName);
  }

  getCrd(pluginName: string, plural: string, clusterId: string): ParsedCrd | undefined {
    return this.parsedCrdByPlural.get(`${pluginName}/${clusterId}/${plural}`);
  }

  allPlugins = this.plugins.asReadonly();
}
