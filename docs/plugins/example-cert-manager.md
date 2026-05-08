---
title: "Example: cert-manager"
sidebar:
  order: 4
---

The cert-manager plugin is a reference implementation that installs and manages cert-manager.

## What it does

1. **Start**: Checks if cert-manager is already installed, then runs `helm upgrade --install cert-manager` from the Jetstack Helm repo
2. **Verify**: Checks that all cert-manager CRDs exist (`certificates`, `issuers`, `clusterissuers`, `certificaterequests`)
3. **Reconcile**: Periodically re-checks CRD availability, reports degraded if missing
4. **Console**: Serves list and detail HTML for each managed CRD from `/console/`, talking to the host through the plugin SDK's `fundament.k8s.*` broker

## File structure

```
plugins/cert-manager/
├── main.go             # Entry point: load definition, call pluginruntime.Run()
├── plugin.go           # Plugin implementation (Start, Install, Reconcile, etc.)
├── console.go          # Embeds console/ directory as http.FileSystem
├── definition.yaml     # Metadata, permissions, menu, customComponents, allowedResources
├── console/
│   ├── _shared.js                  # SDK loader + shared helpers (escapeHtml, formatAge, …)
│   ├── certificates-list.html
│   ├── certificates-detail.html
│   ├── certificaterequests-list.html
│   ├── certificaterequests-detail.html
│   ├── issuers-list.html
│   ├── issuers-detail.html
│   ├── clusterissuers-list.html
│   └── clusterissuers-detail.html
├── plugin_test.go      # Unit tests
└── Dockerfile          # Multi-stage build (Go build + alpine with helm)
```

Each `<resource>-list.html` and `<resource>-detail.html` corresponds to an
entry in `spec.customComponents` in `definition.yaml`. `_shared.js`
contains the SDK loader (`loadSdk()` reads `?host=` from the query string
and injects the `plugin-sdk.js` / `.css` tags) plus the rendering
helpers — copy it as a starting point for new plugins. See
[Custom UI](custom-ui) for the pattern and
[Console integration](console-integration) for the architecture.

## Why it needs `cluster-admin`

cert-manager installs cluster-scoped resources that require broad permissions:
- CRDs (`certificates.cert-manager.io`, etc.)
- ClusterRoles and ClusterRoleBindings
- ValidatingWebhookConfigurations / MutatingWebhookConfigurations
- Resources across multiple namespaces

The default namespace-admin RoleBinding only covers the plugin's own namespace. The `clusterRoles: [cluster-admin]` field in the PluginInstallation grants the additional access.

```yaml
# plugins/cert-manager/install.yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: cert-manager-test
spec:
  image: localhost:5111/cert-manager-plugin:latest
  pluginName: cert-manager-test
  clusterRoles:
    - cluster-admin
```

## Plugin lifecycle

```
  Container starts
       │
       ▼
  pluginruntime.Run()
       │
       ├─ HTTP server on :8080
       │
       ▼
  Start()
       │
       ├─ Check if cert-manager is already installed
       ├─ ReportStatus("installing", "checking/installing cert-manager")
       ├─ helm upgrade --install cert-manager jetstack/cert-manager
       ├─ Create k8s client
       ├─ crd.VerifyAll([certificates, certificaterequests, issuers, clusterissuers])
       ├─ ReportReady()
       ├─ ReportStatus("running", "cert-manager is running")
       └─ Block until SIGTERM
              │
              ▼
  Reconcile() (every 5 minutes)
       │
       ├─ crd.VerifyAll(...)
       ├─ If OK:  ReportStatus("running")
       └─ If not: ReportStatus("degraded")
```
