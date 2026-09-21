import { type PluginPermission } from './marketplace.service';

// The catalog API projects each RBAC rule of a plugin definition into one
// PluginPermission: `resource` is the rule's Kubernetes resource names joined
// with ", " (plural, lowercase, e.g. "validatingwebhookconfigurations") and
// `access` is "Read" or "Read and write". This file turns that into something
// a visitor who does not know Kubernetes can read.

export interface ResourceEntry {
  // Kubernetes resource name as written in the definition, e.g. "gateways/status".
  name: string;
  label: string;
  description: string;
}

export interface AccessGroup {
  access: string;
  heading: string;
  resources: ResourceEntry[];
}

interface KnownResource {
  label: string;
  description: string;
}

// Built-in Kubernetes resources, plus a few that plugins commonly touch. The
// descriptions say what holding the permission means, not what the API is.
const KNOWN_RESOURCES: Record<string, KnownResource> = {
  '*': {
    label: 'All resources',
    description: 'Every kind of resource in the cluster, including ones added later.',
  },
  pods: { label: 'Pods', description: 'Running containers.' },
  services: { label: 'Services', description: 'Network endpoints for workloads.' },
  endpoints: { label: 'Endpoints', description: 'Addresses behind a service.' },
  endpointslices: { label: 'Endpoint slices', description: 'Addresses behind a service.' },
  configmaps: { label: 'Config maps', description: 'Non-secret configuration data.' },
  secrets: {
    label: 'Secrets',
    description: 'Passwords, keys and certificates stored in the cluster.',
  },
  serviceaccounts: {
    label: 'Service accounts',
    description: 'Identities that workloads use to talk to the cluster.',
  },
  namespaces: { label: 'Namespaces', description: 'The spaces workloads are grouped in.' },
  nodes: { label: 'Nodes', description: 'The machines the cluster runs on.' },
  events: { label: 'Events', description: 'Activity reported by cluster components.' },
  persistentvolumes: { label: 'Persistent volumes', description: 'Storage in the cluster.' },
  persistentvolumeclaims: {
    label: 'Persistent volume claims',
    description: 'Requests by workloads for storage.',
  },
  storageclasses: {
    label: 'Storage classes',
    description: 'The kinds of storage workloads can request.',
  },
  deployments: { label: 'Deployments', description: 'Workloads and how they are rolled out.' },
  replicasets: { label: 'Replica sets', description: 'The pod copies behind a deployment.' },
  statefulsets: { label: 'Stateful sets', description: 'Workloads with stable identity.' },
  daemonsets: { label: 'Daemon sets', description: 'Workloads that run on every node.' },
  jobs: { label: 'Jobs', description: 'One-off tasks.' },
  cronjobs: { label: 'Cron jobs', description: 'Scheduled tasks.' },
  ingresses: { label: 'Ingresses', description: 'Routes for traffic into the cluster.' },
  ingressclasses: {
    label: 'Ingress classes',
    description: 'The controllers that serve ingresses.',
  },
  networkpolicies: {
    label: 'Network policies',
    description: 'Rules for which workloads may talk to each other.',
  },
  leases: { label: 'Leases', description: 'Locks components use to coordinate.' },
  customresourcedefinitions: {
    label: 'Custom resource definitions',
    description: 'New kinds of resources added to the cluster.',
  },
  validatingwebhookconfigurations: {
    label: 'Validating webhooks',
    description: 'Checks that can reject changes made to the cluster.',
  },
  mutatingwebhookconfigurations: {
    label: 'Mutating webhooks',
    description: 'Hooks that can alter changes made to the cluster.',
  },
  roles: { label: 'Roles', description: 'Sets of permissions within a namespace.' },
  rolebindings: {
    label: 'Role bindings',
    description: 'Who is granted a role within a namespace.',
  },
  clusterroles: { label: 'Cluster roles', description: 'Sets of permissions across the cluster.' },
  clusterrolebindings: {
    label: 'Cluster role bindings',
    description: 'Who is granted a cluster role.',
  },
  priorityclasses: { label: 'Priority classes', description: 'Scheduling priorities.' },
  poddisruptionbudgets: {
    label: 'Pod disruption budgets',
    description: 'Limits on how many pods may be down at once.',
  },
  horizontalpodautoscalers: {
    label: 'Horizontal pod autoscalers',
    description: 'Rules that scale workloads with load.',
  },
};

// Parts of a resource a rule can target on their own, as in "gateways/status".
const SUBRESOURCES: Record<string, KnownResource> = {
  status: { label: 'Status', description: 'The state reported back on the resource.' },
  finalizers: {
    label: 'Finalizers',
    description: 'Cleanup steps that must finish before the resource is removed.',
  },
  scale: { label: 'Scaling', description: 'How many copies of a workload run.' },
  log: { label: 'Logs', description: 'Output written by running containers.' },
  exec: { label: 'Command execution', description: 'Running commands inside containers.' },
  portforward: { label: 'Port forwarding', description: 'Direct connections into containers.' },
  proxy: {
    label: 'Proxy access',
    description: 'Requests passed straight through to the resource.',
  },
  eviction: { label: 'Eviction', description: 'Removing pods from their node.' },
};

