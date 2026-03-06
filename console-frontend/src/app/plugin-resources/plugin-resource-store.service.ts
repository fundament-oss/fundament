import { Injectable, signal } from '@angular/core';
import type { KubeResource } from './types';
import { MOCK_RESOURCES, type MockResourceMap } from './mock-resources';

// TODO: Replace mock data with real Kubernetes API calls.
@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  private resources = signal<MockResourceMap>(structuredClone(MOCK_RESOURCES));

  listResources(pluginName: string, kind: string): KubeResource[] {
    return this.resources()[pluginName]?.[kind] ?? [];
  }

  getResource(pluginName: string, kind: string, name: string): KubeResource | undefined {
    const list = this.listResources(pluginName, kind);
    return list.find((r) => r.metadata.name === name);
  }

  createResource(pluginName: string, kind: string, resource: KubeResource): string {
    const uid = crypto.randomUUID();
    const newResource: KubeResource = {
      ...resource,
      metadata: {
        ...resource.metadata,
        uid,
        creationTimestamp: new Date().toISOString(),
      },
    };

    this.resources.update((current) => {
      const updated = structuredClone(current);
      if (!updated[pluginName]) updated[pluginName] = {};
      if (!updated[pluginName][kind]) updated[pluginName][kind] = [];
      updated[pluginName][kind] = [...updated[pluginName][kind], newResource];
      return updated;
    });

    return uid;
  }

  deleteResource(pluginName: string, kind: string, name: string): void {
    this.resources.update((current) => {
      const updated = structuredClone(current);
      if (updated[pluginName]?.[kind]) {
        updated[pluginName][kind] = updated[pluginName][kind].filter(
          (r) => r.metadata.name !== name,
        );
      }
      return updated;
    });
  }

  updateResource(pluginName: string, kind: string, name: string, resource: KubeResource): void {
    this.resources.update((current) => {
      const updated = structuredClone(current);
      if (updated[pluginName]?.[kind]) {
        updated[pluginName][kind] = updated[pluginName][kind].map((r) =>
          r.metadata.name === name ? { ...resource, metadata: { ...resource.metadata, name } } : r,
        );
      }
      return updated;
    });
  }
}
