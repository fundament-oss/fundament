# Plugin Sandbox

These are Fundament's first-party plugins, built in-tree. A **new** plugin should
be scaffolded as a standalone project instead:

```shell
functl plugin create my-plugin
```

See [docs/developer/plugins/writing-a-plugin.md](../docs/developer/plugins/writing-a-plugin.md).

A self-contained development environment lives in `sandbox/`. It creates an isolated K3D cluster with only the plugin controller -- no database, auth services, or other Fundament components needed. The sandbox cluster (`fundament-plugin`) uses a separate registry on port `5112`, so it can coexist with the main Fundament cluster without conflicts.

## Quick Start

```shell
just cluster-create   # Create K3D cluster + registry (~10s)
just dev              # Build + deploy plugin-controller with file watching
just deploy           # One-time build without file watching

# In another terminal. `just plugins install` no longer exists: publish the
# definition, then create a PluginInstallation pinning the published
# pluginVersion/definitionHash (see the comments in plugins/mod.just).
just plugins publish cert-manager   # Build + push the image, publish the definition
just plugins status                 # Check PluginInstallation status
just logs                          # Watch controller logs

# Verify cert-manager actually works:
just cert-manager test             # Creates a self-signed ClusterIssuer + Certificate
just cert-manager test-cleanup     # Remove test resources

# Install and verify external-dns:
just plugins publish external-dns   # Build + push the image, publish the definition
just external-dns test             # Creates a DNSEndpoint resource
just external-dns test-cleanup     # Remove test resources

# Install and verify OpenFSC (see plugins/openfsc/README.md):
just openfsc operator-push         # Build the openfsc-operator image for the sandbox
just plugins publish openfsc        # Build + push the image, publish the definition
just openfsc test                  # Sample FSCInstallation reaches Active
just openfsc test-cleanup          # Remove the sample installation

# Cleanup:
just plugins uninstall cert-manager
just plugins uninstall external-dns
just cluster-delete
```

All commands are defined in the `Justfile`. Run `just --list` to see available commands.
