/**
 * APIKeyService client for organization-api.
 * Uses generated proto types with Connect RPC.
 */

import { type Client, ConnectError } from '@connectrpc/connect';
import { createServiceClient, ConnectRpcError } from './client.ts';
import {
  APIKeyService as APIKeyServiceDesc,
  type APIKey,
  type CreateAPIKeyResponse,
  type ListAPIKeysResponse,
  type GetAPIKeyResponse,
} from '../generated/v1/apikey_pb.ts';

export type { APIKey, CreateAPIKeyResponse, ListAPIKeysResponse, GetAPIKeyResponse };

export class APIKeyService {
  private client: Client<typeof APIKeyServiceDesc>;

  constructor(baseUrl: string, authToken: string) {
    this.client = createServiceClient(APIKeyServiceDesc, baseUrl, authToken);
  }

  async createAPIKey(request: { name: string; expiresInDays?: number }): Promise<CreateAPIKeyResponse> {
    try {
      return await this.client.createAPIKey({
        name: request.name,
        expiresInDays: request.expiresInDays ? BigInt(request.expiresInDays) : undefined,
      });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async listAPIKeys(): Promise<ListAPIKeysResponse> {
    try {
      return await this.client.listAPIKeys({});
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async getAPIKey(apiKeyId: string): Promise<GetAPIKeyResponse> {
    try {
      return await this.client.getAPIKey({ apiKeyId });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async revokeAPIKey(apiKeyId: string): Promise<void> {
    try {
      await this.client.revokeAPIKey({ apiKeyId });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async deleteAPIKey(apiKeyId: string): Promise<void> {
    try {
      await this.client.deleteAPIKey({ apiKeyId });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }
}
