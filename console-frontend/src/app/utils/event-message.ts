/** Keys an API error body puts its human-readable text under, in order of
 *  preference. Matched case-insensitively. */
const MESSAGE_KEYS = [
  'message',
  'msg',
  'detail',
  'details',
  'description',
  'reason',
  'error',
  'title',
];

/** Keys that hold a status code. `status` comes last: Kubernetes uses it for
 *  "Failure", so only a numeric value counts. */
const CODE_KEYS = ['statuscode', 'status_code', 'httpstatus', 'code', 'status'];

/** Nested error bodies are rare past two levels; the cap only stops a
 *  pathological message from recursing forever. */
const MAX_DEPTH = 3;

type JsonRecord = Record<string, unknown>;

/** The formatter itself, handed down so a message found inside a body can be
 *  formatted in turn without the helpers depending on it by name. */
type Format = (text: string, depth: number) => string;

const isRecord = (value: unknown): value is JsonRecord =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const tryParse = (text: string): unknown => {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
};

/** Undo the escaping a message picks up when it is quoted on its way through
 *  Go (`%q`) or JSON-encoded twice, so `{\"code\":422}` reads `{"code":422}`. */
const unescape = (text: string): string => {
  if (!text.includes('\\')) {
    return text;
  }
  const quoted = text.startsWith('"') && text.endsWith('"') ? text : `"${text}"`;
  const parsed = tryParse(quoted);
  if (typeof parsed === 'string') {
    return parsed;
  }
  return text.replace(/\\(["\\/])/g, '$1').replace(/\\[nrt]/g, ' ');
};

/** JSON that decodes to a string was encoded twice; decode it once more. */
const parseBody = (text: string): unknown => {
  const parsed = tryParse(text);
  return typeof parsed === 'string' ? tryParse(parsed) : parsed;
};

const lookup = (body: JsonRecord, keys: string[]): unknown[] => {
  const byLowerKey = new Map(
    Object.entries(body).map(([key, value]) => [key.toLowerCase(), value]),
  );
  return keys.filter((key) => byLowerKey.has(key)).map((key) => byLowerKey.get(key));
};

/** The first value `pick` accepts; later values are not looked at. */
const firstFound = <T>(values: unknown[], pick: (value: unknown) => T | undefined): T | undefined =>
  values.reduce<T | undefined>((found, value) => found ?? pick(value), undefined);

const findCode = (body: JsonRecord): string | undefined =>
  firstFound(lookup(body, CODE_KEYS), (value) => {
    if (typeof value === 'number' || (typeof value === 'string' && /^\d{3}$/.test(value))) {
      return String(value);
    }
    return isRecord(value) ? findCode(value) : undefined;
  }) ??
  // `{"error":{"code":403,...}}` keeps the code next to the message it wraps.
  firstFound(lookup(body, MESSAGE_KEYS), (value) =>
    isRecord(value) ? findCode(value) : undefined,
  );

const findMessage = (body: JsonRecord, depth: number, format: Format): string | undefined => {
  const direct = firstFound(lookup(body, MESSAGE_KEYS), (value) => {
    if (typeof value === 'string' && value.trim()) {
      return format(value, depth + 1);
    }
    return isRecord(value) ? findMessage(value, depth, format) : undefined;
  });
  if (direct) {
    return direct;
  }

  const [errors] = lookup(body, ['errors']);
  if (!Array.isArray(errors)) {
    return undefined;
  }
  const messages = errors
    .map((item: unknown) => {
      if (typeof item === 'string') {
        return item;
      }
      return isRecord(item) ? findMessage(item, depth, format) : undefined;
    })
    .filter((message): message is string => !!message);
  return messages.length > 0 ? messages.join('; ') : undefined;
};

/**
 * Turns a raw cluster event message into something a person can read. Errors
 * from Gardener and the cloud provider arrive as a Go error chain that ends in
 * a JSON response body, often escaped:
 *
 *   create shoot: {\"statusCode\":422,\"message\":\"Invalid machine type\"}
 *
 * becomes
 *
 *   create shoot: Invalid machine type (status 422)
 *
 * Anything that is not recognisably an error body is returned unescaped but
 * otherwise as it was.
 */
const formatEventMessage = (raw: string, depth = 0): string => {
  const text = unescape(raw.trim());
  if (depth >= MAX_DEPTH) {
    return text;
  }

  const start = text.indexOf('{');
  const end = text.lastIndexOf('}');
  if (start === -1 || end <= start) {
    return text;
  }

  const body = parseBody(text.slice(start, end + 1));
  if (!isRecord(body)) {
    return text;
  }

  const message = findMessage(body, depth, formatEventMessage);
  const code = findCode(body);
  if (!message && !code) {
    return text;
  }

  const prefix = text.slice(0, start).replace(/[\s:,;-]+$/, '');
  const suffix = text.slice(end + 1).trim();
  // Go error chains often repeat the innermost message before the body.
  const head =
    message && prefix.endsWith(message) ? prefix : [prefix, message].filter(Boolean).join(': ');

  return [head, code ? `(status ${code})` : '', suffix].filter(Boolean).join(' ');
};

export default formatEventMessage;
