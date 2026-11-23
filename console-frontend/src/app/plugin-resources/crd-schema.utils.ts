import type {
  CrdObjectSchema,
  CrdPropertySchema,
  AdditionalPrinterColumn,
  KubeResource,
} from './types';

function parsePropertySchema(raw: Record<string, unknown>): CrdPropertySchema {
  const schema: CrdPropertySchema = {
    type: (raw['type'] as CrdPropertySchema['type']) ?? 'string',
  };

  if (raw['description']) schema.description = raw['description'] as string;
  if (raw['enum']) schema.enum = raw['enum'] as (string | number | boolean)[];
  if (raw['format']) schema.format = raw['format'] as string;
  if (raw['default'] !== undefined) schema.default = raw['default'];
  if (raw['required']) schema.required = raw['required'] as string[];

  if (raw['properties']) {
    schema.properties = {};
    const nestedProps = raw['properties'] as Record<string, Record<string, unknown>>;
    Object.entries(nestedProps).forEach(([name, propDef]) => {
      schema.properties![name] = parsePropertySchema(propDef);
    });
  }

  if (raw['items']) {
    const itemsRaw = raw['items'] as Record<string, unknown>;
    schema.items = parsePropertySchema(itemsRaw);
  }

  return schema;
}

/**
 * Parse the openAPIV3Schema properties section into a CrdObjectSchema.
 */
export function parseObjectSchema(
  schema: Record<string, unknown>,
  requiredFields?: string[],
): CrdObjectSchema {
  const properties: Record<string, CrdPropertySchema> = {};
  const rawProps = schema as Record<string, Record<string, unknown>>;

  Object.entries(rawProps).forEach(([name, propDef]) => {
    properties[name] = parsePropertySchema(propDef);
  });

  return { properties, required: requiredFields };
}

