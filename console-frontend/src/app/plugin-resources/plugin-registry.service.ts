import { Injectable, signal } from '@angular/core';
import * as yaml from 'js-yaml';
import type { PluginDefinition, ParsedCrd, RawPluginYaml, RawCrdYaml } from './types';
import { parseObjectSchema } from './crd-schema.utils';

function parseCrd(crdYamlStr: string): ParsedCrd {
  const raw = yaml.load(crdYamlStr) as RawCrdYaml;
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
  const parsedCrds: ParsedCrd[] = raw.crds.map((crdStr) => parseCrd(crdStr));

  return {
    apiVersion: raw.apiVersion,
    kind: 'PluginDefinition',
    metadata: raw.metadata,
    menu: raw.menu,
    uiHints: raw.uiHints,
    customComponents: raw.customComponents,
    crds: parsedCrds,
  };
}

@Injectable({ providedIn: 'root' })
export default class PluginRegistryService {
  private plugins = signal<PluginDefinition[]>([]);

  private loaded = signal(false);

  private readonly pluginFiles = [
    '/plugins/cert-manager.plugin.yaml',
    '/plugins/cnpg.plugin.yaml',
    '/plugins/sample-plugin.plugin.yaml',
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

    const definitions: PluginDefinition[] = results
      .filter((r): r is PromiseFulfilledResult<PluginDefinition> => r.status === 'fulfilled')
      .map((r) => r.value);

    this.plugins.set(definitions);
    this.loaded.set(true);
  }

  getPlugin(name: string): PluginDefinition | undefined {
    return this.plugins().find((p) => p.metadata.name === name);
  }

  getCrd(pluginName: string, kind: string): ParsedCrd | undefined {
    const plugin = this.getPlugin(pluginName);
    return plugin?.crds.find((c) => c.kind === kind);
  }

  getCrdByPlural(pluginName: string, plural: string): ParsedCrd | undefined {
    const plugin = this.getPlugin(pluginName);
    return plugin?.crds.find((c) => c.plural === plural);
  }

  allPlugins = this.plugins.asReadonly();
}
