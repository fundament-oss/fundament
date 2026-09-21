# Plugins

First-party Fundament plugins, built in-tree by the core team. Plugins in their own project start at [Writing a plugin](../docs/developer/plugins/writing-a-plugin.md).

## Where to look

| For | Go to |
|---|---|
| Running a plugin in the local sandbox, step by step | [Testing plugins locally](../docs/developer/plugins/testing-plugins-locally.md) |
| One plugin's flow, config and recipes | its README, see [First-party plugins](#first-party-plugins) |
| Recipes | `just plugins`, and `just plugins <module>` per plugin |
| Runtime, controller, PluginInstallation, RBAC model | [Plugins overview](../docs/developer/plugins/index.md) |
| Console pages | [Custom UI](../docs/developer/plugins/custom-ui.md), [Console integration](../docs/developer/plugins/console-integration.md) |

## Layout

```
plugins/
├── mod.just                  just plugins <recipe>
├── sandbox/                  the k3d-fundament-plugin cluster: k3d config, skaffold, chart values
└── <path>/                   one plugin; gateway-api/ and storage/ group related plugins
    ├── main.go, plugin.go    package main in the root Go module
    ├── definition.yaml       PluginDefinition
    ├── Dockerfile            built with the repository root as context
    ├── console/              console pages (console-ui/: Vite app built into console/)
    ├── Justfile              test and plugin-specific recipes, a module of mod.just
    ├── test-resources.yaml   sample resources for the test recipe
    └── README.md             the plugin's flow, config and recipes
```

## First-party plugins

| Plugin | `<path>` | Module |
|---|---|---|
| [Cert Manager](cert-manager/README.md) | `cert-manager` | `cert-manager` |
| [External DNS](external-dns/README.md) | `external-dns` | `external-dns` |
| [Gateway API (Envoy Gateway)](gateway-api/envoy-gateway/README.md) | `gateway-api/envoy-gateway` | `envoy-gateway` |
| [Gateway API (Istio)](gateway-api/istio/README.md) | `gateway-api/istio` | none |
| [OpenFSC](openfsc/README.md) | `openfsc` | `openfsc` |
| [Ceph Storage (Rook)](../docs/developer/plugins/example-ceph-rook.md) | `storage/ceph-rook` | `ceph-rook` |

## Images

CI builds every plugin here on every PR and master build, and pushes
`ghcr.io/fundament-oss/fundament/<metadata.name>-plugin:<sha>-master-<run>` (`linux/amd64`)
from master only.

`just plugins publish` builds its own image, for the local sandbox registry.

## Adding a first-party plugin

1. A directory under `plugins/` with the files from [Layout](#layout).
2. `permissions.rbac` covers everything the plugin creates outside its own namespace, including Helm's release storage (Secrets): [RBAC model](../docs/developer/plugins/index.md#rbac-model).
3. `mod <module> '<path>'` in `mod.just`.
4. A `build-plugins` matrix entry in `.github/workflows/build.yml`.
5. A README with its flow.
6. The first publish with `--create`, or a listing in `db/seed/0101-appstore-catalog.sql`.
