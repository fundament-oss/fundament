import type PluginComponentRegistryService from '../plugin-resources/plugin-component-registry.service';

/**
 * Register all compiled plugin components.
 *
 * To add a custom UI component for a plugin:
 * 1. Create the component in src/app/plugins/<plugin-name>/
 * 2. Add a registry.register() call below, using the same name referenced in the
 *    plugin YAML's `customComponents` section.
 * 3. The component will be loaded on demand by the dispatcher.
 *
 * Phase 2 note: this function is a temporary in-bundle arrangement.
 * It will be replaced by PluginLoaderService loading remote Native Federation bundles.
 * Do not grow this file beyond Phase 1 needs.
 */
export default function registerPluginComponents(
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  registry: PluginComponentRegistryService,
): void {
  // cert-manager custom components (if any)
  // cnpg custom components (if any)
  // sample plugin for development/testing
}
