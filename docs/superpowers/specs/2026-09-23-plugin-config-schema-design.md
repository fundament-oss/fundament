# PluginDefinition configSchema — Design

Date: 2026-09-23
Status: approved for implementation planning

## Problem

`PluginInstallation.spec.config` is a `map[string]string` that plugin-controller
materialises into `FUNP_<KEY>` env vars (`plugin-controller/pkg/controller/resources.go`,
`mutateDeployment`). PluginDefinition declares no schema for those keys, so the
Console has nothing to render an install form from and no way to validate input.
Every plugin needing non-default config must be installed with `kubectl apply`,
bypassing the appstore flow. ceph-rook makes this acute: a single-node or arm64
cluster needs several keys set (`DEV_LOOP_DEVICES`, `MON_COUNT`, `MGR_COUNT`,
`ALLOW_MULTIPLE_PER_NODE`, sometimes `ALLOW_UNSUPPORTED_CEPH`) or the install
silently fails to reach quorum.

## Corrected premises (from codebase exploration)

- **PluginDefinition is not a CRD.** It is a plain YAML manifest parsed by
  `plugin-sdk/pluginruntime/definition.go` (`PluginSpec`), stored as raw `bytea`
  in `appstore.plugin_definitions`, and hashed byte-for-byte (`HashManifest` =
  SHA-256 of manifest bytes). `configSchema` therefore rides inside the manifest:
  the consent-hash property is automatic, and no CRD change, deepcopy change,
  hashing change, or DB migration is needed.
- The YAML decoder uses `KnownFields(true)`: the Go struct field must exist
  before any definition.yaml uses the key, in every consumer.
- Published versions are create-only: adding `configSchema` to ceph-rook requires
  a `metadata.version` bump.
- **There is no server-side install handler.** The Console writes the
  PluginInstallation CR directly through kube-api-proxy
  (`console-frontend/src/app/plugin-installation/plugin-installation.service.ts`)
  and today never sends `spec.config`. Install-time validation must live in the
  Console form and/or plugin-controller — there is no organization-api install
  endpoint to extend.
- Publish-time validation *does* have a single natural home: `parseDefinition`,
  called by every publish path (marketplace-registry-api, legacy
  organization-api `PutPluginDefinition`, functl).
- ceph-rook has eight config keys in `plugins/storage/ceph-rook/config.go`, not
  five: the five above plus `ROOK_NAMESPACE`, `CLUSTER_NAMESPACE`, `CEPH_IMAGE`.

## Decisions (made with the user)

1. **Enforcement: Console + controller.** The Console form validates before
   POST; plugin-controller re-validates against the pinned, hash-verified
   definition at reconcile and fails the installation status on violation. This
   also covers kubectl/Terraform installs.
2. **Undeclared keys: reject** — when (and only when) the definition declares a
   `configSchema`. A definition with no schema keeps today's accept-anything
   behavior, so all existing plugins and installs are untouched.
3. **Schema shape: flat key list**, not an OpenAPI object. List order is form
   order; names map 1:1 to `FUNP_*` keys; an `advanced` flag fits naturally.

## Design

### 1. Schema type + publish-time validation (plugin-sdk)

New struct in `plugin-sdk/pluginruntime/definition.go`:

```go
type ConfigSchemaEntry struct {
    Name        string   `yaml:"name"`
    Type        string   `yaml:"type"`        // string | int | bool | enum
    Default     string   `yaml:"default"`     // optional; always a string
    Description string   `yaml:"description"` // optional
    Required    bool     `yaml:"required"`    // optional
    Values      []string `yaml:"values"`      // enum only
    Advanced    bool     `yaml:"advanced"`    // optional; collapsed by default in UI
}
```

`PluginSpec` gains `ConfigSchema []ConfigSchemaEntry`. Values and defaults are
strings because `spec.config` stays `map[string]string`; typing lives in
validation and the form, not the representation.

`parseDefinition` rules (each mirrored by a `TestParseDefinition_Rejects*` test,
following the `validImagePullPolicies` pattern):

- `name` matches `^[A-Z][A-Z0-9_]*$` and is unique within the schema.
- `type` is one of `string`, `int`, `bool`, `enum`. Exhaustive switch; panic in
  default per project convention.
- `enum` requires non-empty `values`; other types must not set `values`.
- `default`, when set, must validate against the declared type
  (an `int` default of `"three"` is a publish error; an enum default must be one
  of `values`).
- `required` and `default` are mutually exclusive: required means "the admin
  must choose"; a default would make it silently optional.

Because all publish paths call `ParseDefinition`, a malformed schema can never
be published.

A shared helper (exported from `pluginruntime`) validates a
`map[string]string` config against a `[]ConfigSchemaEntry` — undeclared key,
type/enum mismatch, missing required key — so publish-side and controller-side
logic cannot drift.

### 2. Install-time enforcement (plugin-controller)

