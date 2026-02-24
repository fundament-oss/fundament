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
    if (raw['required']) {
      schema.required = raw['required'] as string[];
    }
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

  const filterMatch = path.match(/^(.+?)\[\?\(@\.(\w+)\s*==\s*"([^"]+)"\)\](?:\.(.+))?$/);

  if (filterMatch) {
    const arrayPath = filterMatch[1];
    const filterKey = filterMatch[2];
    const filterValue = filterMatch[3];
    const remainingPath = filterMatch[4];

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
 * Get visible spec fields for a CRD, applying hiddenFields filter.
 */
export function getVisibleFields(
  specSchema: CrdObjectSchema,
  hiddenFields?: string[],
): [string, CrdPropertySchema][] {
  const hidden = new Set(hiddenFields ?? []);
  return Object.entries(specSchema.properties).filter(([name]) => !hidden.has(name));
}

/**
 * Group fields into form sections based on uiHints formGroups.
 */
export function groupFields(
  specSchema: CrdObjectSchema,
  formGroups?: { name: string; fields: string[] }[],
  hiddenFields?: string[],
): { name: string; fields: [string, CrdPropertySchema][] }[] {
  const hidden = new Set(hiddenFields ?? []);
  const allFields = Object.entries(specSchema.properties).filter(([name]) => !hidden.has(name));

  if (!formGroups || formGroups.length === 0) {
    return [{ name: 'Configuration', fields: allFields }];
  }

  const usedFields = new Set<string>();
  const groups: { name: string; fields: [string, CrdPropertySchema][] }[] = [];

  formGroups.forEach((group) => {
    const fields: [string, CrdPropertySchema][] = group.fields
      .filter((fieldName) => !hidden.has(fieldName))
      .reduce<[string, CrdPropertySchema][]>((acc, fieldName) => {
        const schema = specSchema.properties[fieldName];
        if (schema) {
          acc.push([fieldName, schema]);
          usedFields.add(fieldName);
        }
        return acc;
      }, []);

    if (fields.length > 0) {
      groups.push({ name: group.name, fields });
    }
  });

  const remaining = allFields.filter(([name]) => !usedFields.has(name));
  if (remaining.length > 0) {
    groups.push({ name: 'Other', fields: remaining });
  }

  return groups;
}

/**
 * Build a default value for a CRD property based on its schema.
 */
export function buildDefaultValue(schema: CrdPropertySchema): unknown {
  if (schema.default !== undefined) return schema.default;

  switch (schema.type) {
    case 'string':
      return '';
    case 'integer':
    case 'number':
      return null;
    case 'boolean':
      return false;
    case 'array':
      return [];
    case 'object': {
      if (!schema.properties) return {};
      const obj: Record<string, unknown> = {};
      Object.entries(schema.properties).forEach(([name, propSchema]) => {
        obj[name] = buildDefaultValue(propSchema);
      });
      return obj;
    }
    default:
      return null;
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
    return word.slice(0, -1) + 'ies';
  }
  if (word.endsWith('s') || word.endsWith('x') || word.endsWith('z')) {
    return word + 'es';
  }
  return word + 's';
}

/**
 * Convert a CRD kind (PascalCase) to a human-readable plural label in sentence case.
 * Examples: "Certificate" → "Certificates", "ClusterIssuer" → "Cluster issuers"
 */
export function kindToLabel(kind: string): string {
  const words = kind.replace(/([A-Z])/g, ' $1').trim().split(' ');
  words[words.length - 1] = pluralize(words[words.length - 1]);
  return words.map((w, i) => (i === 0 ? w : w.toLowerCase())).join(' ');
}

/**
 * Convert a CRD property name to a human-readable label.
 */
export function fieldNameToLabel(name: string): string {
  return name
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, (s) => s.toUpperCase())
    .trim();
}

/**
 * Check if a field is required in the schema.
 */
export function isFieldRequired(fieldName: string, schema: CrdObjectSchema): boolean {
  return schema.required?.includes(fieldName) ?? false;
}

/**
 * Resolve status badge class from a resource using statusMapping.
 */
export function resolveStatusBadge(
  resource: KubeResource,
  statusMapping?: { jsonPath: string; values: Record<string, { badge: string; label: string }> },
): { badge: string; label: string } | undefined {
  if (!statusMapping) return undefined;

  const fullObj = {
    metadata: resource.metadata,
    spec: resource.spec,
    status: resource.status ?? {},
  };
  const value = resolveJsonPath(fullObj, statusMapping.jsonPath);
  if (value === undefined || value === null) return undefined;

  return statusMapping.values[String(value)];
}
