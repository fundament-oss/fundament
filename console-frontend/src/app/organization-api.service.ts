import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';

const CONFIG = {
  apiBaseUrl: 'http://organization.127.0.0.1.nip.io:8080',
  servicePath: '/organization.v1.OrganizationService',
};

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

@Injectable({
  providedIn: 'root',
})
export class OrganizationApiService {
  private apiService = inject(ApiService);

  private async connectRpc<T>(method: string, request: object = {}): Promise<T> {
    const url = `${CONFIG.apiBaseUrl}${CONFIG.servicePath}/${method}`;

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

    return response.json();
  }

  async getTenant(id: string): Promise<Tenant> {
    const response = await this.connectRpc<GetTenantResponse>('GetTenant', { id });
    return response.tenant;
  }

  async updateTenant(id: string, name: string): Promise<Tenant> {
    const response = await this.connectRpc<UpdateTenantResponse>('UpdateTenant', { id, name });
    return response.tenant;
  }
}