At reconcile, where `mutateDeployment` already holds `cr.Spec.Config` and the
verified `def`: if `def.Spec.ConfigSchema` is non-empty, validate the config
with the shared helper before building env vars. Violations flow through the
existing fail-closed status mechanism (same path as a definition-hash mismatch)
with a message naming the offending key. `FUNP_*` injection itself is unchanged;
no schema means no validation (back-compat).

### 3. Surfacing the schema to the Console (catalog API)

Follow the derive-on-read pattern of
`marketplace-catalog-api/pkg/catalog/manifest.go`: parse the pinned manifest and
expose `repeated ConfigSchemaEntry config_schema` on the `GetPluginDefinition`
response in `catalog.proto`. No DB column, so the schema can never drift from
the hash: what the admin sees is provably what was hashed into the consent
record. The legacy `organization-api/pkg/proto/v1/plugin.proto` message mirror
does **not** get the field (deprecated path, no consumer).

Console generated client regenerated with `buf generate` (per project rule:
`just generate` / `go generate ./...` at the repo level).

### 4. Console install form

`install-plugin-modal` behavior:

- Selected version has an empty/absent schema → unchanged instant install.
- Non-empty schema → the sheet shows a reactive form (`FormBuilder` +
  `Validators`, `nldd-form-field` markup as in `new-namespace-sheet`):
  - `string`/`int` → text input (`int` gets a pattern validator),
  - `bool` → toggle,
  - `enum` → select/menu of `values`,
  - defaults pre-filled, `required` keys marked (`.form-field-header` styles),
  - `advanced` entries in a collapsed section, list order preserved.
- On submit, only keys whose value differs from the schema default (plus all
  `required` keys) are written to `spec.config`. The plugin binary's own
  `envDefault` tags remain the authoritative fallback; CRs stay minimal and a
  future definition version can change defaults without stranding old values.

Plumbing: `installPlugin(...)` gains a `config` parameter written into the CR
spec; the four call sites (`plugins.component.ts` ×2, `plugin-details.component.ts`
×2, `cluster-plugins.component.ts`) and the demo fake
(`fake-plugin-installation.service.ts`) updated; `PluginInstallationItem.spec`
in `plugin-resources/types.ts` gains `config`.

Typecheck via `ng build` / `tsconfig.app.json` (root tsconfig falsely passes).

### 5. ceph-rook adoption

`plugins/storage/ceph-rook/definition.yaml` declares all eight keys from
`config.go`; `ROOK_NAMESPACE`, `CLUSTER_NAMESPACE`, `CEPH_IMAGE` are marked
`advanced` (as are `ALLOW_UNSUPPORTED_CEPH` and `DEV_LOOP_DEVICES`). Defaults
are copied from the `envDefault` tags, which stay authoritative in code; a short
comment in `config.go` notes the manifest schema must be kept in sync — accepted
cost of the consent-hash property. `metadata.version` is bumped (create-only
versions).

### 6. Docs and dev-workflow cleanup

- `docs/funs/FUN-11.adoc`: add `configSchema` to the definition-structure
  section; update the `config: {}` install example and the install-lifecycle
  step about `FUNP_*` injection to mention validation.
- `docs/developer/plugins/writing-a-plugin.md`: new section on declaring config.
- `docs/developer/plugins/ceph-rook-runbook.md`: add the console install flow;
  keep the `kubectl apply` path documented until this ships in a release (the
  ticket's prerequisite note).
- `plugins/storage/ceph-rook/test-resources.yaml`: the commented dev-config
  block becomes redundant.
- Correction to the original ticket: `cluster-create-storage` does **not**
  become redundant — it mounts `/dev` and udev into k3d for loop devices, which
  config UI cannot replace. Only the manual `kubectl apply` install step does.
- Docs are edited under `docs/`; `docs-frontend` copies are synced, never edited.

## Out of scope

- Secret-valued config: no `sensitive` flag; `spec.config` is world-readable to
  anyone with `get plugininstallations`. Needs a separate secretRef design.
- Post-install reconfiguration (editing `spec.config` on a live installation).
- Admission webhook / CEL enforcement at the kube-api level.
- Terraform provider changes: it already passes arbitrary config and now gets
  controller-side validation for free.
- Legacy organization-api proto mirror.

## Testing

- `plugin-sdk/pluginruntime/definition_test.go`: one rejection test per publish
  rule; acceptance test for a full valid schema; shared-helper table tests for
  config validation (undeclared / type mismatch / enum mismatch / missing
  required / no-schema passthrough).
- `plugin-controller/pkg/controller`: reconcile tests asserting status error on
  invalid config and unchanged env injection on valid config.
- `marketplace-catalog-api`: derive test for `config_schema` in
  `GetPluginDefinition`.
- Console: component spec for the form (render per type, advanced collapse,
  diff-from-default submission); `ng build` for typecheck.
- Go tests use testify `assert`/`require` per project convention.

## Sequencing hazards

1. `KnownFields(true)`: land the Go struct field before any definition.yaml
   uses `configSchema`.
2. ceph-rook needs a `metadata.version` bump to publish the schema-bearing
   definition (create-only versions).
