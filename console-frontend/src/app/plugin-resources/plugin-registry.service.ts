import { inject, Injectable, signal } from '@angular/core';
import type {
  PluginDefinition,
  ParsedCrd,
  RawCrdYaml,
  PluginInstallationListResponse,
  GetDefinitionResponse,
} from './types';
import { parseObjectSchema } from './crd-schema.utils';
import { ConfigService } from '../config.service';
import OrganizationContextService from '../organization-context.service';

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

function toTablerIconName(icon: string): string {
  return `tabler${icon
    .split('-')
    .map((s) => s.charAt(0).toUpperCase() + s.slice(1))
    .join('')}`;
}

function mapDefinition(def: GetDefinitionResponse): PluginDefinition {
  return {
    apiVersion: def.apiVersion,
    kind: 'PluginDefinition',
    name: def.name,
    label: def.displayName,
    version: def.version,
    description: def.description,
    author: def.author,
    menu: {
      project: def.menu.project?.map((e) => ({
        crd: e.crd,
        label: e.label,
        icon: e.icon ? toTablerIconName(e.icon) : undefined,
      })),
    },
    crds: def.crds ?? [],
    customUI: def.customUI,
  };
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  private loadedForClusterId: string | null = null;

  // Parsed CRDs indexed by plural; key: "${pluginName}/${clusterId}/${plural}"
  private parsedCrdByPlural = new Map<string, ParsedCrd>();

  private configService = inject(ConfigService);

  private organizationContextService = inject(OrganizationContextService);

  async loadPlugins(clusterId: string): Promise<void> {
    if (clusterId === this.loadedForClusterId) return;

    const { kubeApiProxyUrl } = this.configService.getConfig();

    const orgId =
      this.organizationContextService.currentOrganizationId() ??
      OrganizationContextService.getStoredOrganizationId();

    if (!orgId) return;

    const headers: Record<string, string> = {
      'Fun-Organization': orgId,
      'Fun-Cluster': clusterId,
    };

    let listData: PluginInstallationListResponse;

    try {
      const listRes = await fetch(
        `${kubeApiProxyUrl}/apis/plugins.fundament.io/v1/plugininstallations`,
        { credentials: 'include', headers },
      );
      if (!listRes.ok) return;

      listData = (await listRes.json()) as PluginInstallationListResponse;
    } catch {
      return;
    }

    const runningPlugins = (listData.items ?? []).filter(
      (item) => item.status.phase === 'Running' && item.status.ready,
    );

    const results = await Promise.allSettled(
      runningPlugins.map(async (item) => {
        const { pluginName } = item.spec;
        const defRes = await fetch(
          `${kubeApiProxyUrl}/api/v1/namespaces/plugin-${pluginName}/services/http:plugin-${pluginName}:8080/proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition`,
          { credentials: 'include', headers },
        );
        if (!defRes.ok) {
          throw new Error(`Failed to fetch definition for ${pluginName}: ${defRes.status}`);
        }

        return defRes.json() as Promise<GetDefinitionResponse>;
      }),
    );

    const definitions: PluginDefinition[] = results
      .filter((r): r is PromiseFulfilledResult<GetDefinitionResponse> => r.status === 'fulfilled')
      .map((r) => mapDefinition(r.value));

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
