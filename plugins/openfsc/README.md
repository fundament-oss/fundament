# OpenFSC plugin

Thin installer around the standalone [openfsc-operator](../../openfsc-operator)
plus a Console UI for FSCInstallations. The plugin:

- installs the prerequisites the operator preflights but never installs itself:
  cert-manager and CloudNativePG,
- helm-installs the operator chart vendored into the plugin image at
  `/operator-chart` (the FSCInstallation CRD included), and
- serves the console pages — **list, detail, and create** — for FSCInstallations,
  each with per-gateway status.

The plugin's installer never *auto-creates* FSCInstallations: teams own them.
The Console now offers a **create form** (alongside list/detail), so a user can
declare an FSCInstallation through the UI under their own RBAC — in addition to
kubectl/GitOps (see
`openfsc-operator/config/samples/openfsc_v1_fscinstallation.yaml`). Either way the
CR lands in the team's namespace and the operator reconciles it; all
reconciliation lives in the operator, so installations keep running when the
plugin pod is down. The plugin status summarizes the cluster
(`N installations: A active, P pending, E error`); uninstalling refuses while
FSCInstallations exist, because removing the operator first would strand their
finalizers.

## Console UI

The console pages are a **Vite app** (vanilla TS) in `console-ui/`, built into the
plugin binary (`go:embed console/*`) and served same-origin from `/console/`:

- `fscinstallations-{list,detail,create}.html` — one entry point per view; the
  filenames must match `definition.yaml`'s `customComponents`.
- `src/{list,detail,create}.ts` — one module per view; `src/shared.ts` holds the
  SDK loader + helpers; `src/{sdk,types,nldd-design-system}.ts` are types only.
- The create form writes the FSCInstallation CR through the host SDK k8s broker
  (`window.fundament.k8s.create`, see `src/sdk.ts`), so it runs under the user's
  RBAC — `definition.yaml`'s `allowedResources` grants the `create` verb.

### NLDD Design System

The views are built with the **NLDD Design System** (`@nldd/design-system`) — the
same one the host Console renders — using its `<nldd-*>` Lit web components
(`<nldd-button>`, `<nldd-text-field>`, `<nldd-dropdown>`, `<nldd-checkbox-field>`,
…). Its **runtime is not bundled** into the plugin: the views call `loadNlddDesignSystem()`
(`src/shared.ts`) to pull the shared, host-pinned bundle from the Console origin at
`/plugin-ui/nldd-design-system.{js,css}`, so every plugin uses one version that can't drift from
the host. `loadNlddDesignSystem()` also mirrors the host light/dark theme onto
`<html data-scheme>` so the design tokens follow the Console theme.

`@nldd/design-system` *is* a **devDependency** — for types only. `src/nldd-design-system.ts`
re-exports the real component types via `import type`, which is erased at build, so
the bundle stays byte-for-byte free of NLDD Design System code while `tsc` still
checks every property the views read. The pin must equal console-frontend's (a unit
test enforces this); bump the two together.

- **Browse components in the storybook:** <https://minbzk.github.io/storybook/> —
  the canonical reference for the `<nldd-*>` components and the
  [icon gallery](https://minbzk.github.io/storybook/?path=/story/components-content-icon--icon-gallery)
  (icons are kebab-case identifiers, e.g. `info-circle`). Styling conventions live
  in [`FUN-10`](../../docs/funs/FUN-10.adoc).
- **How the design system reaches the sandboxed iframe** (the shared `/plugin-ui/`
  channel, why the NLDD Design System is externalized, theming, build wiring, the generated-artifact
  policy) is recorded in [`FUN-18`](../../docs/funs/FUN-18.adoc). OpenFSC is the
  reference implementation — `fscinstallations-create.html` shows the per-component
  authoring rules (manual validation, `nldd-dropdown` wrapping a native
  `<select>`, `nldd-checkbox-field` reads, submit via click handler).

## Configuration

`PluginInstallation.spec.config` entries surface as `FUNP_<KEY>` env vars:

| Env var | Default | Purpose |
| --- | --- | --- |
| `FUNP_OPERATOR_IMAGE` | `ghcr.io/fundament-oss/fundament/openfsc-operator:latest` | openfsc-operator image deployed by the vendored chart |

## Sandbox flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `just plugins openfsc operator-push`: builds `openfsc-operator/` and prints `OPERATOR_IMAGE: <ref>`. The published operator image is amd64 only.
2. `PLUGIN_REGISTRY=localhost:5112 just plugins publish openfsc`
3. Install `system--openfsc` (step 5) with `spec.config` `OPERATOR_IMAGE: <ref>`. The plugin deploys the operator only when its release is absent: a new ref needs uninstall and install.
4. `just plugins openfsc test`: sample FSCInstallation `demo` in `fsc-demo` reaches Active; the one-per-namespace guard holds.
5. `just plugins openfsc console-preview` or `console-dev`: the console pages against the sandbox, without publishing. Both need an FSCInstallation.
6. `just plugins openfsc test-cleanup`

The console create form needs an existing namespace in the sandbox: local projects have no namespaces, so the form asks for a name and the console does not create it.

`console-preview` builds the shared `/plugin-ui/` bundle
(`plugin-sdk.{js,css}` + `nldd.{js,css}`, the same assets prod serves) and the
plugin's Vite app, then serves the built `console/` with a kubectl-backed stand-in
for the plugin SDK (`scripts/console-preview/`), so the templates render the same
CRs the real console would — no console host needed. `console-dev` runs that same
backend behind the Vite dev server (which proxies `/api/*` and `/plugin-ui/*` to
it) for HMR against the live cluster.
