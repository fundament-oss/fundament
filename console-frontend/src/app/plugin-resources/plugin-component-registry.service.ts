import { Injectable, Type } from '@angular/core';

type ComponentLoader = () => Promise<Type<unknown>>;

/**
 * Registry mapping component names (referenced in plugin YAML `customComponents` sections)
 * to lazy-loaded Angular component types.
 *
 * All components are compiled TypeScript — no eval() or new Function() is used.
 * Register components in src/app/plugins/index.ts and call registerPluginComponents()
 * from app.config.ts during app initialisation.
 */
@Injectable({ providedIn: 'root' })
export default class PluginComponentRegistryService {
  private readonly registry = new Map<string, ComponentLoader>();

  register(name: string, loader: ComponentLoader): void {
    this.registry.set(name, loader);
  }

  hasComponent(name: string): boolean {
    return this.registry.has(name);
  }

  async load(name: string): Promise<Type<unknown> | null> {
    const loader = this.registry.get(name);
    if (!loader) return null;
    return loader();
  }
}
