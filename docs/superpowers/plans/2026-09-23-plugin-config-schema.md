# PluginDefinition configSchema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a PluginDefinition declare its install-time config keys so the Console renders a validated install form and plugin-controller rejects config that violates the declared schema.

**Architecture:** The schema is a flat key list inside the plugin manifest (`spec.configSchema`), so it is covered by the byte-for-byte definition hash (the consent record) with no hashing, CRD, or DB changes. Publish-time validation lives in the shared parser `pluginruntime.parseDefinition`; install-time validation is a shared `pluginruntime.ValidateConfig` helper enforced by plugin-controller at reconcile and mirrored by the Console form. The Console gets the schema from marketplace-catalog-api's `GetPluginDefinition`, derived on read from the pinned manifest.

**Tech Stack:** Go (yaml.v3, testify, controller-runtime fake client), protobuf editions 2023 + buf + connectrpc, Angular 20 (signals, reactive forms, nldd design system), bun.

**Spec:** `docs/superpowers/specs/2026-09-23-plugin-config-schema-design.md`

## Global Constraints

- **NO per-task commits.** The user's standing policy: implement all tasks, then ONE commit at the end, only after the user explicitly approves. Never add a Co-Authored-By trailer. Skip every "commit" step convention from the executing skill.
- Run all commands from the worktree root: `/Users/chiel/projects/github.com/fundament-oss/fundament/.claude/worktrees/plugin-config-schema`.
- Go tests use `github.com/stretchr/testify/assert` and `require` — never hand-rolled `if got != want`.
- Panic in switch default when all enum cases should be exhaustively handled.
- Regenerate protobuf with `go generate ./marketplace-catalog-api/...` (Go) and `cd console-frontend && buf generate` (TS). Never hand-edit generated files.
- Frontend: `bun` (never npm). Typecheck via `cd console-frontend && bun run build` — the root tsconfig falsely passes, only `ng build` catches errors. Check `console-frontend/src/styles.css` for predefined classes before adding new ones (`.form-field-header`, `__label`, `__optional`, `__supporting-label` exist at lines 172–198).
- Angular: standalone components (no `standalone: true` flag), `input()`/`output()`/`inject()` functions, signals, `ChangeDetectionStrategy.OnPush`, native `@if`/`@for`, reactive forms.
- Docs: edit under `docs/` only; `docs-frontend/` copies are synced by script, never edited.
- Do NOT touch `db/migrations/`, `db/fundament.sql`, or generate any migration — no DB change is needed or allowed here.
- The manifest YAML decoder uses `KnownFields(true)`: Task 1 (the Go field) must land before Task 7 (the ceph-rook manifest using it). Task order already guarantees this — do not reorder 7 before 1.
- If `go test ./marketplace-catalog-api/...` fails with embedded-postgres `pg_ctl: could not start server`, macOS SysV shared memory is exhausted: `sudo sysctl -w kern.sysv.shmall=65536 kern.sysv.shmmax=268435456` (no reboot needed).

## Review Focus

The five spec-implied failure modes most likely to bite, each pinned by a test in the owning task:

1. **Empty-string value for an int/bool key** (`config: {MON_COUNT: ""}`) — present-but-empty must be rejected, not treated as absent; a user who blanks a field and force-applies must get a clear error. → Task 2 test `TestValidateConfig` case "empty int value rejected".
2. **Console form: clearing an optional field must omit the key, not submit ""** — otherwise the controller rejects the empty string per (1) and the UI created the very error it exists to prevent. → Task 6 test "clearing an optional field omits the key".
3. **Retry of a failed install must preserve the existing spec.config** — retry deletes and re-creates the CR; dropping config would turn a retried ceph-rook install into a differently-failing one. → Task 5 test "retry re-creates the installation with its previous config".
4. **Install with no config against a schema-bearing definition must succeed** — all-default installs (cluster-plugins bulk path, old console, kubectl without config) send no config; only *required* keys may fail this. → Task 3 test `TestReconcileChildren_AcceptsEmptyConfigWithSchema`.
5. **Pre-schema manifests must keep today's behavior end to end** — catalog returns an empty schema, the modal installs instantly with no form, the controller validates nothing. → Task 4 test `TestGetPluginDefinitionNoConfigSchema`, Task 6 test "empty schema installs immediately".

---

### Task 1: plugin-sdk — ConfigSchemaEntry type and publish-time validation