// One step of a printer column's JSONPath: a field (".status"), an index
// ("[0]", "[-1]"), every element ("[*]") or a filter ("[?(@.type=="Ready")]").
const JSON_PATH_STEP =
  /\.?([^.[\]]+)|\[(?:(\*)|(-?\d+)|\?\(@\.(\w+)\s*==\s*(?:"([^"]*)"|'([^']*)'|(-?\d+(?:\.\d+)?))\))\]/y;

const isObject = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object';

/** What [*] reaches: an array's elements, or an object's values. */
const children = (value: unknown): unknown[] => {
  if (Array.isArray(value)) return value;
  return isObject(value) ? Object.values(value) : [];
};

/**
 * Resolve a Kubernetes jsonPath expression against a resource object, as far as
 * printer columns use it. A path with [*] gives every value it reaches, the way
 * kubectl prints a Gateway's `.status.addresses[*].value`; any other path gives
 * its first one, so a filter on a condition's type gives that condition's
 * field. Anything this does not read gives undefined.
 */
export function resolveJsonPath(obj: Record<string, unknown>, jsonPath: string): unknown {
  const path = jsonPath.startsWith('.') ? jsonPath.substring(1) : jsonPath;
  let values: unknown[] = [obj];
  let wildcard = false;
  JSON_PATH_STEP.lastIndex = 0;
  while (JSON_PATH_STEP.lastIndex < path.length) {
    const start = JSON_PATH_STEP.lastIndex;
    const step = JSON_PATH_STEP.exec(path);
    if (!step || step.index !== start) return undefined;
    const [, field, all, index, filterKey, ...filterValues] = step;
    if (field !== undefined) {
      values = values.flatMap((v) => (isObject(v) && v[field] !== undefined ? [v[field]] : []));
    } else if (all !== undefined) {
      wildcard = true;
      values = values.flatMap(children);
    } else if (index !== undefined) {
      const i = Number(index);
      values = values.flatMap((v) => {
        if (!Array.isArray(v)) return [];
        const item = v[i < 0 ? v.length + i : i];
        return item === undefined ? [] : [item];
      });
    } else {
      const wanted = filterValues.find((value) => value !== undefined);
      values = values.flatMap((v) =>
        Array.isArray(v)
          ? v.filter((item) => isObject(item) && String(item[filterKey]) === wanted)
          : [],
      );
    }
  }
  if (wildcard) return values.length > 0 ? values : undefined;
  return values[0];
}

/**
 * Format an ISO date string for display.
 */
export function formatDate(isoString: string): string {
  if (!isoString) return '\u2014';
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) return isoString;
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/**
 * Format an unknown value as an ISO date string for display.
 */
export function toDateValue(val: unknown): string {
  return formatDate(String(val ?? ''));
}

/** An RFC 3339 timestamp, the shape Kubernetes writes its times in. */
const ISO_TIMESTAMP = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

/**
 * Format an unknown value as a simple string for display. A timestamp is read
 * as a date rather than as a string: the schema does not always say a field is
 * one, and 2026-10-09T11:08:30.638Z is nobody's idea of a readable moment.
 */
export function toSimpleValue(val: unknown): string {
  if (val === null || val === undefined) return '\u2014';
  if (typeof val === 'string' && ISO_TIMESTAMP.test(val)) return formatDate(val);
  if (typeof val === 'object') return JSON.stringify(val);
  return String(val);
}

/**
 * Format a cell value for display in a list table based on column type.
 */
export function formatColumnValue(value: unknown, columnType: string): string {
  if (value === null || value === undefined) return '\u2014';
  if (Array.isArray(value))
    return value.map((item) => formatColumnValue(item, columnType)).join(', ');

  switch (columnType) {
    case 'date':
      return formatDate(String(value));
    case 'boolean':
      return value ? 'Yes' : 'No';
    case 'integer':
      return String(value);
    default:
      return String(value);
  }
}

/** A printer column's cell for one resource, as the list shows it. */
export function printerColumnValue(resource: KubeResource, col: AdditionalPrinterColumn): string {
  // The whole resource, so a column on apiVersion, kind or another top-level
  // field reads too.
  const fullObj = { ...resource, status: resource.status ?? {} };
  return formatColumnValue(resolveJsonPath(fullObj, col.jsonPath), col.type);
}

/** Whether a printer column reads one of the resource's conditions, such as
 *  `.status.conditions[?(@.type=="Ready")].status`. */
const readsCondition = (col: AdditionalPrinterColumn): boolean =>
  /\.conditions\[/.test(col.jsonPath);

/**
 * The badge a printer column's cell gets, the same as a cluster's status, or
 * null when the cell stays text. The list's card and the detail page's title
 * both carry it. True or False is a condition the platform keeps, not a word,
 * so the column names it: "Ready" reads as Ready and Not ready rather than as
 * True and False, and a column called something else follows along. Unknown is
 * a condition too, but only from a column that reads one: a phase of Unknown
 * is a word, and stays text.
 */
export function printedConditionBadge(
  col: AdditionalPrinterColumn,
  value: string,
): { text: string; color: string } | null {
  if (value === 'True') return { text: col.name, color: 'success' };
  if (value === 'False') {
    return {
      text: `Not ${col.name.charAt(0).toLowerCase()}${col.name.slice(1)}`,
      color: 'warning',
    };
  }
  if (value === 'Unknown' && readsCondition(col)) {
    return { text: `${col.name} unknown`, color: 'neutral' };
  }
  return null;
}

/**
 * Get columns for the list view.
 */
export function getListColumns(
  printerColumns: AdditionalPrinterColumn[],
): AdditionalPrinterColumn[] {
  if (printerColumns.length > 0) return printerColumns;

  return [
    { name: 'Name', jsonPath: '.metadata.name', type: 'string' },
    { name: 'Age', jsonPath: '.metadata.creationTimestamp', type: 'date' },
  ];
}

/**
 * Pluralize an English word (handles common patterns).
 */
function pluralize(word: string): string {
  if (word.endsWith('y') && !'aeiou'.includes(word[word.length - 2])) {
    return `${word.slice(0, -1)}ies`;
  }
  if (
    word.endsWith('s') ||
    word.endsWith('x') ||
    word.endsWith('z') ||
    word.endsWith('ch') ||
    word.endsWith('sh')
  ) {
    return `${word}es`;
  }
  return `${word}s`;
}

/**
 * Split a camelCase / PascalCase identifier into words, keeping runs of capitals
 * (acronyms) intact. Splitting on every capital instead would shatter them:
 * "FSCInstallation" → "F S C Installation", "peerID" → "peer I D".
 *
 * Examples: "ClusterIssuer" → ["Cluster", "Issuer"], "FSCInstallation" → ["FSC",
 * "Installation"], "peerID" → ["peer", "ID"], "HTTPRoute" → ["HTTP", "Route"]
 */
function splitWords(name: string): string[] {
  return name
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2') // end of a word → start of the next
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2') // end of an acronym → start of a word
    .trim()
    .split(/\s+/)
    .filter(Boolean);
}

/**
 * A run of capitals is an acronym and keeps its own case in a label, the plural
 * "s" included: pluralizing "HTTP" gives "HTTPs", which is still the acronym and
 * must not be flattened to "https" by the mid-sentence pass.
 */
function isAcronym(word: string): boolean {
  return /^[A-Z0-9]{2,}s?$/.test(word);
}

/**
 * Capitalize every word of a split identifier, leaving an acronym the case it
 * was written in: "DNS Endpoint" keeps "DNS" rather than becoming "Dns", and
 * "tls CA Bundle" keeps "CA".
 */
function titleCase(words: string[]): string[] {
  return words.map((word) =>
    isAcronym(word) ? word : word.charAt(0).toUpperCase() + word.slice(1),
  );
}

/**
 * Convert a CRD kind (PascalCase) to a human-readable singular label, one word
 * per case transition and acronyms intact.
 * Examples: "Certificate" → "Certificate", "ClusterIssuer" → "Cluster Issuer",
 * "FSCInstallation" → "FSC Installation"
 */
export function kindToSingularLabel(kind: string): string {
  return titleCase(splitWords(kind)).join(' ');
}

/**
 * Convert a CRD kind (PascalCase) to a human-readable plural label, one word per
 * case transition and acronyms intact.
 * Examples: "Certificate" → "Certificates", "CertificateRequest" → "Certificate
 * Requests", "DNSEndpoint" → "DNS Endpoints", "FSCInstallation" → "FSC Installations"
 */
export function kindToLabel(kind: string): string {
  const words = titleCase(splitWords(kind));
  if (words.length === 0) return '';
  // Pluralized after the case of every word is settled, on the word as written:
  // "FSC" and "FSCs" are the same acronym, but only the first says so by its
  // shape alone.
  const last = words.length - 1;
  words[last] = pluralize(words[last]);
  return words.join(' ');
}

/**
 * Convert a CRD reference as it appears in a plugin menu ("certificates.cert-manager.io")
 * to a human-readable label: "Certificates".
 *
 * This is a last resort, not the preferred label: a CRD's plural is lowercase by
 * Kubernetes' rules, so "dnsendpoints" can only ever become "Dnsendpoints" — the
 * word boundaries and the acronym's case are not in the reference to recover.
 * Prefer the CRD's own kind through kindToLabel, or a label the plugin manifest
 * states outright. Unlike kindToLabel this must not pluralize: the reference
 * already carries the plural, so it would yield "certificates.cert-manager.ios".
 */
export function crdRefToLabel(crdRef: string): string {
  const words = titleCase(splitWords(crdRef.split('.')[0]));
  return words.join(' ');
}

/**
 * Convert a CRD property name to a human-readable label.
 * Examples: "selfAddress" → "Self Address", "peerID" → "Peer ID",
 * "controllerURL" → "Controller URL"
 */
export function fieldNameToLabel(name: string): string {
  return titleCase(splitWords(name)).join(' ');
}

/**
 * Lowercase a label for use mid-sentence ("No DNS endpoints found", "New
 * certificate request…"), leaving an acronym its case: a plain toLowerCase()
 * would write "no dns endpoints found".
 */
export function labelMidSentence(label: string): string {
  return label
    .split(' ')
    .map((word) => (isAcronym(word) ? word : word.toLowerCase()))
    .join(' ');
}

/**
 * The name the Console gives a plugin's resource list: the plugin's own menu
 * label first, so the page title matches the nav item (CloudNativePG calls its
 * Clusters "Databases"), then the CRD's kind, and the CRD reference only while
 * the CRD has not loaded.
 */
export function resourceListLabel(
  menuItem: { label?: string } | undefined,
  kind: string | undefined,
  crdRef: string,
): string {
  if (menuItem?.label) return menuItem.label;
  if (kind) return kindToLabel(kind);
  // The reference is "certificates.cert-manager.io", not a kind, so it must not
  // go through kindToLabel: that would pluralize the group.
  return crdRefToLabel(crdRef);
}

/**
 * One item of that list, as it reads mid-sentence ("New database…", not "New
 * databases…"): the plugin's singular label, else the CRD's kind.
 */
export function resourceSingularLabelMidSentence(
  menuItem: { singularLabel?: string } | undefined,
  kind: string | undefined,
): string {
  return labelMidSentence(menuItem?.singularLabel ?? kindToSingularLabel(kind ?? ''));
}
