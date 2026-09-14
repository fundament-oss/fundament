import { Injectable } from '@angular/core';

// Sideloading a build onto a cluster is deliberately not part of the
// marketplace APIs: it creates a PluginInstallation, so it belongs to
// organization-api (FUN-20 defers it there). Routing it through the publication
// API would give the marketplace cluster-write authority it should not have.
//
// Until that lands this stays a mock in every build, so the flow can still be
// walked end to end on the developer surface.

// A cluster the author can sideload onto. Sideloading targets a normal cluster
// the user already owns; one of them is flagged as a development cluster.
export interface SideloadCluster {
  id: string;
  name: string;
  isDevelopment: boolean;
}

export interface SideloadRequest {
  image: string;
  version: string;
  displayName?: string;
  description?: string;
  clusterId: string;
}

const MOCK_CLUSTERS: SideloadCluster[] = [
  { id: 'cl-dev-01', name: 'team-sandbox', isDevelopment: true },
  { id: 'cl-prod-01', name: 'production-eu-west', isDevelopment: false },
  { id: 'cl-stg-01', name: 'staging-eu-west', isDevelopment: false },
];

@Injectable({ providedIn: 'root' })
export default class SideloadMockService {
  private readonly clusters = MOCK_CLUSTERS;

  // Records sideload requests made during this session (mock only).
  private readonly sideloaded: SideloadRequest[] = [];

  listClusters(): Promise<SideloadCluster[]> {
    return Promise.resolve(this.clusters.map((cluster) => ({ ...cluster })));
  }

  // Mock sideload: records the request and resolves successfully.
  sideload(request: SideloadRequest): Promise<void> {
    this.sideloaded.push(request);
    return Promise.resolve();
  }
}
