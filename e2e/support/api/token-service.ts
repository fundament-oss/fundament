/**
 * TokenService client for authn-api.
 * Uses generated proto types with Connect RPC.
 */

import { type Client, ConnectError } from '@connectrpc/connect';
import { createServiceClient, ConnectRpcError } from './client.ts';
import { TokenService as TokenServiceDesc, type ExchangeTokenResponse } from '../generated/authn/v1/authn_pb.ts';

export type { ExchangeTokenResponse };

export class TokenService {
  constructor(private baseUrl: string) {}

  /**
   * Exchange an API token for a short-lived JWT.
   * @param apiToken The API token (fun_...) to exchange
   * @returns JWT access token and metadata
   */
  async exchangeToken(apiToken: string): Promise<ExchangeTokenResponse> {
    // Create a client with the API token as the auth header
    const client: Client<typeof TokenServiceDesc> = createServiceClient(
      TokenServiceDesc,
      this.baseUrl,
      apiToken
    );

    try {
      return await client.exchangeToken({});
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }
}
