import { Injectable, inject } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { ApiService } from './api.service';
import { PROTO_API_VERSION } from '../proto-version';

const CONFIG = {
  apiBaseUrl: 'http://organization.127.0.0.1.nip.io:8080',
  organizationServicePath: '/organization.v1.OrganizationService',
  clusterServicePath: '/organization.v1.ClusterService',
};

const EXPECTED_API_VERSION = PROTO_API_VERSION;

export interface Organization {
  id: string;
  name: string;
  created: string;
}

export interface GetOrganizationRequest {
  id: string;
}

export interface GetOrganizationResponse {
  organization: Organization;
}

export interface UpdateOrganizationRequest {
  id: string;
  name: string;
}

export interface UpdateOrganizationResponse {
  organization: Organization;
}

export interface NodePoolSpec {
  name: string;
  machineType: string;
  autoscaleMin: number;
  autoscaleMax: number;
}

export interface CreateClusterRequest {
  name: string;
  region: string;
  kubernetesVersion: string;
  nodePools?: NodePoolSpec[];
  pluginIds?: string[];
  pluginPreset?: string;
}

export interface CreateClusterResponse {
  clusterId: string;
  status: string;
}

export interface UpdateClusterRequest {
  clusterId: string;
  kubernetesVersion?: string;
  nodePools?: NodePoolSpec[];
}

export interface UpdateClusterResponse {
  cluster: ClusterDetails;
}

export interface GetClusterRequest {
  clusterId: string;
}

export interface ClusterSummary {
  id: string;
  name: string;
  status: string;
  region: string;
  projectCount: number;
  nodePoolCount: number;
}

export interface ListClustersRequest {
  projectId?: string;
}

export interface ListClustersResponse {
  clusters: ClusterSummary[];
}

export interface ClusterDetails {
  id: string;
  name: string;
  region: string;
  kubernetesVersion: string;
  status: string;
}

export interface GetClusterResponse {
  cluster: ClusterDetails;
}

@Injectable({
  providedIn: 'root',
})
export class OrganizationApiService {
  private apiService = inject(ApiService);
  private versionMismatchSubject = new BehaviorSubject<boolean>(false);
  public versionMismatch$ = this.versionMismatchSubject.asObservable();

  private async connectRpc<T>(
    servicePath: string,
    method: string,
    request: object = {},
  ): Promise<T> {
    const url = `${CONFIG.apiBaseUrl}${servicePath}/${method}`;

    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include', // Authentication via HTTP-only cookies
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `Request failed: ${response.status}`);
    }

    // Check API version from response header
    const serverVersion = response.headers.get('X-API-Version');
    if (serverVersion && serverVersion !== EXPECTED_API_VERSION) {
      console.warn(`API version mismatch: expected ${EXPECTED_API_VERSION}, got ${serverVersion}`);
      this.versionMismatchSubject.next(true);
    }

    return response.json();
  }

  async getOrganization(id: string): Promise<Organization> {
    const response = await this.connectRpc<GetOrganizationResponse>(
      CONFIG.organizationServicePath,
      'GetOrganization',
      { id },
    );
    return response.organization;
  }

  async updateOrganization(id: string, name: string): Promise<Organization> {
    const response = await this.connectRpc<UpdateOrganizationResponse>(
      CONFIG.organizationServicePath,
      'UpdateOrganization',
      { id, name },
    );
    return response.organization;
  }

  async createCluster(request: CreateClusterRequest): Promise<CreateClusterResponse> {
    return this.connectRpc<CreateClusterResponse>(
      CONFIG.clusterServicePath,
      'CreateCluster',
      request,
    );
  }

  async updateCluster(request: UpdateClusterRequest): Promise<ClusterDetails> {
    const response = await this.connectRpc<UpdateClusterResponse>(
      CONFIG.clusterServicePath,
      'UpdateCluster',
      request,
    );
    return response.cluster;
  }

  async getCluster(clusterId: string): Promise<ClusterDetails> {
    const response = await this.connectRpc<GetClusterResponse>(
      CONFIG.clusterServicePath,
      'GetCluster',
      { clusterId },
    );
    return response.cluster;
  }

  async listClusters(projectId?: string): Promise<ClusterSummary[]> {
    const response = await this.connectRpc<ListClustersResponse>(
      CONFIG.clusterServicePath,
      'ListClusters',
      projectId ? { projectId } : {},
    );
    return response.clusters;
  }
}
