import { Injectable, Type } from '@angular/core';

type ComponentLoader = () => Promise<Type<unknown>>;

/**
 * Registry mapping component names (referenced in plugin YAML `customComponents` sections)
 * to lazy-loaded Angular component types.
 *
 * All components are compiled TypeScript — no eval() or new Function() is used.
 * Register components in src/app/plugins/index.ts and call registerPluginComponents()
 * at app initialization.
 *
 * Phase 2 note: this registry will be populated by PluginLoaderService loading remote
 * Native Federation bundles instead of statically registered components.
 */
@Injectable({ providedIn: 'root' })
export default class PluginComponentRegistryService {
  private registry = new Map<string, ComponentLoader>();

  /**
   * Register a component under a given name. The loader is a function returning
   * a promise of the component type (e.g. an async import).
   */
  register(name: string, loader: ComponentLoader): void {
    this.registry.set(name, loader);
  }

  /**
   * Load a registered component by name. Returns undefined if not found.
   */
  async load(name: string): Promise<Type<unknown> | undefined> {
    const loader = this.registry.get(name);
    if (!loader) return undefined;
    return loader();
  }

  /**
   * Check whether a component name is registered.
   */
  hasComponent(name: string): boolean {
    return this.registry.has(name);
  }
}
