import { Injectable, inject } from '@angular/core';

import { ConfigService } from '../config.service';

export interface ShootPod {
  name: string;
  containers: string[];
}

interface PodListResponse {
  items?: {
    metadata?: { name?: string };
    spec?: { containers?: { name: string }[] };
  }[];
}

/**
 * Lists pods in one shoot namespace through the kube-api-proxy, with the
 * caller's own credentials (per-user RBAC applies — members without access
 * simply get an error). Used by the log explorer's live mode to populate the
 * pod/container dropdowns for plugin namespaces, which Vali does not cover.
 */
@Injectable({ providedIn: 'root' })
export class ShootPodsService {
  private configService = inject(ConfigService);

  async listPods(clusterId: string, namespace: string): Promise<ShootPod[]> {
    const { kubeApiProxyUrl } = this.configService.getConfig();
    const url = `${kubeApiProxyUrl}/clusters/${clusterId}/api/v1/namespaces/${namespace}/pods`;
    const res = await fetch(url, { credentials: 'include' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const body: PodListResponse = await res.json();
    return (body.items ?? [])
      .map((item) => ({
        name: item.metadata?.name ?? '',
        containers: (item.spec?.containers ?? []).map((c) => c.name),
      }))
      .filter((p) => p.name !== '');
  }
}