// Words resource names are made of, used to split names that are not in
// KNOWN_RESOURCES (usually a plugin's own CRDs) such as "clusterissuers".
// Values are how the word is written in a label.
const WORDS: Record<string, string> = {
  acme: 'ACME',
  api: 'API',
  backend: 'backend',
  binding: 'binding',
  block: 'block',
  bucket: 'bucket',
  ca: 'CA',
  ceph: 'Ceph',
  certificate: 'certificate',
  challenge: 'challenge',
  class: 'class',
  claim: 'claim',
  client: 'client',
  cluster: 'cluster',
  config: 'config',
  configuration: 'configuration',
  database: 'database',
  disk: 'disk',
  dns: 'DNS',
  endpoint: 'endpoint',
  file: 'file',
  filesystem: 'file system',
  fsc: 'FSC',
  gateway: 'gateway',
  grant: 'grant',
  grpc: 'gRPC',
  http: 'HTTP',
  installation: 'installation',
  issuer: 'issuer',
  listener: 'listener',
  object: 'object',
  order: 'order',
  peer: 'peer',
  policy: 'policy',
  pool: 'pool',
  proxy: 'proxy',
  realm: 'realm',
  reference: 'reference',
  request: 'request',
  route: 'route',
  rule: 'rule',
  scaled: 'scaled',
  secret: 'secret',
  sealed: 'sealed',
  server: 'server',
  service: 'service',
  snapshot: 'snapshot',
  storage: 'storage',
  store: 'store',
  tcp: 'TCP',
  tls: 'TLS',
  udp: 'UDP',
  user: 'user',
  volume: 'volume',
};

// Every way a dictionary word can start at `start`, in singular or plural
// form: "issuer(s)", "class(es)", "polic(y|ies)".
function wordsAt(name: string, start: number): { end: number; text: string }[] {
  const matches: { end: number; text: string }[] = [];
  Object.entries(WORDS).forEach(([word, text]) => {
    const forms: [string, string][] = [
      [word, text],
      [`${word}s`, `${text}s`],
      [`${word}es`, `${text}es`],
    ];
    if (word.endsWith('y')) forms.push([`${word.slice(0, -1)}ies`, `${text.slice(0, -1)}ies`]);
    forms.forEach(([form, formText]) => {
      if (name.startsWith(form, start)) matches.push({ end: start + form.length, text: formText });
    });
  });
  return matches;
}

// Splits a lowercase name into dictionary words, preferring the fewest words.
// Returns undefined when the name cannot be covered completely.
function splitWords(name: string): string[] | undefined {
  const best: (string[] | undefined)[] = new Array(name.length + 1).fill(undefined);
  best[0] = [];
  for (let start = 0; start < name.length; start += 1) {
    const prefix = best[start];
    if (prefix) {
      wordsAt(name, start).forEach(({ end, text }) => {
        const current = best[end];
        if (!current || current.length > prefix.length + 1) best[end] = [...prefix, text];
      });
    }
  }
  return best[name.length];
}

// Capitalizes the first word unless it has its own casing, like "gRPC".
function sentenceCase(words: string[]): string {
  const sentence = words.join(' ');
  if (words[0] !== words[0].toLowerCase()) return sentence;
  return sentence.charAt(0).toUpperCase() + sentence.slice(1);
}

// Lowercases a label's first letter for use mid-sentence, leaving acronyms.
function midSentence(label: string): string {
  if (label.charAt(1) !== label.charAt(1).toLowerCase()) return label;
  return label.charAt(0).toLowerCase() + label.slice(1);
}

function resourceLabel(name: string): string {
  const known = KNOWN_RESOURCES[name];
  if (known) return known.label;
  const words = splitWords(name);
  return words ? sentenceCase(words) : sentenceCase([name]);
}

export function describeResource(name: string): ResourceEntry {
  const [base, subresource] = name.split('/', 2);
  if (subresource !== undefined) {
    const part = SUBRESOURCES[subresource];
    return {
      name,
      label: `${part?.label ?? sentenceCase([subresource])} of ${midSentence(resourceLabel(base))}`,
      description: part?.description ?? '',
    };
  }
  return {
    name,
    label: resourceLabel(base),
    description: KNOWN_RESOURCES[base]?.description ?? '',
  };
}

const ACCESS_HEADINGS: Record<string, string> = {
  'Read and write': 'Can view and change',
  Read: 'Can view',
};

// Most permissive first: that is what a visitor weighs up before installing.
const ACCESS_ORDER = Object.keys(ACCESS_HEADINGS);

function accessRank(access: string): number {
  const rank = ACCESS_ORDER.indexOf(access);
  return rank === -1 ? ACCESS_ORDER.length : rank;
}

// Merges the per-rule rows into one group per access level. A resource that
// appears in several rules is listed once, under its most permissive access.
export function groupPermissions(permissions: PluginPermission[]): AccessGroup[] {
  const accessByResource = new Map<string, string>();
  permissions.forEach(({ resource, access }) => {
    resource
      .split(',')
      .map((name) => name.trim())
      .filter((name) => name !== '')
      .forEach((name) => {
        const current = accessByResource.get(name);
        if (current === undefined || accessRank(access) < accessRank(current)) {
          accessByResource.set(name, access);
        }
      });
  });

  const groups = new Map<string, ResourceEntry[]>();
  accessByResource.forEach((access, name) => {
    const entries = groups.get(access) ?? [];
    entries.push(describeResource(name));
    groups.set(access, entries);
  });

  return [...groups.entries()]
    .sort(([a], [b]) => accessRank(a) - accessRank(b))
    .map(([access, resources]) => ({
      access,
      heading: ACCESS_HEADINGS[access] ?? access,
      resources: resources.sort((a, b) => a.label.localeCompare(b.label)),
    }));
}

// True when the plugin asks for every resource, which the page calls out.
export function grantsEverything(permissions: PluginPermission[]): boolean {
  return permissions.some(({ resource }) =>
    resource.split(',').some((name) => name.trim() === '*'),
  );
}
