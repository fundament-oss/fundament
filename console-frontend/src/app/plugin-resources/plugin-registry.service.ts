import { Injectable, signal } from '@angular/core';
import * as yaml from 'js-yaml';
import type { PluginDefinition, ParsedCrd, RawPluginYaml, RawCrdYaml } from './types';
import { parseObjectSchema } from './crd-schema.utils';

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

function parsePluginYaml(yamlText: string): PluginDefinition {
  const raw = yaml.load(yamlText) as RawPluginYaml;

  return {
    apiVersion: raw.apiVersion,
    kind: 'PluginDefinition',
    name: raw.name,
    label: raw.label,
    version: raw.version,
    description: raw.description,
    author: raw.author,
    menu: raw.menu,
    crds: raw.crds,
  };
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  private loaded = signal(false);

  // Tracks which CRDs have already been fetched; key: "${pluginName}/${clusterId}/${crdK8sName}"
  private fetchedCrdKeys = new Set<string>();

  // Parsed CRDs indexed for O(1) lookup by kind; key: "${pluginName}/${clusterId}/${kind}"
  private parsedCrdByKind = new Map<string, ParsedCrd>();

  private readonly pluginFiles = [
    '/plugins/cert-manager/cert-manager.plugin.yaml',
    '/plugins/cnpg/cnpg.plugin.yaml',
  ];

  async loadPlugins(): Promise<void> {
    if (this.loaded()) return;

    const results = await Promise.allSettled(
      this.pluginFiles.map(async (file) => {
        const response = await fetch(file);
        if (!response.ok) {
          throw new Error(`Failed to load plugin file ${file}: ${response.status}`);
        }
        const text = await response.text();
        return parsePluginYaml(text);
      }),
    );

    results
      .filter((r): r is PromiseRejectedResult => r.status === 'rejected')
      // eslint-disable-next-line no-console
      .forEach((r) => console.error('[PluginRegistry] Failed to load plugin:', r.reason));

    const definitions: PluginDefinition[] = results
      .filter((r): r is PromiseFulfilledResult<PluginDefinition> => r.status === 'fulfilled')
      .map((r) => r.value);

    this.plugins.set(definitions);
    this.loaded.set(true);
  }

  async loadCrdsForPlugin(
    pluginName: string,
    clusterId: string,
    orgApiUrl: string,
    orgId: string,
  ): Promise<void> {
    const plugin = this.getPlugin(pluginName);
    if (!plugin) return;

    const base = orgApiUrl.replace(/\/$/, '');

    await Promise.allSettled(
      plugin.crds.map(async (crdName) => {
        const cacheKey = `${pluginName}/${clusterId}/${crdName}`;
        if (this.fetchedCrdKeys.has(cacheKey)) return;

        const url = `${base}/k8sproxy/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${crdName}`;
        const response = await fetch(url, {
          credentials: 'include',
          headers: { 'Fun-Organization': orgId, 'Fun-Cluster': clusterId },
        });
        if (!response.ok) {
          // eslint-disable-next-line no-console
          console.error(`[PluginRegistry] Failed to fetch CRD ${crdName}: ${response.status}`);
          return;
        }

        const raw = (await response.json()) as RawCrdYaml;
        const parsed = parseCrd(raw);
        this.fetchedCrdKeys.add(cacheKey);
        this.parsedCrdByKind.set(`${pluginName}/${clusterId}/${parsed.kind}`, parsed);
      }),
    );
  }

  getPlugin(name: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.name === name);
  }

  getCrd(pluginName: string, kind: string, clusterId: string): ParsedCrd | undefined {
    return this.parsedCrdByKind.get(`${pluginName}/${clusterId}/${kind}`);
  }

  allPlugins = this.plugins.asReadonly();
}
