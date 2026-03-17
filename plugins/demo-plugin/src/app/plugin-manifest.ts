/**
 * Native Federation exposed module: './PluginManifest'
 *
 * Called by the Fundament host's PluginLoaderService after fetching this remote's
 * remoteEntry.json. The `register` function registers all custom component loaders
 * into the host's PluginComponentRegistryService so that dispatcher components can
 * find and instantiate them.
 *
 * Component names are prefixed with 'demo-' to avoid collisions with other plugins.
 */

/** Minimal interface matching the host's PluginComponentRegistryService. */
interface ComponentRegistry {
  register(name: string, loader: () => Promise<unknown>): void;
}

export function register(registry: ComponentRegistry): void {
  registry.register('demo-SampleItemList', () =>
    import('./components/sample-item-list.component').then((m) => m.default),
  );
  registry.register('demo-SampleItemDetail', () =>
    import('./components/sample-item-detail.component').then((m) => m.default),
  );
  registry.register('demo-SampleItemCreate', () =>
    import('./components/sample-item-create.component').then((m) => m.default),
  );
  registry.register('demo-SampleItemEdit', () =>
    import('./components/sample-item-edit.component').then((m) => m.default),
  );
  registry.register('demo-SampleItemWidget', () =>
    import('./components/sample-item-widget.component').then((m) => m.default),
  );
  registry.register('demo-ScaleModal', () =>
    import('./components/scale-modal.component').then((m) => m.default),
  );
}