**Files:**
- Modify: `plugin-sdk/pluginruntime/definition.go` (PluginSpec at 46–60, parseDefinition at 184–210)
- Test: `plugin-sdk/pluginruntime/definition_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `type ConfigSchemaEntry struct { Name, Type, Default, Description string; Required bool; Values []string; Advanced bool }` (yaml tags `name`, `type`, `default`, `description`, `required`, `values`, `advanced`); `PluginSpec.ConfigSchema []ConfigSchemaEntry` (yaml tag `configSchema`); unexported `validateConfigValue(value string, entry ConfigSchemaEntry) error` (Task 2 reuses it). Valid `Type` strings: `"string"`, `"int"`, `"bool"`, `"enum"`.

- [ ] **Step 1: Write the failing tests**

Append to `plugin-sdk/pluginruntime/definition_test.go`. The envelope of each manifest mirrors the existing `TestParseDefinition_Rejects*` tests; `ParseSourceDefinition` is used so no image is needed:

```go
// configSchemaManifest wraps a configSchema block in a minimal valid source
// definition, so each case below states only the schema under test.
func configSchemaManifest(schema string) []byte {
	return []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
  configSchema:
` + schema)
}

func TestParseSourceDefinition_AcceptsValidConfigSchema(t *testing.T) {
	def, err := ParseSourceDefinition(configSchemaManifest(`    - name: MON_COUNT
      type: int
      default: "3"
      description: Number of Ceph monitors
    - name: ALLOW_MULTIPLE_PER_NODE
      type: bool
      default: "false"
      advanced: true
    - name: FAILURE_DOMAIN
      type: enum
      values: [host, osd]
      required: true
`))
	require.NoError(t, err)
	require.Len(t, def.Spec.ConfigSchema, 3)
	assert.Equal(t, "MON_COUNT", def.Spec.ConfigSchema[0].Name)
	assert.Equal(t, "int", def.Spec.ConfigSchema[0].Type)
	assert.Equal(t, "3", def.Spec.ConfigSchema[0].Default)
	assert.True(t, def.Spec.ConfigSchema[1].Advanced)
	assert.Equal(t, []string{"host", "osd"}, def.Spec.ConfigSchema[2].Values)
	assert.True(t, def.Spec.ConfigSchema[2].Required)
}

func TestParseSourceDefinition_RejectsBadConfigSchema(t *testing.T) {
	for name, tc := range map[string]struct {
		schema  string
		wantErr string
	}{
		"lowercase key name": {
			schema:  "    - name: mon_count\n      type: int\n",
			wantErr: "mon_count",
		},
		"duplicate key": {
			schema:  "    - name: MON_COUNT\n      type: int\n    - name: MON_COUNT\n      type: int\n",
			wantErr: "twice",
		},
		"unknown type": {
			schema:  "    - name: MON_COUNT\n      type: integer\n",
			wantErr: "integer",
		},
		"enum without values": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n",
			wantErr: "no values",
		},
		"values on non-enum": {
			schema:  "    - name: MON_COUNT\n      type: int\n      values: [\"1\", \"3\"]\n",
			wantErr: "not an enum",
		},
		"non-integer int default": {
			schema:  "    - name: MON_COUNT\n      type: int\n      default: three\n",
			wantErr: "not an integer",
		},
		"non-boolean bool default": {
			schema:  "    - name: DEV_LOOP_DEVICES\n      type: bool\n      default: yes\n",
			wantErr: "not a boolean",
		},
		"enum default outside values": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n      values: [host, osd]\n      default: rack\n",
			wantErr: "rack",
		},
		"required with default": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n      values: [host, osd]\n      default: host\n      required: true\n",
			wantErr: "mutually exclusive",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSourceDefinition(configSchemaManifest(tc.schema))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./plugin-sdk/pluginruntime/ -run 'TestParseSourceDefinition_AcceptsValidConfigSchema|TestParseSourceDefinition_RejectsBadConfigSchema' -v`
Expected: FAIL — the accept test fails on `KnownFields(true)` rejecting the unknown `configSchema` key; reject cases fail because no error contains the expected message.

- [ ] **Step 3: Implement the type and validation**

In `plugin-sdk/pluginruntime/definition.go`:

Add `"slices"` and `"strconv"` to the imports.

After `validImagePullPolicies` (line 34), add:

```go
// configKeyNameRegex constrains configSchema key names to what survives the
// FUNP_<NAME> env-var injection unmangled: uppercase, digits, underscores,
// starting with a letter.
var configKeyNameRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// validConfigTypes is the set of types a configSchema entry may declare.
// Values stay strings in spec.config; the type drives validation and the
// console's input widget, not the representation.
var validConfigTypes = map[string]bool{"string": true, "int": true, "bool": true, "enum": true}
```

In `PluginSpec` (after `AllowedResources`, before `Image`), add:

```go
	// ConfigSchema declares the install-time config keys the plugin accepts.
	// The console renders its install form from this list (in order) and
	// plugin-controller rejects a spec.config that violates it. An empty list
	// means the plugin declares no schema and any config is accepted.
	ConfigSchema []ConfigSchemaEntry `yaml:"configSchema"`
```

After the `AllowedResource` type, add:

```go
// ConfigSchemaEntry declares one install-time config key. Default and Values
// are strings because PluginInstallation.spec.config is map[string]string;
// typing lives in validation and the form, not the representation.
type ConfigSchemaEntry struct {
	// Name is the key in spec.config, injected as the FUNP_<Name> env var.
	Name string `yaml:"name"`
	// Type is one of "string", "int", "bool", "enum".
	Type        string `yaml:"type"`
	Default     string `yaml:"default"`
	Description string `yaml:"description"`
	// Required forces the admin to choose a value; mutually exclusive with
	// Default, which would make the choice silently optional.
	Required bool `yaml:"required"`
	// Values enumerates the allowed values; set exactly when Type is "enum".
	Values []string `yaml:"values"`
	// Advanced keys start collapsed in the console install form.
	Advanced bool `yaml:"advanced"`
}
```

Before the `PluginPhase` section, add the validators:

```go
// validateConfigSchema enforces the publish-time rules on a declared schema.
// It runs inside parseDefinition, so every publish path (marketplace-registry,
// legacy organization-api, functl) rejects a malformed schema identically.
func validateConfigSchema(schema []ConfigSchemaEntry) error {
	seen := make(map[string]bool, len(schema))
	for _, entry := range schema {
		if !configKeyNameRegex.MatchString(entry.Name) {
			return fmt.Errorf("configSchema key %q must match %s (it becomes the FUNP_ env var name)", entry.Name, configKeyNameRegex)
		}
		if seen[entry.Name] {
			return fmt.Errorf("configSchema declares key %q twice", entry.Name)
		}
		seen[entry.Name] = true
		if !validConfigTypes[entry.Type] {
			return fmt.Errorf("configSchema key %q has invalid type %q, expected one of \"string\", \"int\", \"bool\", \"enum\"", entry.Name, entry.Type)
		}
		if entry.Type == "enum" && len(entry.Values) == 0 {
			return fmt.Errorf("configSchema key %q is an enum but declares no values", entry.Name)
		}
		if entry.Type != "enum" && len(entry.Values) > 0 {
			return fmt.Errorf("configSchema key %q declares values but is not an enum", entry.Name)
		}
		if entry.Required && entry.Default != "" {
			return fmt.Errorf("configSchema key %q sets both required and a default; they are mutually exclusive (required means the admin must choose)", entry.Name)
		}
		if entry.Default != "" {
			if err := validateConfigValue(entry.Default, entry); err != nil {
				return fmt.Errorf("configSchema key %q default: %w", entry.Name, err)
			}
		}
	}
	return nil
}

// validateConfigValue checks one value against its declared entry. Bool uses
// strconv.ParseBool to match what caarlos0/env accepts in the plugin binary,
// so a value the schema passes never fails inside the pod.
func validateConfigValue(value string, entry ConfigSchemaEntry) error {
	switch entry.Type {
	case "string":
		return nil
	case "int":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%q is not an integer", value)
		}
		return nil
	case "bool":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%q is not a boolean", value)
		}
		return nil
	case "enum":
		if slices.Contains(entry.Values, value) {
			return nil
		}
		return fmt.Errorf("%q is not one of %v", value, entry.Values)
	default:
		// Unreachable: validateConfigSchema rejects unknown types before any
		// value is checked against them.
		panic(fmt.Sprintf("unhandled config type %q", entry.Type))
	}
}
```

In `parseDefinition`, after the `imagePullPolicy` check (line 206–208), add:

```go
	if err := validateConfigSchema(def.Spec.ConfigSchema); err != nil {
		return PluginDefinition{}, err
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugin-sdk/pluginruntime/ -v -run 'ConfigSchema'` then the full package `go test ./plugin-sdk/pluginruntime/`
Expected: PASS, including all pre-existing tests (no envelope behavior changed).

---

### Task 2: plugin-sdk — shared ValidateConfig helper

**Files:**
- Modify: `plugin-sdk/pluginruntime/definition.go`
- Test: `plugin-sdk/pluginruntime/definition_test.go`

**Interfaces:**
- Consumes: `validateConfigValue`, `ConfigSchemaEntry` (Task 1).
- Produces: `func ValidateConfig(config map[string]string, schema []ConfigSchemaEntry) error` — exported; Task 3 calls it from plugin-controller.

- [ ] **Step 1: Write the failing test**

Append to `definition_test.go`:

```go
func TestValidateConfig(t *testing.T) {
	schema := []ConfigSchemaEntry{
		{Name: "MON_COUNT", Type: "int", Default: "3"},
		{Name: "DEV_LOOP_DEVICES", Type: "bool", Default: "false"},
		{Name: "FAILURE_DOMAIN", Type: "enum", Values: []string{"host", "osd"}, Required: true},
		{Name: "CEPH_IMAGE", Type: "string"},
	}

	for name, tc := range map[string]struct {
		config  map[string]string
		schema  []ConfigSchemaEntry
		wantErr string // empty means valid
	}{
		"no schema accepts anything (pre-schema back-compat)": {
			config: map[string]string{"WHATEVER": "yes"}, schema: nil,
		},
		"valid config": {
			config: map[string]string{"MON_COUNT": "1", "FAILURE_DOMAIN": "osd"}, schema: schema,
		},
		"undeclared key rejected": {
			config:  map[string]string{"MON_CONUT": "1", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: `"MON_CONUT" is not declared`,
		},
		"non-integer int rejected": {
			config:  map[string]string{"MON_COUNT": "three", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not an integer",
		},
		"empty int value rejected": {
			// Present-but-empty is a value, not an omission: "" would crash the
			// plugin's own env parsing, so it must fail here first.
			config:  map[string]string{"MON_COUNT": "", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not an integer",
		},
		"bad bool rejected": {
			config:  map[string]string{"DEV_LOOP_DEVICES": "yes", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not a boolean",
		},
		"enum outside values rejected": {
			config:  map[string]string{"FAILURE_DOMAIN": "rack"},
			schema:  schema,
			wantErr: "not one of",
		},
		"missing required rejected": {
			config:  map[string]string{"MON_COUNT": "1"},
			schema:  schema,
			wantErr: `"FAILURE_DOMAIN" is required`,
		},
		"empty config with only-default schema is valid": {
			config: nil,
			schema: []ConfigSchemaEntry{{Name: "MON_COUNT", Type: "int", Default: "3"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateConfig(tc.config, tc.schema)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin-sdk/pluginruntime/ -run TestValidateConfig -v`
Expected: FAIL with "undefined: ValidateConfig".

- [ ] **Step 3: Implement ValidateConfig**

In `definition.go`, add `"maps"` to the imports, and after `validateConfigSchema` add:

```go
// ValidateConfig checks a PluginInstallation's spec.config against the
// definition's declared configSchema. An empty schema accepts anything —
// definitions published before configSchema existed keep their historical
// accept-anything behavior. A declared schema rejects undeclared keys, values
// that fail their declared type, and missing required keys. Shared between
// publish-side tooling and plugin-controller so the two can never drift.
func ValidateConfig(config map[string]string, schema []ConfigSchemaEntry) error {
	if len(schema) == 0 {
		return nil
	}
	entries := make(map[string]ConfigSchemaEntry, len(schema))
	for _, entry := range schema {
		entries[entry.Name] = entry
	}
	// Sorted so a multi-error config reports the same first error every
	// reconcile, keeping the status condition stable.
	for _, key := range slices.Sorted(maps.Keys(config)) {
		entry, declared := entries[key]
		if !declared {
			return fmt.Errorf("config key %q is not declared in the definition's configSchema", key)
		}
		if err := validateConfigValue(config[key], entry); err != nil {
			return fmt.Errorf("config key %q: %w", key, err)
		}
	}
	for _, entry := range schema {
		if entry.Required {
			if _, set := config[entry.Name]; !set {
				return fmt.Errorf("config key %q is required by the definition's configSchema but not set", entry.Name)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugin-sdk/pluginruntime/`
Expected: PASS.

---

### Task 3: plugin-controller — enforce ValidateConfig at reconcile

**Files:**
- Modify: `plugin-controller/pkg/controller/reconciler.go` (consts at 28–52, reconcileChildren at 266–278, setPluginScopeCondition at 374–396)
- Test: `plugin-controller/pkg/controller/reconciler_test.go`

**Interfaces:**
- Consumes: `pluginruntime.ValidateConfig` (Task 2).
- Produces: condition type `ConditionConfigValid = "ConfigValid"` on `PluginInstallation.Status.Conditions` with reasons `ConfigInvalid` / `Validated`; helper `setCondition(cr, condType string, status metav1.ConditionStatus, reason, message string)`.

- [ ] **Step 1: Write the failing tests**

Append to `reconciler_test.go`. `sampleManifest` (line 62) has no configSchema; add a schema-bearing variant next to it:

```go
// sampleManifestWithConfigSchema returns a valid PluginDefinition YAML that
// declares a configSchema, plus its sha256 pin.
func sampleManifestWithConfigSchema(t *testing.T) ([]byte, string) {
	t.Helper()
	manifest := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  permissions:
    rbac: []
  configSchema:
    - name: LOG_LEVEL
      type: enum
      values: [debug, info, warn]
      default: info
    - name: MON_COUNT
      type: int
      default: "3"
`)
	sum := sha256.Sum256(manifest)
	return manifest, "sha256:" + hex.EncodeToString(sum[:])
}

