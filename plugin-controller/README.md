# Plugin Controller

The plugin controller manages the lifecycle of Fundament plugins. It watches `PluginInstallation` custom resources and creates the necessary Kubernetes resources to run each plugin in its own isolated namespace.

## Config environment variables

Environment variables defined in `spec.config` are injected into the plugin container with a `FUNP_` prefix. For example, `LOG_LEVEL: debug` becomes `FUNP_LOG_LEVEL=debug`. This prevents plugins from accidentally or intentionally overriding system environment variables like `KUBERNETES_SERVICE_HOST` or the controller-managed `FUNDAMENT_*` variables.

See [docs/plugins.md](../docs/plugins.md) for full documentation on the plugin system, SDK, controller architecture, and the development sandbox.
