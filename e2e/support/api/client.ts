/**
 * Connect RPC client for E2E tests.
 * Uses @connectrpc/connect-node for proper serialization.
 */

import { createClient, type Client, type DescService, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-node';

export { ConnectError, Code };

/**
 * Create a Connect client for a service.
 */
export function createServiceClient<T extends DescService>(
  service: T,
  baseUrl: string,
  authToken?: string
): Client<T> {
  const transport = createConnectTransport({
    baseUrl,
    httpVersion: '1.1',
    interceptors: authToken
      ? [
          (next) => async (req) => {
            req.header.set('Authorization', `Bearer ${authToken}`);
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