func TestReconcileChildren_RejectsConfigViolatingSchema(t *testing.T) {
	scheme := newTestScheme()
	manifest, pin := sampleManifestWithConfigSchema(t)

	cr := testCR()
	cr.Spec.DefinitionRef.DefinitionHash = pin
	cr.Spec.Config = map[string]string{"MON_CONUT": "1"} // typo: undeclared key

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &Reconciler{
		client: fakeClient, logger: slog.Default(),
		cfg:                 config.Config{FundamentClusterID: "test-cluster"},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MON_CONUT")

	// The failure is surfaced on the CR as a ConfigValid=False condition.
	var found bool
	for _, cond := range cr.Status.Conditions {
		if cond.Type == ConditionConfigValid {
			found = true
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			assert.Equal(t, "ConfigInvalid", cond.Reason)
			assert.Contains(t, cond.Message, "MON_CONUT")
		}
	}
	assert.True(t, found, "ConfigValid condition should be set")

	// Fail-closed: nothing was materialised, exactly like a hash mismatch.
	var deploy appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin", Namespace: pluginNamespace(cr.Name),
	}, &deploy)
	assert.True(t, apierrors.IsNotFound(err), "Deployment must not be created for invalid config")
}

func TestReconcileChildren_AcceptsEmptyConfigWithSchema(t *testing.T) {
	// The bulk-install path and kubectl installs send no config at all; a
	// schema whose keys all carry defaults must accept that (defaults are
	// applied by the plugin binary's own env parsing, not materialised here).
	scheme := newTestScheme()
	manifest, pin := sampleManifestWithConfigSchema(t)

	cr := testCR()
	cr.Spec.DefinitionRef.DefinitionHash = pin
	cr.Spec.Config = nil

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &Reconciler{
		client: fakeClient, logger: slog.Default(),
		cfg:                 config.Config{FundamentClusterID: "test-cluster"},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	var found bool
	for _, cond := range cr.Status.Conditions {
		if cond.Type == ConditionConfigValid {
			found = true
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
		}
	}
	assert.True(t, found, "ConfigValid condition should be set to True")
}

func TestReconcileChildren_ValidConfigStillInjected(t *testing.T) {
	// A schema does not change the injection contract: declared, valid keys
	// still become FUNP_* env vars verbatim.
	scheme := newTestScheme()
	manifest, pin := sampleManifestWithConfigSchema(t)

	cr := testCR()
	cr.Spec.DefinitionRef.DefinitionHash = pin
	cr.Spec.Config = map[string]string{"LOG_LEVEL": "debug", "MON_COUNT": "1"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &Reconciler{
		client: fakeClient, logger: slog.Default(),
		cfg:                 config.Config{FundamentClusterID: "test-cluster"},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
	}

	require.NoError(t, r.reconcileChildren(context.Background(), slog.Default(), cr))

	var deploy appsv1.Deployment
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin", Namespace: pluginNamespace(cr.Name),
	}, &deploy))
	env := deploy.Spec.Template.Spec.Containers[0].Env
	values := map[string]string{}
	for _, e := range env {
		values[e.Name] = e.Value
	}
	assert.Equal(t, "debug", values["FUNP_LOG_LEVEL"])
	assert.Equal(t, "1", values["FUNP_MON_COUNT"])
}
```

Note: `testCR()` sets `Config: {"LOG_LEVEL": "debug"}` and every existing reconcile test uses `sampleManifest` (no schema) — those must keep passing untouched, which is itself the no-schema back-compat check.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./plugin-controller/pkg/controller/ -run 'TestReconcileChildren_RejectsConfigViolatingSchema|TestReconcileChildren_AcceptsEmptyConfigWithSchema|TestReconcileChildren_ValidConfigStillInjected' -v`
Expected: FAIL with "undefined: ConditionConfigValid" (compile error).

