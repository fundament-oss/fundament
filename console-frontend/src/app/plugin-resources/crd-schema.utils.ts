import type { CrdObjectSchema, CrdPropertySchema, AdditionalPrinterColumn } from './types';

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

function resolveSimplePath(obj: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((current, part) => {
    if (current === null || current === undefined || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, obj);
}

/**
 * Resolve a Kubernetes jsonPath expression against a resource object.
 */
export function resolveJsonPath(obj: Record<string, unknown>, jsonPath: string): unknown {
  const path = jsonPath.startsWith('.') ? jsonPath.substring(1) : jsonPath;

  // Matches: path[?(@.key == "value")], path[?(@.key == 'value')], path[?(@.key == 42)]
  const filterMatch = path.match(
    /^(.+?)\[\?\(@\.(\w+)\s*==\s*(?:"([^"]*)"|'([^']*)'|(-?\d+(?:\.\d+)?))\)\](?:\.(.+))?$/,
  );

  if (filterMatch) {
    const arrayPath = filterMatch[1];
    const filterKey = filterMatch[2];
    const filterValue = filterMatch[3] ?? filterMatch[4] ?? filterMatch[5];
    const remainingPath = filterMatch[6];

    const array = resolveSimplePath(obj, arrayPath);
    if (!Array.isArray(array)) return undefined;

    const match = array.find(
      (item: Record<string, unknown>) => String(item[filterKey]) === filterValue,
    );
    if (!match) return undefined;

    if (remainingPath) {
      return resolveSimplePath(match as Record<string, unknown>, remainingPath);
    }
    return match;
  }

  return resolveSimplePath(obj, path);
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

/**
 * Format an unknown value as a simple string for display.
 */
export function toSimpleValue(val: unknown): string {
  if (val === null || val === undefined) return '\u2014';
  if (typeof val === 'object') return JSON.stringify(val);
  return String(val);
}

/**
 * Format a cell value for display in a list table based on column type.
 */
export function formatColumnValue(value: unknown, columnType: string): string {
  if (value === null || value === undefined) return '\u2014';

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

/** A run of capitals is an acronym and keeps its case in sentence case. */
function isAcronym(word: string): boolean {
  return /^[A-Z0-9]{2,}$/.test(word);
}

/**
 * Convert a CRD kind (PascalCase) to a human-readable plural label in sentence case.
 * Examples: "Certificate" → "Certificates", "ClusterIssuer" → "Cluster issuers",
 * "FSCInstallation" → "FSC installations"
 */
export function kindToLabel(kind: string): string {
  const words = splitWords(kind);
  if (words.length === 0) return '';
  // Sentence case: only the first word is capitalized — but an acronym keeps its
  // case wherever it appears, so "ClusterHTTPRoute" stays "Cluster HTTP routes".
  //
  // Case is decided on the word as written, before pluralizing: a trailing acronym
  // becomes "FSCs", which no longer looks like an acronym, and lowercasing it would
  // give "Cluster fscs".
  const sentenceCased = words.map((word, i) => (i === 0 || isAcronym(word) ? word : word.toLowerCase()));
  const last = sentenceCased.length - 1;
  sentenceCased[last] = pluralize(sentenceCased[last]);
  return sentenceCased.join(' ');
}

/**
 * Convert a CRD property name to a human-readable label.
 * Examples: "selfAddress" → "Self Address", "peerID" → "Peer ID",
 * "controllerURL" → "Controller URL"
 */
export function fieldNameToLabel(name: string): string {
  const words = splitWords(name);
  if (words.length === 0) return '';
  return [words[0].charAt(0).toUpperCase() + words[0].slice(1), ...words.slice(1)].join(' ');
}
