// Plugin Definition YAML schema types

export interface PluginDefinition {
  name: string;
  label: string;
  version: string;
  description: string;
  author?: string;
  menu: PluginMenu;
  crds: string[];
  customComponents?: Record<string, CustomComponentMapping>;
  allowedResources: AllowedResource[];
  // The PluginInstallation CR UID this definition was loaded from.
  // Used as installation_id in MintPluginToken and pinned into every JWT.
  installationId: string;
  // The PluginInstallation CR name (metadata.name), "<organizationName>--<name>".
  // The plugin's namespace is derived from installationName (possibly hashed for
  // long names), so this — NOT the definition's own `name` — must drive the
  // plugin-proxy asset URL. It is also the only unique handle: two organizations
  // may publish the same `name`.
  installationName: string;
  // Publishing organization. Shown in the nav to tell apart two installations
  // that share a plugin name.
  organizationName: string;
  // The pluginVersion pinned in PluginInstallation.spec.definitionRef.
  // Drives the plugin-proxy iframe URL and appears in the minted token.
  installationVersion: string;
}

export interface AllowedResource {
  group: string;
  version: string;
  resource: string;
  verbs: string[];
}

export interface PluginMenu {
  project?: PluginMenuItem[];
}

export interface PluginMenuItem {
  crd: string;
  label?: string;
  icon?: string;
}

// Parsed CRD types

export interface ParsedCrd {
  group: string;
  kind: string;
  plural: string;
  singular: string;
  scope: 'Namespaced' | 'Cluster';
  version: string;
  additionalPrinterColumns: AdditionalPrinterColumn[];
  specSchema: CrdObjectSchema;
  statusSchema?: CrdObjectSchema;
}

export interface AdditionalPrinterColumn {
  name: string;
  type: string;
  jsonPath: string;
  priority?: number;
  description?: string;
}

export interface CrdObjectSchema {
  properties: Record<string, CrdPropertySchema>;
  required?: string[];
}

export interface CrdPropertySchema {
  type: 'string' | 'integer' | 'boolean' | 'object' | 'array' | 'number';
  description?: string;
  enum?: (string | number | boolean)[];
  format?: string;
  default?: unknown;
  properties?: Record<string, CrdPropertySchema>;
  required?: string[];
  items?: CrdPropertySchema;
}

// Kubernetes resource instance

export interface KubeResource {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    uid: string;
    creationTimestamp: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: Record<string, unknown>;
  status?: Record<string, unknown>;
}

// Custom UI configuration

export interface CustomComponentMapping {
  list?: string;
  detail?: string;
  create?: string;
}

// Navigation types

export interface PluginNavGroup {
  // The route segment: the installation name, since a plugin name alone does
  // not identify an installation.
  installationName: string;
  label: string;
  items: PluginNavItem[];
}

export interface PluginNavItem {
  label: string;
  crdPlural: string;
  icon?: string;
}

export interface PluginInstallationItem {
  metadata: { name: string; uid: string };
  spec: {
    definitionRef: {
      organizationName: string;
      pluginName: string;
      pluginVersion: string;
      definitionHash: string;
    };
  };
  status: { phase: string; ready: boolean };
}

export interface PluginInstallationListResponse {
  items: PluginInstallationItem[];
}

export interface RawCrdYaml {
  apiVersion: string;
  kind: string;
  metadata: { name: string };
  spec: {
    group: string;
    names: {
      kind: string;
      plural: string;
      singular: string;
      listKind?: string;
      shortNames?: string[];
      categories?: string[];
    };
    scope: string;
    versions: RawCrdVersion[];
  };
}

export interface RawCrdVersion {
  name: string;
  served: boolean;
  storage: boolean;
  additionalPrinterColumns?: AdditionalPrinterColumn[];
  schema: {
    openAPIV3Schema: {
      description?: string;
      properties: Record<string, unknown>;
      required?: string[];
      type: string;
    };
  };
}
