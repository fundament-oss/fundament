import type { Data } from '@angular/router';

/** What a plugin resource route's `:id` names, and the page it hangs off. */
export interface PluginRouteScope {
  // Set on the cluster route only; the project route follows the registry's cluster.
  clusterId: string | null;
  projectId: string | null;
  backPath: string;
  menuSection: 'organization' | 'project';
}

/** `id` is the route's `:id`, `data` its (inherited) route data. */
export default function pluginRouteScope(id: string, data: Data): PluginRouteScope {
  if (data['pluginScope'] === 'cluster') {
    return {
      clusterId: id,
      projectId: null,
      backPath: `/clusters/${id}`,
      menuSection: 'organization',
    };
  }
  return { clusterId: null, projectId: id, backPath: `/projects/${id}`, menuSection: 'project' };
}
