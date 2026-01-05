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

export interface Tenant {
  id: string;
  name: string;
  created: string;
}

export interface GetTenantRequest {
  id: string;
}

export interface GetTenantResponse {
  tenant: Tenant;
}

export interface UpdateTenantRequest {
  id: string;
  name: string;
}

export interface UpdateTenantResponse {
  tenant: Tenant;
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

  private async connectRpc<T>(servicePath: string, method: string, request: object = {}): Promise<T> {
    const url = `${CONFIG.apiBaseUrl}${servicePath}/${method}`;

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    // Get the access token from ApiService
    const token = this.apiService.getAccessToken();
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(url, {
      method: 'POST',
      headers,
      credentials: 'include',
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

  async getTenant(id: string): Promise<Tenant> {
    const response = await this.connectRpc<GetTenantResponse>(CONFIG.organizationServicePath, 'GetTenant', { id });
    return response.tenant;
  }

  async updateTenant(id: string, name: string): Promise<Tenant> {
    const response = await this.connectRpc<UpdateTenantResponse>(CONFIG.organizationServicePath, 'UpdateTenant', { id, name });
    return response.tenant;
  }

  async createCluster(request: CreateClusterRequest): Promise<CreateClusterResponse> {
    return this.connectRpc<CreateClusterResponse>(CONFIG.clusterServicePath, 'CreateCluster', request);
  }

  async updateCluster(request: UpdateClusterRequest): Promise<ClusterDetails> {
    const response = await this.connectRpc<UpdateClusterResponse>(CONFIG.clusterServicePath, 'UpdateCluster', request);
    return response.cluster;
  }

  async getCluster(clusterId: string): Promise<ClusterDetails> {
    const response = await this.connectRpc<GetClusterResponse>(CONFIG.clusterServicePath, 'GetCluster', { clusterId });
    return response.cluster;
  }
}