- [ ] **Step 3: Implement enforcement**

In `reconciler.go`:

Add to the const block (after `ConditionPluginScopeReady`, line 38):

```go
	// ConditionConfigValid is the Status Condition surfaced on the CR reporting
	// whether spec.config conforms to the pinned definition's configSchema. A
	// definition with no schema always validates (pre-schema back-compat).
	ConditionConfigValid = "ConfigValid"
```

Generalise the condition helper — replace `setPluginScopeCondition` (lines 372–396) with:

```go
// setCondition upserts a Condition of the given type on the CR's status.
// Same-value updates preserve LastTransitionTime; changes reset it.
func setCondition(cr *pluginsv1.PluginInstallation, condType string, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	}
	for i, existing := range cr.Status.Conditions {
		if existing.Type != condType {
			continue
		}
		if existing.Status == status {
			cond.LastTransitionTime = existing.LastTransitionTime
		} else {
			cond.LastTransitionTime = metav1.Now()
		}
		cr.Status.Conditions[i] = cond
		return
	}
	cond.LastTransitionTime = metav1.Now()
	cr.Status.Conditions = append(cr.Status.Conditions, cond)
}

// setPluginScopeCondition upserts the PluginScopeReady Condition on the CR.
func setPluginScopeCondition(cr *pluginsv1.PluginInstallation, status metav1.ConditionStatus, reason, message string) {
	setCondition(cr, ConditionPluginScopeReady, status, reason, message)
}
```

In `reconcileChildren`, directly after the `fetchDefinition` error handling (after line 278), add:

```go
	// Enforce the definition's configSchema before materialising anything: an
	// invalid spec.config fails closed exactly like a hash mismatch. The
	// definition is the hash-verified one, so what is enforced is what the
	// admin consented to.
	if err := pluginruntime.ValidateConfig(cr.Spec.Config, def.Spec.ConfigSchema); err != nil {
		setCondition(cr, ConditionConfigValid, metav1.ConditionFalse, "ConfigInvalid", err.Error())
		return fmt.Errorf("validate config: %w", err)
	}
	setCondition(cr, ConditionConfigValid, metav1.ConditionTrue, "Validated",
		"spec.config conforms to the definition's configSchema")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugin-controller/...`
Expected: PASS, including all pre-existing tests.

---

### Task 4: catalog API — surface config_schema on GetPluginDefinition

**Files:**
- Modify: `marketplace-catalog-api/pkg/proto/catalog/v1/catalog.proto` (GetPluginDefinitionResponse at ~184)
- Modify: `marketplace-catalog-api/pkg/catalog/manifest.go`
- Modify: `marketplace-catalog-api/pkg/catalog/plugin.go` (GetPluginDefinition at 254–289)
- Test: `marketplace-catalog-api/pkg/catalog/catalog_test.go`
- Generated (via tooling, not by hand): `marketplace-catalog-api/pkg/proto/gen/...`, `console-frontend/src/generated/catalog/v1/catalog_pb.ts`

**Interfaces:**
- Consumes: `pluginruntime.ParseDefinition`, `ConfigSchemaEntry` (Task 1).
- Produces: proto message `catalog.v1.ConfigSchemaEntry` (fields `name`, `type` [enum `ConfigType`], `default_value`, `description`, `required`, `values`, `advanced`) and `repeated ConfigSchemaEntry config_schema = 30` on `GetPluginDefinitionResponse`; Go helper `configSchemaFromManifest(manifest []byte, logger *slog.Logger, ...) []*catalogv1.ConfigSchemaEntry`. The regenerated TS types `ConfigSchemaEntry` and `ConfigType` in `catalog_pb.ts` are what Task 6 renders from.

- [ ] **Step 1: Extend the proto**

