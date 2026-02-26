import PluginComponentRegistryService from '../plugin-resources/plugin-component-registry.service';

/**
 * Register all compiled plugin components.
 *
 * To add a custom UI component for a plugin:
 * 1. Create the component in src/app/plugins/<plugin-name>/
 * 2. Add a registry.register() call below, using the same name referenced in the
 *    plugin YAML's `customComponents` section.
 * 3. Call registerPluginComponents() from app.config.ts during app initialisation.
 *
 * Components are lazily loaded — only the bundles for components actually used are fetched.
 * No eval() or new Function() is used; all code is compiled TypeScript.
 */
export default function registerPluginComponents(registry: PluginComponentRegistryService): void {
  registry.register('DemoAppListComponent', () =>
    import('./demo-app/demo-app-list.component').then((m) => m.default),
  );
  registry.register('DemoAppDetailComponent', () =>
    import('./demo-app/demo-app-detail.component').then((m) => m.default),
  );
}
