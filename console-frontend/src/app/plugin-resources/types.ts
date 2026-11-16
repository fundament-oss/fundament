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
  organization?: PluginMenuItem[];
  project?: PluginMenuItem[];
}

export interface PluginMenuItem {
  crd: string;
  label?: string;
  singularLabel?: string;
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
  /** Opened only in a sheet (see iframe/plugin-sheet.component.ts). */
  edit?: string;
}

// Navigation types

export interface PluginNavGroup {
  // The route segment: the installation name, since a plugin name alone does
  // not identify an installation.
  installationName: string;
  label: string;
  items: PluginNavItem[];
  // What is wrong with the installation, for a marker next to its heading;
  // null while it runs and is ready.
  issue: PluginInstallIssue | null;
}

/** The health of one installation, as its CR's status reports it. */
export interface PluginInstallationHealth {
  // installPhase() of status.phase: an installation without one is Pending.
  phase: string;
  ready: boolean;
  // Why the controller or the plugin set this phase; '' when nothing is said.
  message: string;
}

/** An installation that is not running as it should, as the console shows it. */
export interface PluginInstallIssue {
  label: string;
  badgeColor: string;
  // Still on its way up or down, rather than broken.
  inProgress: boolean;
  message: string;
}

export interface PluginNavItem {
  label: string;
  crdPlural: string;
  icon?: string;
}

// One entry of the CR's status.conditions (metav1.Condition); only the fields
// the console reads. `status` is the Kubernetes spelling: 'True' | 'False' |
// 'Unknown'.
export interface PluginInstallationCondition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
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
    // Key-value config injected as FUNP_* env vars; absent when the install
    // uses only defaults.
    config?: Record<string, string>;
  };
  status: {
    phase: string;
    ready: boolean;
    // Why the installation is in this phase: the plugin's own status message,
    // or what the controller could not do.
    message?: string;
    conditions?: PluginInstallationCondition[];
  };
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