In `catalog.proto`, after the `PluginRef` message, add (field numbers follow the file's gap-of-10 convention):

```proto
// One declared install-time config key, derived on read from the pinned
// manifest — never stored in a column, so it can never drift from the hash
// the install pins against (the schema shown is provably the one consented to).
message ConfigSchemaEntry {
  string name = 10;
  ConfigType type = 20;
  // The manifest's default, always a string: spec.config is map[string]string,
  // so typing drives the form widget and validation, not the representation.
  string default_value = 30;
  string description = 40;
  bool required = 50;
  // Allowed values when type is CONFIG_TYPE_ENUM; empty otherwise.
  repeated string values = 60;
  // Advanced keys start collapsed in the console install form.
  bool advanced = 70;
}

// Mirrors the manifest's configSchema type strings.
enum ConfigType {
  CONFIG_TYPE_UNSPECIFIED = 0;
  CONFIG_TYPE_STRING = 1;
  CONFIG_TYPE_INT = 2;
  CONFIG_TYPE_BOOL = 3;
  CONFIG_TYPE_ENUM = 4;
}
```

And in `GetPluginDefinitionResponse`:

```proto
  // Declared install-time config keys, in manifest order; empty when the
  // definition declares no configSchema (install proceeds with no form).
  repeated ConfigSchemaEntry config_schema = 30;
```

- [ ] **Step 2: Regenerate**

Run: `go generate ./marketplace-catalog-api/...` then `cd console-frontend && buf generate && cd ..`
Expected: `marketplace-catalog-api/pkg/proto/gen/catalog/v1/catalog.pb.go` gains `ConfigSchemaEntry`/`ConfigType`; `console-frontend/src/generated/catalog/v1/catalog_pb.ts` gains the same. `git status` shows only generated files plus the proto changed.

- [ ] **Step 3: Write the failing Go tests**

Append to `catalog_test.go` (fixture style copied from `TestGetPluginDefinitionReturnsManifestAndHash` at line 350; `testManifest` is the existing fixture around line 195):

```go
const testManifestWithConfigSchema = testManifest + `  configSchema:
    - name: MON_COUNT
      type: int
      default: "3"
      description: Number of Ceph monitors
    - name: FAILURE_DOMAIN
      type: enum
      values: [host, osd]
      required: true
      advanced: true
`

func TestGetPluginDefinitionDerivesConfigSchema(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{
		Name: "with-config-schema", Visibility: "public", Published: true,
		Manifest: []byte(testManifestWithConfigSchema),
	})

	resp, err := newServer(t, env).GetPluginDefinition(context.Background(),
		catalogv1.GetPluginDefinitionRequest_builder{PluginId: new(id.String()), Version: "1.0.0"}.Build())
	require.NoError(t, err)

	entries := resp.GetConfigSchema()
	require.Len(t, entries, 2)
	assert.Equal(t, "MON_COUNT", entries[0].GetName())
	assert.Equal(t, catalogv1.ConfigType_CONFIG_TYPE_INT, entries[0].GetType())
	assert.Equal(t, "3", entries[0].GetDefaultValue())
	assert.Equal(t, "Number of Ceph monitors", entries[0].GetDescription())
	assert.Equal(t, catalogv1.ConfigType_CONFIG_TYPE_ENUM, entries[1].GetType())
	assert.Equal(t, []string{"host", "osd"}, entries[1].GetValues())
	assert.True(t, entries[1].GetRequired())
	assert.True(t, entries[1].GetAdvanced())
}

func TestGetPluginDefinitionNoConfigSchema(t *testing.T) {
	// Pre-schema manifests must come back with an empty schema, keeping the
	// console's instant-install path.
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{
		Name: "no-config-schema", Visibility: "public", Published: true,
		Manifest: []byte(testManifest),
	})

	resp, err := newServer(t, env).GetPluginDefinition(context.Background(),
		catalogv1.GetPluginDefinitionRequest_builder{PluginId: new(id.String()), Version: "1.0.0"}.Build())
	require.NoError(t, err)
	assert.Empty(t, resp.GetConfigSchema())
}
```

Check how `testManifest` ends before concatenating — the `configSchema:` block must sit under `spec:` at the right indentation (the existing fixture's spec fields are 2-space indented; adjust the constant accordingly, and verify by running the test).

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./marketplace-catalog-api/pkg/catalog/ -run 'TestGetPluginDefinitionDerivesConfigSchema|TestGetPluginDefinitionNoConfigSchema' -v`
Expected: `TestGetPluginDefinitionDerivesConfigSchema` FAILS (empty schema returned); `NoConfigSchema` may already pass — that is fine.
(If embedded postgres fails to start, see Global Constraints for the macOS shm fix.)

- [ ] **Step 5: Implement derive-on-read**

In `manifest.go`, add:

```go
// configSchemaFromManifest derives the install-form schema from the pinned
// manifest, like capabilitiesAndPermissions above: never stored in a column,
// so it can never drift from the hash.
func configSchemaFromManifest(manifest []byte) ([]*catalogv1.ConfigSchemaEntry, error) {
	definition, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return nil, fmt.Errorf("parsing plugin definition: %w", err)
	}

	entries := make([]*catalogv1.ConfigSchemaEntry, 0, len(definition.Spec.ConfigSchema))
	for _, entry := range definition.Spec.ConfigSchema {
		entries = append(entries, catalogv1.ConfigSchemaEntry_builder{
			Name:         entry.Name,
			Type:         configTypeFromManifest(entry.Type),
			DefaultValue: entry.Default,
			Description:  entry.Description,
			Required:     entry.Required,
			Values:       entry.Values,
			Advanced:     entry.Advanced,
		}.Build())
	}
	return entries, nil
}

// configTypeFromManifest maps the manifest's type string to the proto enum.
func configTypeFromManifest(t string) catalogv1.ConfigType {
	switch t {
	case "string":
		return catalogv1.ConfigType_CONFIG_TYPE_STRING
	case "int":
		return catalogv1.ConfigType_CONFIG_TYPE_INT
	case "bool":
		return catalogv1.ConfigType_CONFIG_TYPE_BOOL
	case "enum":
		return catalogv1.ConfigType_CONFIG_TYPE_ENUM
	default:
		// Unreachable: parseDefinition already rejected unknown types.
		panic(fmt.Sprintf("unhandled config type %q", t))
	}
}
```

Add the `catalogv1` import (`github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1`) to `manifest.go`.

In `plugin.go`'s `GetPluginDefinition`, before the final return, derive the schema — degrading on a stale manifest exactly like `GetPlugin` does (lines 152–159):

```go
	configSchema, err := configSchemaFromManifest(row.Manifest)
	if err != nil {
		// Manifests outlive the binary that parses them; the manifest bytes and
		// hash are still what the caller needs, so degrade rather than fail.
		s.logger.WarnContext(ctx, "unparseable plugin manifest", "error", err)
		configSchema = nil
	}
```

and add `ConfigSchema: configSchema` to the `GetPluginDefinitionResponse_builder`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./marketplace-catalog-api/...`
Expected: PASS.

---

### Task 5: Console plumbing — config through the install request, retry preservation

**Files:**
- Modify: `console-frontend/src/app/plugin-installation/plugin-installation.service.ts` (installPlugin at 51–83)
- Modify: `console-frontend/src/app/demo/fake-plugin-installation.service.ts` (installPlugin at ~136)
- Modify: `console-frontend/src/app/plugin-resources/types.ts` (`PluginInstallationItem` at 126–141)
- Modify: `console-frontend/src/app/plugins/plugins.component.ts` (install at 794, retry at 865)
- Modify: `console-frontend/src/app/plugin-details/plugin-details.component.ts` (install at ~396, retry at ~445)
- Modify: `console-frontend/src/app/install-plugin-modal/install-plugin-modal.ts` (`InstallSelection` at 56–60 only — the form itself is Task 6)
- Test: `console-frontend/src/app/plugins/plugins.component.spec.ts`

**Interfaces:**
- Consumes: nothing from Task 4 yet (config values arrive in Task 6; this task threads the parameter).
- Produces: `installPlugin(clusterId, organizationName, pluginName, pluginVersion, definitionHash, config: Record<string, string> = {})`; `InstallSelection` gains `config: Record<string, string>`; `PluginInstallationItem['spec']` gains `config?: Record<string, string>`. Task 6 fills `config` from the form.

- [ ] **Step 1: Thread the parameter**

`plugin-installation.service.ts` — add the sixth parameter and include it in the CR only when non-empty (an empty `config: {}` on every CR would be noise):

```ts
  async installPlugin(
    clusterId: string,
    organizationName: string,
    pluginName: string,
    pluginVersion: string,
    definitionHash: string,
    config: Record<string, string> = {},
  ): Promise<void> {
```

and in the body spec:

```ts
        spec: {
          definitionRef: {
            organizationName,
            pluginName,
            pluginVersion,
            definitionHash,
          },
          ...(Object.keys(config).length > 0 ? { config } : {}),
        },
```

`fake-plugin-installation.service.ts` — mirror the signature (`config: Record<string, string> = {}`); the fake ignores the value but the signatures must stay in sync (its comment says so).

`types.ts` — in `PluginInstallationItem`'s `spec` type, alongside `definitionRef`, add:

```ts
    // Key-value config injected as FUNP_* env vars; absent when the install
    // uses only defaults.
    config?: Record<string, string>;
```

`install-plugin-modal.ts` — extend the selection type (Task 6 wires the value; default it here so the modal still compiles):

```ts
export interface InstallSelection {
  clusterIds: string[];
  version: string;
  hash: string;
  // Chosen config values; {} when the definition declares no schema or all
  // defaults were kept.
  config: Record<string, string>;
}
```

and in `onInstallOne`, emit `config: {}` for now (Task 6 replaces this).

- [ ] **Step 2: Pass config at the install call sites**

`plugins.component.ts` `onInstallOnClusters` (line 794) and `plugin-details.component.ts` `onInstallOnClusters` (~396): append `selection.config` as the sixth argument to `installPlugin(...)`. `cluster-plugins.component.ts` stays unchanged on purpose — bulk install uses defaults (see spec §4; a plugin with *required* schema keys will fail there with a clear ConfigValid status error, which is the accepted v1 behavior).

- [ ] **Step 3: Preserve config on retry**

Retry deletes and re-creates the CR, so the existing config must be read *before* the uninstall and passed to the re-create. In BOTH `plugins.component.ts` `onRetryInstall` (~853) and `plugin-details.component.ts` `onRetryInstall` (~435), before the `uninstallPlugin` call add:

```ts
      // The failed install may carry config; read it before deleting the CR so
      // the retry re-creates the installation as it was, not with defaults.
      const existing = await this.pluginInstallationService
        .getInstallation(clusterId, resourceName)
        .catch(() => null);
      const config = existing?.spec.config ?? {};
```

and pass `config` as the sixth argument to the `installPlugin` call that follows. (In `plugins.component.ts` the variable `resourceName` already exists in that scope; same in `plugin-details.component.ts`.)

- [ ] **Step 4: Write the retry-preservation test**

In `plugins.component.spec.ts`, follow the file's existing spy/stub pattern for `PluginInstallationService` (read the top of the spec to reuse its TestBed setup). The test in the established style:

```ts
  it('retry re-creates the installation with its previous config', async () => {
    // Arrange: getInstallation returns a CR that carries config.
    installationService.getInstallation.and.resolveTo({
      metadata: { name: 'acme--ceph-rook' },
      spec: {
        definitionRef: {
          organizationName: 'acme', pluginName: 'ceph-rook',
          pluginVersion: 'v0.2.0', definitionHash: 'sha256:abc',
        },
        config: { MON_COUNT: '1', DEV_LOOP_DEVICES: 'true' },
      },
    } as PluginInstallationItem);

    await component.onRetryInstall({ clusterId: 'c1', version: 'v0.2.0', hash: 'sha256:abc' });

    expect(installationService.installPlugin).toHaveBeenCalledWith(
      'c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc',
      { MON_COUNT: '1', DEV_LOOP_DEVICES: 'true' },
    );
  });
```

Adapt the arrange/act mechanics (selected plugin, waitForUninstall stubbing) to the spec file's existing helpers — the assertion above is the contract.

- [ ] **Step 5: Verify**

Run: `cd console-frontend && bun run test -- --no-watch` and `bun run build`
Expected: tests PASS, build clean (the build is the real typecheck).

---

### Task 6: Console — schema-driven config form in the install modal

**Files:**
- Create: `console-frontend/src/app/install-plugin-modal/plugin-config-form.component.ts` (inline template)
- Modify: `console-frontend/src/app/install-plugin-modal/install-plugin-modal.ts` and `.html`
- Modify: `console-frontend/src/app/plugins/plugins.component.html` and `plugin-details.component.html` (pass two new modal inputs)
- Test: `console-frontend/src/app/install-plugin-modal/plugin-config-form.component.spec.ts` (create)

**Interfaces:**
- Consumes: `ConfigSchemaEntry`, `ConfigType` from `../../generated/catalog/v1/catalog_pb` (Task 4); `CATALOG` token from `../../connect/tokens`; `InstallSelection.config` (Task 5).
- Produces: `PluginConfigFormComponent` with `schema = input<ConfigSchemaEntry[]>([])`, `submitLabel = input('Install')`, outputs `confirmed = output<Record<string, string>>()` and `cancelled = output<void>()`. Modal inputs `organizationName = input('')`, `pluginName = input('')` that BOTH parent templates must bind.

- [ ] **Step 1: Build the form component**

`plugin-config-form.component.ts` — schema-driven reactive form. Core behavior:
- One control per schema entry, in list order, pre-filled with `defaultValue`.
- Widgets by type: `CONFIG_TYPE_STRING`/`CONFIG_TYPE_INT` → `nldd-text-field` inside `nldd-form-field` (int gets `Validators.pattern(/^-?\d+$/)`); `CONFIG_TYPE_BOOL` → `nldd-checkbox`; `CONFIG_TYPE_ENUM` → `nldd-toggle-button-group` with one `nldd-toggle-button` per value (the pattern `new-cluster.component.html:42-56` uses).
- Required entries get `Validators.required` and the `.form-field-header` label markup with `__label`; optional ones use `nldd-form-field`'s `optional`/`optional-label` attributes (see `new-namespace-sheet.component.html:97-100`).
- Entries with `advanced` render inside a collapsed section (a `showAdvanced` signal toggled by a secondary `nldd-button`; hidden entries keep their controls so defaults survive).
- On submit, emit only the diff: keys whose value differs from `defaultValue`, plus every `required` key. **A cleared optional field emits nothing** — absence means "use the default", and an empty string would be rejected server-side (Review Focus 2):

```ts
  onSubmit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const config: Record<string, string> = {};
    for (const entry of this.schema()) {
      const raw = this.form.get(entry.name)?.value;
      const value = entry.type === ConfigType.BOOL ? String(raw === true || raw === 'true') : String(raw ?? '').trim();
      if (entry.required) {
        config[entry.name] = value;
      } else if (value !== '' && value !== entry.defaultValue) {
        config[entry.name] = value;
      }
    }
    this.confirmed.emit(config);
  }
```

(Enum member names in the generated TS are `ConfigType.STRING`/`INT`/`BOOL`/`ENUM` — confirm against the regenerated `catalog_pb.ts` and adjust; the generated names strip the `CONFIG_TYPE_` prefix in protobuf-es.)

Build the `FormGroup` from the `schema` input; since inputs are signals, construct the group in a `computed`/`effect` or in `ngOnInit` reading `this.schema()` — the modal only renders the form once a schema is known, so `ngOnInit` construction is sufficient. Component per project rules: OnPush, `inject(FormBuilder)`... actually use `new FormGroup`/`FormControl` directly or `inject(NonNullableFormBuilder)` — follow `new-namespace-sheet.component.ts:133-136`'s style. Import `ReactiveFormsModule` and `CUSTOM_ELEMENTS_SCHEMA` like that component does.

- [ ] **Step 2: Integrate into the modal**

`install-plugin-modal.ts`:
- New inputs: `organizationName = input('')`, `pluginName = input('')`.
- Inject the catalog client: `private catalogClient = inject(CATALOG);` (import `CATALOG` from `../../connect/tokens`, plus `create` from `@bufbuild/protobuf` and `GetPluginDefinitionRequestSchema`, `PluginRefSchema`, `ConfigSchemaEntry` from `../../generated/catalog/v1/catalog_pb`, `firstValueFrom` from `rxjs`).
- State: `pendingInstall = signal<{ clusterId: string; option: PluginVersionOption; schema: ConfigSchemaEntry[] } | null>(null)`, `schemaError = signal(false)`, `schemaLoading = signal(false)`, and a `Map<string, ConfigSchemaEntry[]>` cache keyed by version.
- Replace `onInstallOne`:

```ts
  async onInstallOne(clusterId: string, option: PluginVersionOption): Promise<void> {
    this.schemaError.set(false);
    this.schemaLoading.set(true);
    let schema: ConfigSchemaEntry[];
    try {
      schema = await this.fetchConfigSchema(option.version);
    } catch {
      // Without the schema there is no telling whether a form is needed, so
      // installing anyway could skip required config; surface the error instead.
      this.schemaError.set(true);
      return;
    } finally {
      this.schemaLoading.set(false);
    }
    if (schema.length === 0) {
      // No declared config: instant install, exactly the pre-schema behavior.
      this.install.emit({ clusterIds: [clusterId], version: option.version, hash: option.hash, config: {} });
      return;
    }
    this.pendingInstall.set({ clusterId, option, schema });
  }

  private async fetchConfigSchema(version: string): Promise<ConfigSchemaEntry[]> {
    const cached = this.schemaCache.get(version);
    if (cached) return cached;
    const resp = await firstValueFrom(
      this.catalogClient.getPluginDefinition(
        create(GetPluginDefinitionRequestSchema, {
          lookup: {
            case: 'name',
            value: { organizationName: this.organizationName(), pluginName: this.pluginName() },
          },
          version,
        }),
      ),
    );
    this.schemaCache.set(version, resp.configSchema);
    return resp.configSchema;
  }

  onConfigConfirmed(config: Record<string, string>): void {
    const pending = this.pendingInstall();
    if (!pending) return;
    this.pendingInstall.set(null);
    this.install.emit({
      clusterIds: [pending.clusterId],
      version: pending.option.version,
      hash: pending.option.hash,
      config,
    });
  }

  onConfigCancelled(): void {
    this.pendingInstall.set(null);
  }
```

(The `lookup` oneof literal shape depends on protobuf-es codegen — check the generated `GetPluginDefinitionRequest` type in `catalog_pb.ts` and match it; the `case: 'name'` form above is protobuf-es's standard oneof encoding.)
- Retry path (`onRetry`) stays as is — retry re-uses the previous CR's config via the Task 5 parent logic, no form.
- Reset `pendingInstall`, `schemaError`, and clear the cache in `onClose()` so a re-opened sheet starts clean.

`install-plugin-modal.html`: wrap the existing intro + cluster list in `@if (pendingInstall() === null) { ... } @else { <app-plugin-config-form [schema]="pendingInstall()!.schema" (confirmed)="onConfigConfirmed($event)" (cancelled)="onConfigCancelled()" /> }`, and add an error `nldd-inline-dialog` (mirroring the existing "Couldn't load versions" one at lines 26–32) shown when `schemaError()` is true. Add `PluginConfigFormComponent` to the modal's `imports`.

Parent templates — bind the two new inputs on `<app-install-plugin-modal>`:
- `plugins.component.html`: `[organizationName]="selectedPlugin?.organizationName ?? ''" [pluginName]="selectedPlugin?.name ?? ''"` (match how the template already reads `selectedPlugin`).
- `plugin-details.component.html`: same pattern from its `plugin()` signal.
- `cluster-plugins` does not use this modal — no change.
- The demo/tour flow (`data-tour="install-confirm"`) installs a fixture plugin with no schema, so the instant path keeps the walkthrough working; keep both `data-tour` attributes untouched.

- [ ] **Step 3: Write the form component tests**

`plugin-config-form.component.spec.ts`, following the project's component-spec conventions (look at `install-plugin-modal` siblings or `new-namespace-sheet` specs for TestBed setup). The four contracts to pin:

```ts
  it('prefills each field with its declared default', ...);
  // schema [{name: MON_COUNT, type: INT, defaultValue: '3'}] → control value '3'

  it('emits only values that differ from the default, plus required keys', ...);
  // set MON_COUNT to '1', leave DEV_LOOP_DEVICES at default, required FAILURE_DOMAIN chosen 'host'
  // → confirmed emits { MON_COUNT: '1', FAILURE_DOMAIN: 'host' }

  it('clearing an optional field omits the key entirely', ...);
  // clear MON_COUNT to '' → confirmed emits {} (not { MON_COUNT: '' })

  it('blocks submit while a required key is unchosen or an int is malformed', ...);
  // FAILURE_DOMAIN unset, or MON_COUNT 'three' → confirmed never emitted
```

And two modal-level tests in the modal's spec (create `install-plugin-modal.spec.ts` if none exists, stubbing `CATALOG`):

```ts
  it('empty schema installs immediately without showing the form', ...);
  // getPluginDefinition resolves { configSchema: [] } → install emitted with config {}

  it('non-empty schema shows the form instead of installing', ...);
  // getPluginDefinition resolves one entry → install NOT emitted, pendingInstall set
```

- [ ] **Step 4: Verify**

Run: `cd console-frontend && bun run test -- --no-watch` and `bun run build`
Expected: PASS and clean build.

---

### Task 7: ceph-rook adoption

**Files:**
- Modify: `plugins/storage/ceph-rook/definition.yaml` (metadata.version at line 6, `spec:` block)
- Modify: `plugins/storage/ceph-rook/config.go` (doc comment only)

**Interfaces:**
- Consumes: the `configSchema` manifest syntax (Task 1).
- Produces: nothing code-level; this is the feature's first real consumer.

- [ ] **Step 1: Declare the schema and bump the version**

In `definition.yaml`: change `version: v0.1.0` → `version: v0.2.0` (published versions are create-only; the schema changes the manifest bytes and therefore the hash, so it needs a new version).

Add under `spec:` (after `allowedResources`, keeping related keys grouped; defaults copied verbatim from the `envDefault` tags in `config.go`):

```yaml
  configSchema:
    - name: MON_COUNT
      type: int
      default: "3"
      description: Ceph monitors to run. A single-node cluster needs 1 (with ALLOW_MULTIPLE_PER_NODE), or mons never reach quorum.
    - name: MGR_COUNT
      type: int
      default: "2"
      description: Ceph managers to run. Use 1 on a single-node cluster.
    - name: ALLOW_MULTIPLE_PER_NODE
      type: bool
      default: "false"
      description: Allow multiple mons/mgrs on one node. Required on a single-node cluster.
    - name: DEV_LOOP_DEVICES
      type: bool
      default: "false"
      advanced: true
      description: "Local development only: discover ONLY /dev/loopNpN partitions and ignore the host's real disks. A safety control, not a convenience."
    - name: ALLOW_UNSUPPORTED_CEPH
      type: bool
      default: "false"
      advanced: true
      description: Waive Rook's supported-release check. Only for a Ceph release outside Rook's support table (the default image does not need it).
    - name: CEPH_IMAGE
      type: string
      default: "quay.io/ceph/ceph:v19.2.3"
      advanced: true
      description: Ceph container image. v19+ is required on arm64 (v18 segfaults on Apple Silicon).
    - name: ROOK_NAMESPACE
      type: string
      default: "rook-ceph"
      advanced: true
      description: Namespace the Rook operator runs in. Part of the CSI driver name; do not change on a cluster with existing storage.
    - name: CLUSTER_NAMESPACE
      type: string
      default: "rook-ceph"
      advanced: true
      description: Namespace for the CephCluster and its CSI secrets; the CSI clusterID.
```

All eight keys from `config.go` are declared — leaving any undeclared would make the controller (Task 3) reject the runbook's own documented config under the reject-undeclared rule.

- [ ] **Step 2: Note the sync obligation in config.go**

Add one line to the `Config` struct's doc comment in `config.go` (keep it short per the user's comment-length preference):

```go
// Keys, types and defaults are mirrored in definition.yaml's configSchema;
// keep the two in sync (the manifest copy is what the install form shows).
```

- [ ] **Step 3: Verify the manifest parses**

Write a throwaway checker to the scratchpad directory (NOT the repo), then run it from the worktree root:

```go
// <scratchpad>/parsecheck/main.go
package main

import (
	"fmt"
	"os"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	data, err := os.ReadFile("plugins/storage/ceph-rook/definition.yaml")
	if err != nil {
		panic(err)
	}
	def, err := pluginruntime.ParseSourceDefinition(data)
	if err != nil {
		panic(err)
	}
	fmt.Printf("ok: %d configSchema entries, version %s\n", len(def.Spec.ConfigSchema), def.Metadata.Version)
}
```

Run (from the worktree root, where the module can resolve the SDK): `go run <scratchpad>/parsecheck/main.go`
Expected: `ok: 8 configSchema entries, version v0.2.0`. A parse error here means the YAML violates a Task 1 rule — fix `definition.yaml`, not the parser. Delete nothing from the repo afterwards; the checker lives only in the scratchpad.

---

### Task 8: Documentation

**Files:**
- Modify: `docs/funs/FUN-11.adoc` (definition structure ~110–121, install example ~195, lifecycle ~215)
- Modify: `docs/developer/plugins/writing-a-plugin.md`
- Modify: `docs/developer/plugins/ceph-rook-runbook.md` (Phase 4 install, ~258–292)
- Modify: `plugins/storage/ceph-rook/test-resources.yaml` (comment block at 9–19)

**Interfaces:** none — prose only. Do not touch `docs-frontend/`.

- [ ] **Step 1: FUN-11**

In the "Definition structure" bullet list, after the `crds` bullet, add:

```asciidoc
* **configSchema**: Optional list of declared install-time config keys. Each entry has `name` (uppercase, becomes the `FUNP_<name>` env var), `type` (`string`/`int`/`bool`/`enum`), `default`, `description`, `required`, `values` (enum only), and `advanced` (collapsed by default in the install form). The console renders the install form from this list; the plugin-controller rejects a `spec.config` that violates it. A definition without a schema accepts any config (pre-schema behavior).
```

Update the install example comment at ~line 195:

```
  config: {}             # optional: key-value config (injected as FUNP_* env vars, validated against the definition's configSchema when one is declared)
```

In the installation lifecycle, replace the step "Plugin config values are injected as `FUNP_*` environment variables." with:

```asciidoc
. Validates `spec.config` against the pinned definition's `configSchema` (when declared); a violation fails the install with a `ConfigValid=False` condition before any resources are materialised.
. Plugin config values are injected as `FUNP_*` environment variables.
```

- [ ] **Step 2: writing-a-plugin.md**

Add a "Declaring config" section (place it near where the definition's spec fields are described; read the file's flow first). Content: the `configSchema` YAML block from Task 7 as the worked example (trimmed to 3 entries), the four types, required-vs-default exclusivity, the reject-undeclared rule, the advanced flag, and one sentence on scope: *"Secret values do not belong in `spec.config` — it is a plain CR field readable by anyone who can `get plugininstallations`; a secretRef mechanism is a separate design."*

- [ ] **Step 3: ceph-rook-runbook.md**

In "Phase 4 · Install": keep the `kubectl apply` heredoc (it remains the documented path until this feature ships in a release) but update its `pluginVersion: v0.1.0` → `v0.2.0`, and add above it:

```markdown
Once the appstore install form ships (configSchema support), this config can be
entered in the Console at install time instead of hand-writing the CR below.
```

- [ ] **Step 4: test-resources.yaml**

Replace the commented dev-config block (lines ~9–19) with a two-line pointer: the config keys are now declared in `definition.yaml`'s `configSchema` and documented in the runbook Phase 4 — keep the loop-device attach instructions (`just storage-disks attach`) untouched, they are about disks, not config.

- [ ] **Step 5: Verify docs sync**

Run: `just generate` is NOT needed for docs; but confirm no `docs-frontend/` file was touched: `git status --short | grep docs-frontend` → empty.

---

## Final verification (after all tasks, before asking the user to approve the commit)

- [ ] `go build ./...` — whole workspace compiles.
- [ ] `go test ./plugin-sdk/... ./plugin-controller/... ./marketplace-catalog-api/...` — all green.
- [ ] `cd console-frontend && bun run test -- --no-watch && bun run build` — all green, build clean.
- [ ] `git status` — only intended files; nothing under `db/migrations/`, `db/fundament.sql`, or `docs-frontend/`.
- [ ] Then report results to the user and ask for commit approval — ONE commit, no Co-Authored-By.
