import { Injectable, inject } from '@angular/core';
import { loadRemoteModule } from '@angular-architects/native-federation';
import PluginComponentRegistryService from './plugin-component-registry.service';
import PluginRegistryService from './plugin-registry.service';

/** Minimal interface that a remote plugin bundle must expose via './PluginManifest'. */
interface RemotePluginManifest {
  register(registry: PluginComponentRegistryService): void;
}

/**
 * Loads remote Native Federation plugin bundles at runtime.
 *
 * For each installed plugin that declares a `bundleUrl`, fetches the remote
 * entry and calls its `register()` function to register custom components.
 * Load failures are logged but do not crash the app — the affected plugin
 * falls back to the auto-generated UI provided by the dispatcher components.
 */
@Injectable({ providedIn: 'root' })
export default class PluginLoaderService {
  private registry = inject(PluginComponentRegistryService);

  private pluginRegistry = inject(PluginRegistryService);

  async loadRemoteBundles(): Promise<void> {
    await Promise.all(
      this.pluginRegistry
        .allPlugins()
        .filter((plugin) => plugin.bundleUrl)
        .map(async (plugin) => {
          try {
            const mod = await loadRemoteModule<RemotePluginManifest>({
              remoteEntry: plugin.bundleUrl!,
              exposedModule: './PluginManifest',
            });
            mod.register(this.registry);
          } catch (e) {
            // eslint-disable-next-line no-console
            console.error(`[PluginLoader] Failed to load bundle for plugin "${plugin.name}":`, e);
          }
        }),
    );
  }
}
