# Plugin Sandbox

A self-contained development environment lives in `sandbox/`. It creates an isolated K3D cluster with only the plugin controller -- no database, auth services, or other Fundament components needed. The sandbox cluster (`fundament-plugin`) uses a separate registry on port `5112`, so it can coexist with the main Fundament cluster without conflicts.

## Quick Start

```shell
just cluster-create   # Create K3D cluster + registry (~10s)
just dev              # Build + deploy plugin-controller with file watching

# In another terminal:
just plugin-install cert-manager   # Build plugin, push to registry, apply CR
just plugin-status                 # Check PluginInstallation status
just logs                          # Watch controller logs

# Verify cert-manager actually works:
just cert-manager test             # Creates a self-signed ClusterIssuer + Certificate
just cert-manager test-cleanup     # Remove test resources

# Cleanup:
just plugin-uninstall cert-manager
just cluster-delete
```

All commands are defined in the `Justfile`. Run `just --list` to see available commands.
