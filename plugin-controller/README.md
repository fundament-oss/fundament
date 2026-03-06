# Plugin Controller

The plugin controller manages the lifecycle of Fundament plugins. It watches `PluginInstallation` custom resources and creates the necessary Kubernetes resources to run each plugin in its own isolated namespace.

See [docs/plugins.md](../docs/plugins.md) for full documentation on the plugin system, SDK, controller architecture, and the development sandbox.
