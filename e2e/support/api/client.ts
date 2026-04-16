/**
 * Connect RPC client for E2E tests.
 * Uses @connectrpc/connect-node for proper serialization.
 */

import { createClient, type Client, type DescService, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-node';

export const IDEMPOTENCY_KEY_HEADER = 'Idempotency-Key';
export const IDEMPOTENCY_STATUS_HEADER = 'Idempotency-Status';

const IDEMPOTENCY_INITIAL_BACKOFF_MS = 100;
const IDEMPOTENCY_MAX_BACKOFF_MS = 2000;
const IDEMPOTENCY_TOTAL_BUDGET_MS = 30000;

/**
 * Wraps a Create RPC call with idempotency and polling.
 *
 * Generates a UUID key and passes it to `caller`, which must inject it as the
 * `Idempotency-Key` request header and capture `Idempotency-Status` from the
 * response headers. Polls until the server reports `completed` or `failed`.
 *
 * Any error thrown by `caller` (e.g. a ConnectError) propagates as-is.
 */
export async function createWithIdempotency<Resp>(
  caller: (idempotencyKey: string) => Promise<{ response: Resp; status: string }>
): Promise<Resp> {
  const key = crypto.randomUUID();
  const deadline = Date.now() + IDEMPOTENCY_TOTAL_BUDGET_MS;
  let backoff = IDEMPOTENCY_INITIAL_BACKOFF_MS;

  while (true) {
    const { response, status } = await caller(key);

    if (status === 'completed') {
      return response;
    }
    if (status === 'failed') {
      throw new Error('Server reported idempotent operation failed');
    }

    const remaining = deadline - Date.now();
    if (remaining <= 0) {
      throw new Error('Idempotency polling timed out after 30s');
    }

    await new Promise((resolve) => setTimeout(resolve, Math.min(backoff, remaining)));
    backoff = Math.min(backoff * 2, IDEMPOTENCY_MAX_BACKOFF_MS);
  }
}

export { ConnectError, Code };

/**
 * Create a Connect client for a service.
 */
export function createServiceClient<T extends DescService>(
  service: T,
  baseUrl: string,
  authToken?: string,
  organizationId?: string
): Client<T> {
  const transport = createConnectTransport({
    baseUrl,
    httpVersion: '1.1',
    interceptors: authToken
      ? [
          (next) => async (req) => {
            req.header.set('Authorization', `Bearer ${authToken}`);
            if (organizationId) {
              req.header.set('Fun-Organization', organizationId);
            }
            return next(req);
          },
        ]
      : [],
  });

  return createClient(service, transport);
}

/**
 * Legacy error class for backwards compatibility with step definitions.
 */
export class ConnectRpcError extends Error {
  constructor(
    public readonly code: string,
    message: string
  ) {
    super(message);
    this.name = 'ConnectRpcError';
  }

  static fromConnectError(err: ConnectError): ConnectRpcError {
    // Map Connect error codes to our string codes
    const codeMap: Record<number, string> = {
      [Code.Unauthenticated]: 'unauthenticated',
      [Code.NotFound]: 'not_found',
      [Code.AlreadyExists]: 'already_exists',
      [Code.InvalidArgument]: 'invalid_argument',
      [Code.PermissionDenied]: 'permission_denied',
      [Code.Internal]: 'internal',
    };
    const code = codeMap[err.code] || 'unknown';
    return new ConnectRpcError(code, err.message);
  }
}
