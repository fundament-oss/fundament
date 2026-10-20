/**
 * ClusterService client for organization-api.
 * Uses generated proto types with Connect RPC.
 */

import { type Client, ConnectError } from '@connectrpc/connect';
import {
  createServiceClient,
  createWithIdempotency,
  ConnectRpcError,
  IDEMPOTENCY_KEY_HEADER,
  IDEMPOTENCY_STATUS_HEADER,
} from './client.ts';
import {
  ClusterService as ClusterServiceDesc,
  type ClusterDetails,
  type NodePool,
  type CreateNodePoolResponse,
  type ClusterEvent,
} from '../generated/v1/cluster_pb.ts';

export type { ClusterDetails, NodePool, CreateNodePoolResponse, ClusterEvent };

export class ClusterService {
  private client: Client<typeof ClusterServiceDesc>;

  constructor(baseUrl: string, authToken: string, organizationId?: string) {
    this.client = createServiceClient(
      ClusterServiceDesc,
      baseUrl,
      authToken,
      organizationId,
    );
  }

  async getClusterByName(name: string): Promise<ClusterDetails | undefined> {
    try {
      const response = await this.client.getClusterByName({ name });
      return response.cluster;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async listNodePools(clusterId: string): Promise<NodePool[]> {
    try {
      const response = await this.client.listNodePools({ clusterId });
      return response.nodePools;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async createNodePool(request: {
    clusterId: string;
    name: string;
    machineType: string;
    autoscaleMin: number;
    autoscaleMax: number;
  }): Promise<CreateNodePoolResponse> {
    try {
      return await createWithIdempotency(async (idempotencyKey) => {
        let status = '';
        const response = await this.client.createNodePool(request, {
          headers: { [IDEMPOTENCY_KEY_HEADER]: idempotencyKey },
          onHeader(headers) {
            status = headers.get(IDEMPOTENCY_STATUS_HEADER) ?? '';
          },
        });
        return { response, status };
      });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async deleteNodePool(nodePoolId: string): Promise<void> {
    try {
      await this.client.deleteNodePool({ nodePoolId });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async getClusterActivity(
    clusterId: string,
    limit = 50,
  ): Promise<ClusterEvent[]> {
    try {
      const response = await this.client.getClusterActivity({
        clusterId,
        limit,
      });
      return response.events;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }
}
