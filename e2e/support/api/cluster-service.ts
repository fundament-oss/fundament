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
  type ListClustersResponse_ClusterSummary,
  type Region,
} from '../generated/v1/cluster_pb.ts';

export type {
  ClusterDetails,
  NodePool,
  CreateNodePoolResponse,
  ClusterEvent,
  ListClustersResponse_ClusterSummary as ClusterSummary,
  Region,
};

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

  async listRegions(): Promise<Region[]> {
    try {
      const response = await this.client.listRegions({});
      return response.regions;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async listClusters(): Promise<ListClustersResponse_ClusterSummary[]> {
    try {
      const response = await this.client.listClusters({});
      return response.clusters;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async getCluster(clusterId: string): Promise<ClusterDetails | undefined> {
    try {
      const response = await this.client.getCluster({ clusterId });
      return response.cluster;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async createCluster(request: {
    name: string;
    region: string;
    kubernetesVersion: string;
  }): Promise<string> {
    try {
      const response = await createWithIdempotency(async (idempotencyKey) => {
        let status = '';
        const response = await this.client.createCluster(request, {
          headers: { [IDEMPOTENCY_KEY_HEADER]: idempotencyKey },
          onHeader(headers) {
            status = headers.get(IDEMPOTENCY_STATUS_HEADER) ?? '';
          },
        });
        return { response, status };
      });
      return response.clusterId;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async deleteCluster(clusterId: string): Promise<void> {
    try {
      await this.client.deleteCluster({ clusterId });
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
  }

  async getKubeconfig(clusterId: string): Promise<string> {
    try {
      const response = await this.client.getKubeconfig({ clusterId });
      return response.kubeconfigContent;
    } catch (err) {
      if (err instanceof ConnectError) {
        throw ConnectRpcError.fromConnectError(err);
      }
      throw err;
    }
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
