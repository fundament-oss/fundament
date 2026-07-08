import { inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type {
  PluginDefinition,
  ParsedCrd,
  RawCrdYaml,
  PluginInstallationListResponse,
} from './types';
import type { PluginDefinition as ProtoPluginDefinition } from '../../generated/v1/plugin_pb';
import { PLUGIN } from '../../connect/tokens';
import { parseObjectSchema } from './crd-schema.utils';
import { ConfigService } from '../config.service';

function parseCrd(raw: RawCrdYaml): ParsedCrd {
  const version = raw.spec.versions.find((v) => v.storage) ?? raw.spec.versions[0];

  const specRaw = version.schema.openAPIV3Schema.properties?.['spec'] as
    | Record<string, unknown>
    | undefined;

  const statusRaw = version.schema.openAPIV3Schema.properties?.['status'] as
    | Record<string, unknown>
    | undefined;

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
  installation: { installationId: string; installationName: string; installationVersion: string },
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
        label: undefined,
        icon: e.icon || undefined,
      })),
    },
    crds: def.crds ?? [],
    customComponents:
      Object.keys(def.customComponents).length > 0
        ? Object.fromEntries(
            Object.entries(def.customComponents).map(([k, v]) => [
              k,
              { list: v.list || undefined, detail: v.detail || undefined, create: v.create || undefined },
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
  };
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  private loadedForClusterId: string | null = null;

  // Parsed CRDs indexed by plural; key: "${pluginName}/${clusterId}/${plural}"
  private parsedCrdByPlural = new Map<string, ParsedCrd>();

  private configService = inject(ConfigService);

  private pluginClient = inject(PLUGIN);

  async loadPlugins(clusterId: string): Promise<void> {
    if (clusterId === this.loadedForClusterId) return;

    const { kubeApiProxyUrl } = this.configService.getConfig();

    let listData: PluginInstallationListResponse;

    try {
      const listRes = await fetch(
        `${kubeApiProxyUrl}/clusters/${clusterId}/apis/plugins.fundament.io/v1/plugininstallations`,
        { credentials: 'include' },
      );
      if (!listRes.ok) return;

      listData = (await listRes.json()) as PluginInstallationListResponse;
    } catch {
      return;
    }

    const runningPlugins = (listData.items ?? []).filter(
      (item) => item.status?.phase === 'Running' && item.status?.ready,
    );

    const results = await Promise.allSettled(
      runningPlugins.map(async (item) => {
        const ref = item.spec.definitionRef;
        const res = await firstValueFrom(
          this.pluginClient.getPluginDefinition({
            pluginName: ref.pluginName,
            pluginVersion: ref.pluginVersion,
          }),
        );
        return {
          def: res.definition,
          installationId: item.metadata.uid,
          // installationName is the CR metadata.name; plugin-proxy derives the
          // plugin's namespace/Service as `plugin-<installationName>`, so this —
          // not the definition's display name — drives the iframe asset URL.
          installationName: item.metadata.name,
          installationVersion: ref.pluginVersion,
        };
      }),
    );

    const definitions: PluginDefinition[] = results
      .filter(
        (
          r,
        ): r is PromiseFulfilledResult<{
          def: ProtoPluginDefinition | undefined;
          installationId: string;
          installationVersion: string;
        }> => r.status === 'fulfilled',
      )
      .filter((r) => r.value.def !== undefined)
      .map((r) =>
        mapDefinition(r.value.def as ProtoPluginDefinition, {
          installationId: r.value.installationId,
          installationVersion: r.value.installationVersion,
        }),
      );

    this.plugins.set(definitions);

    this.loadedForClusterId = clusterId;
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
    this.loadedForClusterId = null;
    this.plugins.set([]);
    this.parsedCrdByPlural.clear();
  }

  getPlugin(name: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.name === name);
  }

  getCrd(pluginName: string, plural: string, clusterId: string): ParsedCrd | undefined {
    return this.parsedCrdByPlural.get(`${pluginName}/${clusterId}/${plural}`);
  }

  allPlugins = this.plugins.asReadonly();
}
