import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { ConfigService } from '../config.service';
import { NAMESPACE } from '../../connect/tokens';
import { ListProjectNamespacesRequestSchema } from '../../generated/v1/namespace_pb';

/** A project's namespaces as the plugin iframe takes them. */
export interface ProjectNamespaces {
  /** Names on the cluster, which the plugin passes to the Kubernetes API as is. */
  names: string[];
  /** The project-level name for each of names, which the plugin shows. */
  displayNames: Record<string, string>;
}

// Labels cluster-worker puts on every namespace it creates for a project.
const LABEL_NAMESPACE_ID = 'fundament.io/namespace-id';
const LABEL_PROJECT_ID = 'fundament.io/project-id';

interface NamespaceListResponse {
  items?: { metadata?: { name?: string; labels?: Record<string, string> } }[];
}

/**
 * Lists a project's namespaces under their names on the cluster.
 *
 * organization-api derives a namespace's cluster-side name from the current
 * naming scheme ("tnt-<project>--<name>"), but a namespace created under an
 * earlier scheme keeps its old name: Kubernetes cannot rename it. The cluster
 * is the authority, so each name is looked up there by the namespace's id
 * label. When the cluster cannot be asked (unreachable, or the demo build),
 * the derived name stands in.
 */
@Injectable({ providedIn: 'root' })
export default class ProjectNamespacesService {
  private namespaceClient = inject(NAMESPACE);

  private config = inject(ConfigService);

  async list(projectId: string): Promise<ProjectNamespaces> {
    const request = create(ListProjectNamespacesRequestSchema, { projectId });
    const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));

    const clusterIds = [...new Set(response.namespaces.map((n) => n.clusterId))];
    const lookups = await Promise.all(
      clusterIds.map((clusterId) => this.clusterNames(clusterId, projectId)),
    );
    const actual = new Map(lookups.flatMap((names) => [...names]));

    const names: string[] = [];
    const displayNames: Record<string, string> = {};
    response.namespaces.forEach((n) => {
      const name = actual.get(n.id) ?? n.clusterSideName;
      names.push(name);
      displayNames[name] = n.name;
    });
    return { names, displayNames };
  }

  /** Maps namespace id to its name on the cluster; empty when the cluster
   *  cannot be listed. */
  private async clusterNames(clusterId: string, projectId: string): Promise<Map<string, string>> {
    const { kubeApiProxyUrl } = this.config.getConfig();
    const selector = encodeURIComponent(`${LABEL_PROJECT_ID}=${projectId}`);
    const url = `${kubeApiProxyUrl}/clusters/${encodeURIComponent(clusterId)}/api/v1/namespaces?labelSelector=${selector}`;
    try {
      const res = await fetch(url, { credentials: 'include' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const body: NamespaceListResponse = await res.json();
      const names = new Map<string, string>();
      (body.items ?? []).forEach((item) => {
        const id = item.metadata?.labels?.[LABEL_NAMESPACE_ID];
        const name = item.metadata?.name;
        if (id && name) names.set(id, name);
      });
      return names;
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[ProjectNamespaces] Could not list namespaces on the cluster:', err);
      return new Map();
    }
  }
}
