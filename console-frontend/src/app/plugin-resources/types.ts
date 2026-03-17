// Plugin Definition YAML schema types

export interface PluginDefinition {
  apiVersion: string;
  kind: 'PluginDefinition';
  name: string;
  displayName: string;
  version: string;
  description: string;
  author?: string;
  menu: PluginMenu;
  uiHints?: Record<string, CrdUiHints>; // keyed by CRD kind
  customComponents?: Record<string, ResourceExtension>; // keyed by CRD kind
  dashboardWidgets?: WidgetDefinition[];
  navSections?: NavSectionDefinition[];
  crds: string[];
  /** URL to the remote's remoteEntry.json for Native Federation (Phase 2). */
  bundleUrl?: string;
}

export interface CrdUiHints {
  formGroups?: FormGroup[];
  hiddenFields?: string[];
  editableFields?: string[];
  statusMapping?: StatusMapping;
}

export interface FormGroup {
  name: string;
  fields: string[];
}

export interface StatusMapping {
  jsonPath: string;
  values: Record<string, { badge: string; label: string }>;
}

export interface ResourceExtension {
  list?: string; // component name in registry
  detail?: string;
  createWizard?: string;
  edit?: string;
  actions?: ActionDefinition[];
}

export interface ActionDefinition {
  label: string;
  icon?: string;
  modal: string; // component name in registry
}

export interface WidgetDefinition {
  id: string;
  title: string;
  size: 'small' | 'medium' | 'large';
  component: string; // component name in registry
}

export interface NavSectionDefinition {
  label: string;
  icon?: string;
  path: string; // sub-path under /plugin-resources/:pluginName/
  component: string; // component name in registry
}

export interface PluginMenu {
  organization?: PluginMenuItem[];
  project?: PluginMenuItem[];
}

export interface PluginMenuItem {
  crd: string;
  label: string;
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
    resourceVersion?: string;
  };
  spec: Record<string, unknown>;
  status?: Record<string, unknown>;
}

// Navigation types

export interface PluginNavGroup {
  pluginName: string;
  displayName: string;
  items: PluginNavItem[];
}

export interface PluginNavItem {
  label: string;
  crdKind: string;
  icon?: string;
}

// Raw YAML types (before parsing)

export interface RawPluginYaml {
  apiVersion: string;
  kind: string;
  name: string;
  displayName: string;
  version: string;
  description: string;
  author?: string;
  menu: PluginMenu;
  uiHints?: Record<string, CrdUiHints>;
  customComponents?: Record<string, ResourceExtension>;
  dashboardWidgets?: WidgetDefinition[];
  navSections?: NavSectionDefinition[];
  crds: string[];
  bundleUrl?: string;
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
