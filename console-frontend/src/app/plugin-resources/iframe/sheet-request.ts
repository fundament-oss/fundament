import type { ResourceContext, SheetRequest, SheetView } from './postmessage-types';

/** A sheet request that passed parseSheetRequest: the kind resolved, and a
 *  sequence number, so a sheet replaced by another remounts its iframe. */
export interface OpenSheet extends SheetRequest {
  kind: string;
  seq: number;
}

const SHEET_VIEWS: readonly SheetView[] = ['create', 'edit', 'detail'];

// A Kubernetes kind: CamelCase, starting with a capital.
const KIND = /^[A-Z][A-Za-z0-9]{0,62}$/;

// Object names are DNS subdomains (253, dot-separated labels), namespaces DNS
// labels (63).
const NAME =
  /^(?=.{1,253}$)(?:[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$/;
const NAMESPACE = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/;

const PARAM_KEY = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/;
const MAX_PARAMS = 16;
const MAX_PARAM_LENGTH = 1024;

let lastSeq = 0;

function parseResource(value: unknown): ResourceContext | null | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== 'object' || value === null) return null;
  const { name, namespace } = value as Record<string, unknown>;
  if (typeof name !== 'string' || !NAME.test(name)) return null;
  if (namespace === undefined) return { name };
  if (typeof namespace !== 'string' || !NAMESPACE.test(namespace)) return null;
  return { name, namespace };
}

function parseParams(value: unknown): Record<string, string> | null | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null;
  const entries = Object.entries(value);
  if (entries.length > MAX_PARAMS) return null;
  const valid = entries.every(
    ([key, v]) => PARAM_KEY.test(key) && typeof v === 'string' && v.length <= MAX_PARAM_LENGTH,
  );
  return valid ? Object.fromEntries(entries as [string, string][]) : null;
}

/**
 * Reads a plugin:open-sheet message, which comes from plugin code and so is
 * untrusted: null for anything that is not exactly the shape SheetRequest
 * describes. Every value ends up in the sheet page's fundament:init and none
 * in a URL, but the bounds still keep a plugin from handing the console
 * whatever it likes to pass along. Whether the kind and view name a page the
 * plugin declares is the caller's to check, against the plugin's definition.
 */
export default function parseSheetRequest(message: unknown, openerKind: string): OpenSheet | null {
  if (typeof message !== 'object' || message === null) return null;
  const msg = message as Record<string, unknown>;

  const view = msg['view'];
  if (typeof view !== 'string' || !SHEET_VIEWS.includes(view as SheetView)) return null;

  const kind = msg['kind'] ?? openerKind;
  if (typeof kind !== 'string' || !KIND.test(kind)) return null;

  const resource = parseResource(msg['resource']);
  if (resource === null) return null;
  // Editing or showing something needs to know which something.
  if (view !== 'create' && !resource) return null;

  const params = parseParams(msg['params']);
  if (params === null) return null;

  lastSeq += 1;
  return {
    kind,
    view: view as SheetView,
    seq: lastSeq,
    ...(resource ? { resource } : {}),
    ...(params ? { params } : {}),
  };
}
