import { Injectable, signal } from '@angular/core';
import type { KubeResource } from './types';
import { MOCK_RESOURCES, type MockResourceMap } from './mock-resources';

@Injectable({ providedIn: 'root' })
export default class PluginResourceStoreService {
  private resources = signal<MockResourceMap>(structuredClone(MOCK_RESOURCES));

  listResources(pluginName: string, kind: string): KubeResource[] {
    return this.resources()[pluginName]?.[kind] ?? [];
  }

  getResource(pluginName: string, kind: string, uid: string): KubeResource | undefined {
    const list = this.listResources(pluginName, kind);
    return list.find((r) => r.metadata.uid === uid);
  }

  createResource(pluginName: string, kind: string, resource: KubeResource): string {
    const uid = `${kind.toLowerCase()}-${Date.now()}`;
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

  deleteResource(pluginName: string, kind: string, uid: string): void {
    this.resources.update((current) => {
      const updated = structuredClone(current);
      if (updated[pluginName]?.[kind]) {
        updated[pluginName][kind] = updated[pluginName][kind].filter((r) => r.metadata.uid !== uid);
      }
      return updated;
    });
  }
}
