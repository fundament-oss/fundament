# Envoy Gateway Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a second Gateway API implementation plugin backed by Envoy Gateway alongside the existing Istio one, restructured under `plugins/gateway-api/{istio,envoy-gateway}/`, where the plugin installs the platform and users configure Gateways through the console.

**Architecture:** The Go plugin Helm-installs the Envoy Gateway controller (OCI chart, bundled CRDs), ensures the `eg` GatewayClass, and reconciles health — no baked default Gateway. The console gets generic list/detail for the Gateway API + Envoy policy CRDs for free (menu declaration) plus a plugin-shipped custom "guided create" form for `Gateway` (a Vite/TS app embedded into the plugin binary, following the openfsc `console-ui/` pattern).

**Tech Stack:** Go (controller-runtime, unstructured client, Helm CLI via the shared `helm` helper), `testify` for Go tests. Frontend: vanilla TypeScript + Vite + `bun`, `@nldd/design-system` web components, `vitest`/`happy-dom` for tests.

## Global Constraints

- **Commit policy:** Do NOT commit until the user explicitly approves. Implement all tasks first, then commit once at the end (single commit, no per-task commits). Never add a `Co-Authored-By` trailer.
- **Go tests:** Use `github.com/stretchr/testify/assert` and `.../require` — never hand-rolled `if got != want { t.Errorf }`.
- **Panic in switch default** when all enum cases should be exhaustively handled.
- **Frontend package manager:** `bun` (never `npm`).
- **File moves** use `git mv` to preserve history.
- **Envoy Gateway pinned chart version:** `v1.8.3` (OCI chart `oci://docker.io/envoyproxy/gateway-helm`, release `eg`, namespace `envoy-gateway-system`, controller deployment `envoy-gateway`, GatewayClass controllerName `gateway.envoyproxy.io/gatewayclass-controller`).
- **Plugin identities:** existing Istio plugin becomes `gateway-api-istio`; new plugin is `gateway-api-envoy`.
- All `go` and `just` commands run from the repo root (`/Users/chiel/projects/github.com/fundament-oss/fundament`) unless stated.

---

## File Structure

**Restructure (Task 1):**
- `plugins/gateway-api/istio/` — all current `plugins/gateway-api/*` files moved here (logic unchanged); `Dockerfile` + `definition.yaml` paths/name updated.

**Shared helper (Task 2):**
- `plugin-sdk/pluginruntime/helpers/helm/helm.go` — add `InstallFromOCI` + testable `ociInstallArgs`.
- `plugin-sdk/pluginruntime/helpers/helm/helm_test.go` — add arg-builder test.

**New Envoy plugin (`plugins/gateway-api/envoy-gateway/`):**
- `config.go` — `pluginConfig` (3 env vars).
- `gatewayclass.go` — `buildGatewayClass` (+ GVK helpers).
- `envoygateway.go` — `envoyGatewayInstaller` (install/uninstall/isInstalled + chart spec).
- `plugin.go` — lifecycle (`Start`/`Install`/`Uninstall`/`Upgrade`/`Reconcile`, `ensureGatewayClass`, health check, `listUserResources`).
- `main.go` — entrypoint.
- `console.go` — `go:embed console/*`.
- `definition.yaml` — RBAC, menu, customComponents, allowedResources, crds, uiHints.
- `Dockerfile` — console-ui build stage + go build + helm runtime.
- `console/.gitkeep` — placeholder for embedded build output (gitignored dir).
- `console-ui/` — Vite/TS app (`package.json`, `tsconfig.json`, `vite.config.ts`, `gateways-create.html`, `src/{sdk,shared,nldd-design-system,form,create}.ts`, `src/form.test.ts`).
- `Justfile` — console-ui `typecheck`/`test` + sandbox `test`/`test-cleanup` recipes.
- Go test files: `envoygateway_test.go`, `gatewayclass_test.go`, `config_test.go`.

**Repo-wide edits:**
- `.dockerignore` — ignore the new `console-ui/node_modules/` and built `console/`.
- `plugins/Justfile` — register `mod envoy-gateway` (nested under `gateway-api/`).

---

## Task 1: Restructure — move Istio plugin into `gateway-api/istio/`

**Files:**
- Move: `plugins/gateway-api/{config.go,istio.go,gateway.go,istio_test.go,console.go,plugin.go,gateway_test.go,definition.yaml,main.go,Dockerfile}` and `plugins/gateway-api/console/` → `plugins/gateway-api/istio/`
- Modify: `plugins/gateway-api/istio/Dockerfile`, `plugins/gateway-api/istio/definition.yaml`

**Interfaces:**
- Produces: the directory `plugins/gateway-api/istio/` building as `./plugins/gateway-api/istio`; plugin `metadata.name: gateway-api-istio`.

- [ ] **Step 1: Move the files with git mv**

```bash
cd /Users/chiel/projects/github.com/fundament-oss/fundament
mkdir -p plugins/gateway-api/istio
git mv plugins/gateway-api/config.go plugins/gateway-api/istio.go plugins/gateway-api/gateway.go \
       plugins/gateway-api/istio_test.go plugins/gateway-api/console.go plugins/gateway-api/plugin.go \
       plugins/gateway-api/gateway_test.go plugins/gateway-api/definition.yaml plugins/gateway-api/main.go \
       plugins/gateway-api/Dockerfile plugins/gateway-api/istio/
git mv plugins/gateway-api/console plugins/gateway-api/istio/console
```

- [ ] **Step 2: Update the Istio Dockerfile build + copy paths**

In `plugins/gateway-api/istio/Dockerfile`, change the two `gateway-api` paths to `gateway-api/istio`:

```dockerfile
RUN CGO_ENABLED=0 go build -o /bin/gateway-api ./plugins/gateway-api/istio
```
```dockerfile
COPY plugins/gateway-api/istio/definition.yaml /app/definition.yaml
```

- [ ] **Step 3: Rename the plugin identity in the Istio definition**

In `plugins/gateway-api/istio/definition.yaml`, update metadata:

```yaml
  name: gateway-api-istio
  displayName: Gateway API (Istio)
```

- [ ] **Step 4: Verify the moved plugin still builds and tests pass**

Run:
```bash
go build ./plugins/gateway-api/istio && go test ./plugins/gateway-api/istio/...
```
Expected: build succeeds; existing tests (`TestIstioInstallerChartOrder`, `TestBuildDefaultGateway`, …) PASS.

- [ ] **Step 5: Confirm nothing else references the old path**

Run:
```bash
grep -rn "plugins/gateway-api\"" --include="*.go" . ; grep -rn "gateway-api/config\|plugins/gateway-api/definition" . | grep -v "gateway-api/istio"
```
Expected: no stray references to the pre-move path (Justfile addresses plugins by the `name` arg, so `just plugin-publish gateway-api/istio` works without edits).

---

## Task 2: Add `InstallFromOCI` to the shared Helm helper

**Files:**
- Modify: `plugin-sdk/pluginruntime/helpers/helm/helm.go`
- Test: `plugin-sdk/pluginruntime/helpers/helm/helm_test.go`

**Interfaces:**
- Produces: `func (c *Client) InstallFromOCI(ctx context.Context, releaseName, chartRef, version string, values map[string]string) error` and pure helper `func (c *Client) ociInstallArgs(releaseName, chartRef, version string, values map[string]string) []string`.

- [ ] **Step 1: Write the failing test**

Add to `plugin-sdk/pluginruntime/helpers/helm/helm_test.go`:

```go
func TestOCIInstallArgs(t *testing.T) {
	c := NewClient("envoy-gateway-system")
	args := c.ociInstallArgs("eg", "oci://docker.io/envoyproxy/gateway-helm", "v1.8.3",
		map[string]string{"b": "2", "a": "1"})

	assert.Equal(t, []string{
		"upgrade", "--install", "eg", "oci://docker.io/envoyproxy/gateway-helm",
		"--namespace", "envoy-gateway-system", "--create-namespace", "--wait",
		"--version", "v1.8.3",
		"--set", "a=1", "--set", "b=2",
	}, args)
}

func TestOCIInstallArgsNoVersion(t *testing.T) {
	c := NewClient("ns")
	args := c.ociInstallArgs("r", "oci://example/chart", "", nil)
	assert.NotContains(t, args, "--version")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin-sdk/pluginruntime/helpers/helm/ -run TestOCIInstallArgs -v`
Expected: FAIL — `c.ociInstallArgs undefined`.

- [ ] **Step 3: Implement `ociInstallArgs` and `InstallFromOCI`**

In `plugin-sdk/pluginruntime/helpers/helm/helm.go`, after `InstallFromRepo`:

```go
// ociInstallArgs builds the "helm upgrade --install" argument list for an OCI
// chart reference, pinning --version when set. Split out from InstallFromOCI so
// the argument construction is unit-testable without shelling out to helm.
func (c *Client) ociInstallArgs(releaseName, chartRef, version string, values map[string]string) []string {
	args := []string{"upgrade", "--install", releaseName, chartRef, "--namespace", c.namespace, "--create-namespace", "--wait"}
	if version != "" {
		args = append(args, "--version", version)
	}
	return appendSortedValues(args, values)
}

// InstallFromOCI runs "helm upgrade --install" for a chart hosted in an OCI
// registry (chartRef like "oci://docker.io/envoyproxy/gateway-helm"). Unlike
// InstallFromRepo there is no --repo; the version pins the chart via --version.
func (c *Client) InstallFromOCI(ctx context.Context, releaseName, chartRef, version string, values map[string]string) error {
	return c.runInstall(ctx, c.ociInstallArgs(releaseName, chartRef, version, values))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin-sdk/pluginruntime/helpers/helm/ -v`
Expected: PASS (including existing `TestIsRBACForbidden`).

---

## Task 3: Envoy plugin config

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/config.go`
- Test: `plugins/gateway-api/envoy-gateway/config_test.go`

**Interfaces:**
- Produces: `type pluginConfig struct { EnvoyGatewayVersion, GatewayNamespace, GatewayClassName string }` with `FUNP_*` env tags and defaults.

- [ ] **Step 1: Write the failing test**

Create `plugins/gateway-api/envoy-gateway/config_test.go`:

```go
package main

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/require"
)

func TestPluginConfigDefaults(t *testing.T) {
	var cfg pluginConfig
	require.NoError(t, env.Parse(&cfg))

	require.Equal(t, "v1.8.3", cfg.EnvoyGatewayVersion)
	require.Equal(t, "envoy-gateway-system", cfg.GatewayNamespace)
	require.Equal(t, "eg", cfg.GatewayClassName)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestPluginConfigDefaults -v`
Expected: FAIL — `undefined: pluginConfig`.

- [ ] **Step 3: Implement config**

Create `plugins/gateway-api/envoy-gateway/config.go`:

```go
package main

// pluginConfig holds envoy-gateway plugin configuration from FUNP_* env vars.
// These are operator-level knobs only; per-Gateway configuration is done by
// users through the console CRD forms, not here.
type pluginConfig struct {
	EnvoyGatewayVersion string `env:"FUNP_ENVOY_GATEWAY_VERSION" envDefault:"v1.8.3"`
	GatewayNamespace    string `env:"FUNP_GATEWAY_NAMESPACE" envDefault:"envoy-gateway-system"`
	GatewayClassName    string `env:"FUNP_GATEWAY_CLASS_NAME" envDefault:"eg"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestPluginConfigDefaults -v`
Expected: PASS.

---

## Task 4: GatewayClass builder

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/gatewayclass.go`
- Test: `plugins/gateway-api/envoy-gateway/gatewayclass_test.go`

**Interfaces:**
- Consumes: `pluginConfig` (Task 3).
- Produces: `func buildGatewayClass(cfg pluginConfig) []byte`; `func gatewayClassGVK() schema.GroupVersionKind`; `func deploymentGVK() schema.GroupVersionKind`.

- [ ] **Step 1: Write the failing test**

Create `plugins/gateway-api/envoy-gateway/gatewayclass_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestBuildGatewayClass(t *testing.T) {
	cfg := pluginConfig{GatewayClassName: "eg"}

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(buildGatewayClass(cfg), &parsed))

	require.Equal(t, "GatewayClass", parsed["kind"])
	require.Equal(t, "eg", parsed["metadata"].(map[string]any)["name"])
	require.Equal(t, "gateway.envoyproxy.io/gatewayclass-controller",
		parsed["spec"].(map[string]any)["controllerName"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestBuildGatewayClass -v`
Expected: FAIL — `undefined: buildGatewayClass`.

- [ ] **Step 3: Implement the builder and GVK helpers**

Create `plugins/gateway-api/envoy-gateway/gatewayclass.go`:

```go
package main

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// envoyControllerName is the controllerName Envoy Gateway watches for; a
// GatewayClass carrying it binds the class to the installed controller.
const envoyControllerName = "gateway.envoyproxy.io/gatewayclass-controller"

const gatewayClassTemplate = `apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: %s
spec:
  controllerName: %s
`

// buildGatewayClass renders the cluster-scoped GatewayClass that binds the
// configured class name to the Envoy Gateway controller. No parametersRef:
// data-plane infra (service type, replicas) is configured per-need by users via
// EnvoyProxy / Gateway.spec.infrastructure, not baked into the class.
func buildGatewayClass(cfg pluginConfig) []byte {
	return fmt.Appendf(nil, gatewayClassTemplate, cfg.GatewayClassName, envoyControllerName)
}

func gatewayClassGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass"}
}

func deploymentGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestBuildGatewayClass -v`
Expected: PASS.

---

## Task 5: Envoy Gateway installer

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/envoygateway.go`
- Test: `plugins/gateway-api/envoy-gateway/envoygateway_test.go`

**Interfaces:**
- Consumes: `pluginConfig` (Task 3); `helm.NewClient`, `(*helm.Client).InstallFromOCI/Uninstall/IsInstalled` (Task 2).
- Produces: `type envoyGatewayInstaller struct{...}`; `func newEnvoyGatewayInstaller(cfg pluginConfig) *envoyGatewayInstaller`; methods `chart() chartSpec`, `install(ctx) error`, `uninstall(ctx) error`, `isInstalled(ctx) (bool, error)`. `type chartSpec struct { releaseName, chartRef, version string }`. Constants `envoyGatewayReleaseName = "eg"`, `envoyGatewayChartRef = "oci://docker.io/envoyproxy/gateway-helm"`.

- [ ] **Step 1: Write the failing test**

Create `plugins/gateway-api/envoy-gateway/envoygateway_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvoyGatewayChartSpec(t *testing.T) {
	installer := newEnvoyGatewayInstaller(pluginConfig{
		EnvoyGatewayVersion: "v1.8.3",
		GatewayNamespace:    "envoy-gateway-system",
	})

	spec := installer.chart()
	require.Equal(t, "eg", spec.releaseName)
	require.Equal(t, "oci://docker.io/envoyproxy/gateway-helm", spec.chartRef)
	require.Equal(t, "v1.8.3", spec.version)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestEnvoyGatewayChartSpec -v`
Expected: FAIL — `undefined: newEnvoyGatewayInstaller`.

- [ ] **Step 3: Implement the installer**

Create `plugins/gateway-api/envoy-gateway/envoygateway.go`:

```go
package main

import (
	"context"
	"fmt"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	envoyGatewayReleaseName = "eg"
	envoyGatewayChartRef    = "oci://docker.io/envoyproxy/gateway-helm"
)

// chartSpec is the single Helm OCI release that installs the Envoy Gateway
// controller (and, bundled with it, the Gateway API + Envoy CRDs).
type chartSpec struct {
	releaseName string
	chartRef    string
	version     string
}

type envoyGatewayInstaller struct {
	cfg        pluginConfig
	helmClient *helm.Client
}

func newEnvoyGatewayInstaller(cfg pluginConfig) *envoyGatewayInstaller {
	return &envoyGatewayInstaller{
		cfg:        cfg,
		helmClient: helm.NewClient(cfg.GatewayNamespace),
	}
}

func (i *envoyGatewayInstaller) chart() chartSpec {
	return chartSpec{
		releaseName: envoyGatewayReleaseName,
		chartRef:    envoyGatewayChartRef,
		version:     i.cfg.EnvoyGatewayVersion,
	}
}

func (i *envoyGatewayInstaller) install(ctx context.Context) error {
	c := i.chart()
	if err := i.helmClient.InstallFromOCI(ctx, c.releaseName, c.chartRef, c.version, nil); err != nil {
		return fmt.Errorf("install %s: %w", c.releaseName, err)
	}
	return nil
}

func (i *envoyGatewayInstaller) uninstall(ctx context.Context) error {
	if err := i.helmClient.Uninstall(ctx, envoyGatewayReleaseName); err != nil {
		return fmt.Errorf("uninstall %s: %w", envoyGatewayReleaseName, err)
	}
	return nil
}

func (i *envoyGatewayInstaller) isInstalled(ctx context.Context) (bool, error) {
	return i.helmClient.IsInstalled(ctx, envoyGatewayReleaseName)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run TestEnvoyGatewayChartSpec -v`
Expected: PASS.

---

## Task 6: Plugin lifecycle

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/plugin.go`
- Test: `plugins/gateway-api/envoy-gateway/plugin_test.go`

**Interfaces:**
- Consumes: `newEnvoyGatewayInstaller` (Task 5); `buildGatewayClass`, `gatewayClassGVK`, `deploymentGVK` (Task 4); `pluginConfig` (Task 3).
- Produces: `type EnvoyGatewayPlugin struct{...}`; `func NewEnvoyGatewayPlugin() (*EnvoyGatewayPlugin, error)`; the `pluginruntime.Plugin` methods; `var gatewayAPICRDs []string`, `var envoyGatewayCRDs []string`; `func (p *EnvoyGatewayPlugin) listUserResources(ctx) ([]string, error)`.

- [ ] **Step 1: Write the failing test**

Create `plugins/gateway-api/envoy-gateway/plugin_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifiedCRDsCoverStandardAndEnvoy(t *testing.T) {
	all := verifiedCRDs()

	// The 5 standard Gateway API resources.
	for _, name := range []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"grpcroutes.gateway.networking.k8s.io",
		"tcproutes.gateway.networking.k8s.io",
		"tlsroutes.gateway.networking.k8s.io",
	} {
		assert.Contains(t, all, name)
	}
	// Envoy policy CRDs + EnvoyProxy.
	for _, name := range []string{
		"envoyproxies.gateway.envoyproxy.io",
		"securitypolicies.gateway.envoyproxy.io",
		"backendtrafficpolicies.gateway.envoyproxy.io",
		"clienttrafficpolicies.gateway.envoyproxy.io",
	} {
		assert.Contains(t, all, name)
	}
}

func TestNewEnvoyGatewayPlugin(t *testing.T) {
	p, err := NewEnvoyGatewayPlugin()
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "eg", p.cfg.GatewayClassName)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run 'TestVerifiedCRDs|TestNewEnvoyGatewayPlugin' -v`
Expected: FAIL — `undefined: verifiedCRDs` / `NewEnvoyGatewayPlugin`.

- [ ] **Step 3: Implement the lifecycle**

Create `plugins/gateway-api/envoy-gateway/plugin.go`:

```go
package main

import (
	"context"
	"fmt"

	"github.com/caarlos0/env/v11"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

var gatewayAPICRDs = []string{
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
	"grpcroutes.gateway.networking.k8s.io",
	"tcproutes.gateway.networking.k8s.io",
	"tlsroutes.gateway.networking.k8s.io",
}

var envoyGatewayCRDs = []string{
	"envoyproxies.gateway.envoyproxy.io",
	"securitypolicies.gateway.envoyproxy.io",
	"backendtrafficpolicies.gateway.envoyproxy.io",
	"clienttrafficpolicies.gateway.envoyproxy.io",
}

// verifiedCRDs is the full set the plugin checks for after install — the Envoy
// Gateway chart bundles all of them.
func verifiedCRDs() []string {
	return append(append([]string{}, gatewayAPICRDs...), envoyGatewayCRDs...)
}

// EnvoyGatewayPlugin installs and runs the Envoy Gateway platform. Users create
// Gateways/Routes/policies through the console CRD forms; the plugin never
// creates a default Gateway.
type EnvoyGatewayPlugin struct {
	cfg       pluginConfig
	installer *envoyGatewayInstaller
	k8sClient client.Client
}

func NewEnvoyGatewayPlugin() (*EnvoyGatewayPlugin, error) {
	var cfg pluginConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}
	return &EnvoyGatewayPlugin{cfg: cfg, installer: newEnvoyGatewayInstaller(cfg)}, nil
}

func (p *EnvoyGatewayPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	installed, err := p.installer.isInstalled(ctx)
	if err != nil {
		return fmt.Errorf("check envoy gateway status: %w", pluginerrors.NewTransient(err))
	}
	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing Envoy Gateway"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install envoy gateway: %w", pluginerrors.NewTransient(err))
		}
	}

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("add apiextensions to scheme: %w", pluginerrors.NewPermanent(err))
	}

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}
	p.k8sClient = k8sClient

	if err := crd.VerifyAll(ctx, p.k8sClient, verifiedCRDs()); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	if err := p.ensureGatewayClass(ctx); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("ensure gateway class: %w", pluginerrors.NewTransient(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Envoy Gateway is running"})

	<-ctx.Done()
	return nil
}

func (p *EnvoyGatewayPlugin) Shutdown(_ context.Context) error { return nil }

func (p *EnvoyGatewayPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.installer.install(ctx); err != nil {
		return fmt.Errorf("install envoy gateway: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) Uninstall(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient != nil {
		remaining, err := p.listUserResources(ctx)
		if err != nil {
			return fmt.Errorf("check user resources: %w", err)
		}
		if len(remaining) > 0 {
			return fmt.Errorf("cannot uninstall: %d user-created Gateway/Route resources still exist — remove them first", len(remaining))
		}
		if err := p.deleteGatewayClass(ctx); err != nil {
			host.Logger().Warn("failed to delete gateway class during uninstall", "error", err)
		}
	}
	if err := p.installer.uninstall(ctx); err != nil {
		return fmt.Errorf("uninstall envoy gateway: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *EnvoyGatewayPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient == nil {
		return nil
	}
	if err := crd.VerifyAll(ctx, p.k8sClient, verifiedCRDs()); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}
	if !p.isControllerHealthy(ctx) {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "envoy-gateway control plane is unhealthy"})
		return fmt.Errorf("reconcile: envoy-gateway unhealthy: %w", pluginerrors.NewTransient(fmt.Errorf("envoy-gateway not ready")))
	}
	if err := p.ensureGatewayClass(ctx); err != nil {
		host.Logger().Warn("reconcile: failed to ensure gateway class", "error", err)
	}
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Envoy Gateway is running"})
	return nil
}

func (p *EnvoyGatewayPlugin) ensureGatewayClass(ctx context.Context) error {
	gc := &unstructured.Unstructured{}
	gc.SetGroupVersionKind(gatewayClassGVK())
	err := p.k8sClient.Get(ctx, types.NamespacedName{Name: p.cfg.GatewayClassName}, gc)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get gateway class: %w", err)
	}

	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(buildGatewayClass(p.cfg), &obj.Object); err != nil {
		return fmt.Errorf("parse gateway class: %w", err)
	}
	if err := p.k8sClient.Create(ctx, obj); err != nil {
		return fmt.Errorf("create gateway class: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) deleteGatewayClass(ctx context.Context) error {
	gc := &unstructured.Unstructured{}
	gc.SetGroupVersionKind(gatewayClassGVK())
	gc.SetName(p.cfg.GatewayClassName)
	if err := p.k8sClient.Delete(ctx, gc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete gateway class: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) isControllerHealthy(ctx context.Context) bool {
	deploy := &unstructured.Unstructured{}
	deploy.SetGroupVersionKind(deploymentGVK())
	if err := p.k8sClient.Get(ctx, types.NamespacedName{Name: "envoy-gateway", Namespace: p.cfg.GatewayNamespace}, deploy); err != nil {
		return false
	}
	status, ok := deploy.Object["status"].(map[string]any)
	if !ok {
		return false
	}
	available, _ := status["availableReplicas"].(float64)
	return available > 0
}

// listUserResources returns user-created Gateways/Routes that block uninstall.
// Unlike the Istio plugin there is no default Gateway to exclude — any Gateway
// or Route counts.
func (p *EnvoyGatewayPlugin) listUserResources(ctx context.Context) ([]string, error) {
	var resources []string
	gvks := []struct {
		gvk      schema.GroupVersionKind
		listKind string
	}{
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"}, "GatewayList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}, "HTTPRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GRPCRoute"}, "GRPCRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TCPRoute"}, "TCPRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TLSRoute"}, "TLSRouteList"},
	}
	for _, g := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: g.gvk.Group, Version: g.gvk.Version, Kind: g.listKind})
		if err := p.k8sClient.List(ctx, list); err != nil {
			return nil, fmt.Errorf("list %s: %w", g.gvk.Kind, err)
		}
		for _, item := range list.Items {
			resources = append(resources, fmt.Sprintf("%s/%s/%s", item.GetKind(), item.GetNamespace(), item.GetName()))
		}
	}
	return resources, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/gateway-api/envoy-gateway/ -run 'TestVerifiedCRDs|TestNewEnvoyGatewayPlugin' -v`
Expected: PASS.

> **Note:** confirm the `pluginruntime.Host`/`Plugin` interface method set matches the Istio plugin's (compare `plugins/gateway-api/istio/plugin.go`). If `crd.VerifyAll`, `pluginerrors`, or `host.ReportReady` signatures differ, mirror the Istio plugin exactly — it is the source of truth for the SDK surface.

---

## Task 7: main.go and console embed

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/main.go`
- Create: `plugins/gateway-api/envoy-gateway/console.go`
- Create: `plugins/gateway-api/envoy-gateway/console/.gitkeep` (empty file)

**Interfaces:**
- Consumes: `NewEnvoyGatewayPlugin` (Task 6).
- Produces: runnable `main`; `func (p *EnvoyGatewayPlugin) ConsoleAssets() http.FileSystem`.

- [ ] **Step 1: Create main.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	plugin, err := NewEnvoyGatewayPlugin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create plugin: %v\n", err)
		os.Exit(1)
	}
	pluginruntime.Run(plugin)
}
```

- [ ] **Step 2: Create console.go**

```go
package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

// RequireHTML: console/ holds the Vite build output (gitignored) — a binary
// built without it would serve a blank iframe instead of failing.
func (p *EnvoyGatewayPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console", console.RequireHTML())
}
```

- [ ] **Step 3: Create the console placeholder so `go:embed` and `go test` resolve**

```bash
touch plugins/gateway-api/envoy-gateway/console/.gitkeep
```

- [ ] **Step 4: Verify the whole package builds and all Go tests pass**

Run: `go build ./plugins/gateway-api/envoy-gateway && go test ./plugins/gateway-api/envoy-gateway/... -v`
Expected: build succeeds; all tests PASS.

---

## Task 8: definition.yaml

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/definition.yaml`

**Interfaces:**
- Produces: the published plugin manifest; menu CRDs and `customComponents.Gateway.create: gateways-create.html` (consumed by Task 10's HTML filename), `allowedResources` for `gateways` (consumed by Task 11's create form SDK calls).

- [ ] **Step 1: Write the definition**

```yaml
apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: gateway-api-envoy
  displayName: Gateway API (Envoy Gateway)
  version: v0.1.0
  description: Gateway API implementation powered by Envoy Gateway. Manages Gateways, HTTPRoutes, GRPCRoutes, TCPRoutes, TLSRoutes, and Envoy Gateway policies.
  author: Fundament
  license: Apache-2.0
  icon: network
  urls:
    homepage: https://gateway.envoyproxy.io
    repository: https://github.com/envoyproxy/gateway
    documentation: https://gateway.envoyproxy.io/docs/
  tags:
    - networking
    - gateway
    - envoy
    - ingress
spec:
  permissions:
    capabilities:
      - internet_access
    rbac:
      - apiGroups: ["gateway.networking.k8s.io"]
        resources: ["gateways", "httproutes", "grpcroutes", "tcproutes", "tlsroutes"]
        verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
      - apiGroups: ["gateway.networking.k8s.io"]
        resources: ["gateways/status", "httproutes/status", "grpcroutes/status", "tcproutes/status", "tlsroutes/status"]
        verbs: ["get", "update"]
      - apiGroups: ["gateway.networking.k8s.io"]
        resources: ["gatewayclasses"]
        verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
      - apiGroups: ["gateway.networking.k8s.io"]
        resources: ["gatewayclasses/status"]
        verbs: ["get", "update"]
      - apiGroups: ["gateway.envoyproxy.io"]
        resources: ["*"]
        verbs: ["get", "list", "watch"]
      - apiGroups: ["gateway.envoyproxy.io"]
        resources: ["envoyproxies", "securitypolicies", "backendtrafficpolicies", "clienttrafficpolicies"]
        verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
      - apiGroups: [""]
        resources: ["secrets"]
        verbs: ["get", "list", "watch"]
      - apiGroups: [""]
        resources: ["namespaces"]
        verbs: ["get", "list", "watch"]

  crds:
    - gateways.gateway.networking.k8s.io
    - httproutes.gateway.networking.k8s.io
    - grpcroutes.gateway.networking.k8s.io
    - tcproutes.gateway.networking.k8s.io
    - tlsroutes.gateway.networking.k8s.io
    - securitypolicies.gateway.envoyproxy.io
    - backendtrafficpolicies.gateway.envoyproxy.io
    - clienttrafficpolicies.gateway.envoyproxy.io

  menu:
    project:
      - crd: gateways.gateway.networking.k8s.io
        list: true
        detail: true
        icon: server
      - crd: httproutes.gateway.networking.k8s.io
        list: true
        detail: true
        icon: globe
      - crd: grpcroutes.gateway.networking.k8s.io
        list: true
        detail: true
        icon: zap
      - crd: tcproutes.gateway.networking.k8s.io
        list: true
        detail: true
        icon: cable
      - crd: tlsroutes.gateway.networking.k8s.io
        list: true
        detail: true
        icon: lock
      - crd: securitypolicies.gateway.envoyproxy.io
        list: true
        detail: true
        icon: shield
      - crd: backendtrafficpolicies.gateway.envoyproxy.io
        list: true
        detail: true
        icon: gauge
      - crd: clienttrafficpolicies.gateway.envoyproxy.io
        list: true
        detail: true
        icon: gauge
    organization:
      - crd: gateways.gateway.networking.k8s.io
        list: true
        detail: true
        icon: server

  customComponents:
    Gateway:
      create: gateways-create.html

  allowedResources:
    - group: gateway.networking.k8s.io
      version: v1
      resource: gateways
      verbs: [list, get, create]

  uiHints:
    gateways.gateway.networking.k8s.io:
      statusMapping:
        jsonPath: ".status.conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    httproutes.gateway.networking.k8s.io:
      statusMapping:
        jsonPath: ".status.parents[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    grpcroutes.gateway.networking.k8s.io:
      statusMapping:
        jsonPath: ".status.parents[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    tcproutes.gateway.networking.k8s.io:
      statusMapping:
        jsonPath: ".status.parents[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    tlsroutes.gateway.networking.k8s.io:
      statusMapping:
        jsonPath: ".status.parents[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    securitypolicies.gateway.envoyproxy.io:
      statusMapping:
        jsonPath: ".status.ancestors[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    backendtrafficpolicies.gateway.envoyproxy.io:
      statusMapping:
        jsonPath: ".status.ancestors[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
    clienttrafficpolicies.gateway.envoyproxy.io:
      statusMapping:
        jsonPath: ".status.ancestors[0].conditions[?(@.type==\"Accepted\")].status"
        values:
          "True": { badge: success, label: Accepted }
          "False": { badge: danger, label: Rejected }
        default: { badge: info, label: Pending }
```

- [ ] **Step 2: Validate YAML parses**

Run: `bun x js-yaml plugins/gateway-api/envoy-gateway/definition.yaml >/dev/null && echo OK` (or `python3 -c "import yaml,sys; yaml.safe_load(open('plugins/gateway-api/envoy-gateway/definition.yaml'))" && echo OK`)
Expected: `OK`.

---

## Task 9: console-ui scaffold

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/console-ui/package.json`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/tsconfig.json`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/vite.config.ts`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/src/sdk.ts`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/src/shared.ts`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/src/nldd-design-system.ts`

**Interfaces:**
- Produces: `loadSdk`, `loadNlddDesignSystem`, `escapeHtml`, `navigateToDetail`, `navigateBack` from `shared.ts`; the `window.fundament` SDK types from `sdk.ts`; `@nldd` element types from `nldd-design-system.ts`; a Vite build emitting `gateways-create.html` into `../console`.

- [ ] **Step 1: Copy the generic SDK + design-system type files verbatim from openfsc**

These are plugin-agnostic. Copy them unchanged:

```bash
cd /Users/chiel/projects/github.com/fundament-oss/fundament
mkdir -p plugins/gateway-api/envoy-gateway/console-ui/src
cp plugins/openfsc/console-ui/src/sdk.ts plugins/gateway-api/envoy-gateway/console-ui/src/sdk.ts
cp plugins/openfsc/console-ui/src/nldd-design-system.ts plugins/gateway-api/envoy-gateway/console-ui/src/nldd-design-system.ts
```

- [ ] **Step 2: Create a trimmed `shared.ts`**

The create form only needs SDK/design-system loading, escaping, and navigation — not the FSC list/detail renderers. Create `plugins/gateway-api/envoy-gateway/console-ui/src/shared.ts`:

```ts
// Shared helpers for the Envoy Gateway console views. The plugin ships only a
// Gateway create form, so this trims the openfsc shared.ts to loading, escaping
// and host navigation.

import type { FundamentSdk } from './sdk.ts';

function whenSettled(el: HTMLLinkElement | HTMLScriptElement, what: string): Promise<void> {
  return new Promise((resolve, reject) => {
    el.addEventListener('load', () => resolve(), { once: true });
    el.addEventListener('error', () => reject(new Error(`failed to load ${what}`)), { once: true });
  });
}

// Loads the plugin-proxy's /plugins/sdk/v1/<base>.{css,js} pair (same origin as
// the iframe under FUN-17). See docs/funs/FUN-18.adoc.
function loadPluginAsset(base: string): Promise<void> {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `/plugins/sdk/v1/${base}.css`;
  const css = whenSettled(link, `${base}.css`);
  document.head.appendChild(link);

  const script = document.createElement('script');
  script.src = `/plugins/sdk/v1/${base}.js`;
  const js = whenSettled(script, `${base}.js`);
  document.head.appendChild(script);

  return Promise.all([css, js]).then(() => undefined);
}

export function loadSdk(): Promise<FundamentSdk> {
  return loadPluginAsset('plugin-sdk').then(() => window.fundament);
}

function syncNlddDesignSystemTheme(): void {
  const dark = document.body.classList.contains('dark');
  document.documentElement.setAttribute('data-scheme', dark ? 'dark' : 'light');
}

export function loadNlddDesignSystem(): Promise<void> {
  syncNlddDesignSystemTheme();
  new MutationObserver(syncNlddDesignSystemTheme).observe(document.body, {
    attributes: true,
    attributeFilter: ['class'],
  });
  return loadPluginAsset('nldd-design-system');
}

export function escapeHtml(value: unknown): string {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function postToHost(message: unknown): void {
  window.parent.postMessage(message, window.fundament?.parentOrigin ?? '*');
}

export function navigateToDetail(
  name: string | null | undefined,
  namespace: string | null | undefined,
): void {
  postToHost({ type: 'plugin:navigate', name, namespace });
}

export function navigateBack(): void {
  postToHost({ type: 'plugin:navigate-back' });
}
```

- [ ] **Step 3: Create `package.json`**

```json
{
  "name": "gateway-api-envoy-console-ui",
  "version": "0.0.0",
  "private": true,
  "type": "module",
  "description": "Envoy Gateway plugin Console UI. Vite app (vanilla TS) built to ../console, served same-origin from the plugin's /console/. The NLDD Design System is loaded at runtime from the shared /plugin-ui/ bundle. See docs/funs/FUN-18.adoc.",
  "scripts": {
    "build": "vite build && touch ../console/.gitkeep",
    "dev": "vite",
    "test": "vitest run",
    "typecheck": "tsc --noEmit"
  },
  "devDependencies": {
    "@nldd/design-system": "0.8.63",
    "happy-dom": "^20.0.0",
    "typescript": "~5.9.2",
    "vite": "^7.1.0",
    "vitest": "^3.2.0"
  }
}
```

- [ ] **Step 4: Copy `tsconfig.json` verbatim from openfsc**

```bash
cp plugins/openfsc/console-ui/tsconfig.json plugins/gateway-api/envoy-gateway/console-ui/tsconfig.json
```

- [ ] **Step 5: Create `vite.config.ts` (single create entry)**

```ts
import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const entry = (name: string) => fileURLToPath(new URL(`${name}.html`, import.meta.url));

// The plugin serves this app same-origin under /console/, and console.go's
// go:embed console/* compiles the build output into the plugin binary. The NLDD
// Design System is NOT bundled: the app uses <nldd-*> tags whose registrations
// come from the shared /plugin-ui/nldd-design-system.js. See docs/funs/FUN-18.adoc.
export default defineConfig({
  root,
  base: './',
  build: {
    outDir: fileURLToPath(new URL('../console', import.meta.url)),
    emptyOutDir: true,
    rollupOptions: {
      // Output filename must match definition.yaml's customComponents.Gateway.create.
      input: {
        'gateways-create': entry('gateways-create'),
      },
    },
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
  },
});
```

- [ ] **Step 6: Install deps and verify the scaffold typechecks**

Run:
```bash
cd plugins/gateway-api/envoy-gateway/console-ui && bun install && bun run typecheck
```
Expected: `bun install` writes `bun.lock`; `typecheck` passes (no `src/*.ts` type errors; `form.ts`/`create.ts` don't exist yet, which is fine — `tsc` only checks `include: ["src"]` files that exist).

---

## Task 10: Gateway create-form logic (`form.ts`) with tests

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/console-ui/src/form.ts`
- Test: `plugins/gateway-api/envoy-gateway/console-ui/src/form.test.ts`

**Interfaces:**
- Produces: `type GatewayBody`; `function buildGatewayBody(root: ParentNode, namespace: string): GatewayBody`; `function trimmedValue(root, id): string`; `function isChecked(root, id): boolean`; `function validateForm(root: ParentNode): boolean`; `function namespaceFieldHtml(namespaces?: string[]): string`.
- Consumes: nothing outside the DOM (kept pure for testability). `create.ts` (Task 11) wires it to the SDK.

- [ ] **Step 1: Write the failing tests**

Create `plugins/gateway-api/envoy-gateway/console-ui/src/form.test.ts`:

```ts
// Tests for the Gateway create-form logic. The NLDD Design System is not loaded
// here, so <nldd-*> are unknown elements; the harness reflects declared
// attributes onto the same-named properties the form reads (.value / .checked),
// mirroring the part of Lit the form depends on.

import { beforeEach, describe, expect, it } from 'vitest';
import { buildGatewayBody, validateForm } from './form.ts';

function upgrade(root: ParentNode): void {
  root.querySelectorAll('nldd-text-field, nldd-checkbox-field').forEach((el) => {
    const v = el.getAttribute('value');
    if (v !== null) (el as unknown as { value: string }).value = v;
    const checked = el.hasAttribute('checked');
    (el as unknown as { checked: boolean }).checked = checked;
    if (el.hasAttribute('required')) (el as unknown as { required: boolean }).required = true;
  });
}

function renderForm(overrides: {
  name?: string;
  https?: boolean;
  tlsMode?: 'secret' | 'certManager';
  tlsSecret?: string;
  issuer?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <nldd-text-field id="name" required value="${overrides.name ?? 'web'}"></nldd-text-field>
      <nldd-checkbox-field id="https-enabled" ${overrides.https ? 'checked' : ''}></nldd-checkbox-field>
      <select id="tls-mode"><option value="secret" ${overrides.tlsMode === 'secret' ? 'selected' : ''}>secret</option><option value="certManager" ${overrides.tlsMode === 'certManager' ? 'selected' : ''}>certManager</option></select>
      <nldd-text-field id="tls-secret" value="${overrides.tlsSecret ?? ''}"></nldd-text-field>
      <nldd-text-field id="cluster-issuer" value="${overrides.issuer ?? ''}"></nldd-text-field>
    </form>`;
  const form = document.getElementById('form') as HTMLFormElement;
  upgrade(form);
  return form;
}

describe('buildGatewayBody', () => {
  it('builds an HTTP-only Gateway with the eg class and All allowedRoutes', () => {
    const body = buildGatewayBody(renderForm({ name: 'web', https: false }), 'team-a');

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1');
    expect(body.kind).toBe('Gateway');
    expect(body.metadata).toEqual({ name: 'web', namespace: 'team-a' });
    expect(body.spec.gatewayClassName).toBe('eg');
    expect(body.spec.listeners).toHaveLength(1);
    expect(body.spec.listeners[0]).toMatchObject({
      name: 'http',
      protocol: 'HTTP',
      port: 80,
      allowedRoutes: { namespaces: { from: 'All' } },
    });
    expect(body.metadata).not.toHaveProperty('annotations');
  });

  it('adds an HTTPS listener referencing a TLS secret', () => {
    const body = buildGatewayBody(
      renderForm({ https: true, tlsMode: 'secret', tlsSecret: 'web-tls' }),
      'team-a',
    );

    expect(body.spec.listeners).toHaveLength(2);
    expect(body.spec.listeners[1]).toMatchObject({
      name: 'https',
      protocol: 'HTTPS',
      port: 443,
      tls: { mode: 'Terminate', certificateRefs: [{ name: 'web-tls' }] },
    });
    expect(body.metadata).not.toHaveProperty('annotations');
  });

  it('adds a cert-manager cluster-issuer annotation and derives the secret name', () => {
    const body = buildGatewayBody(
      renderForm({ name: 'web', https: true, tlsMode: 'certManager', issuer: 'letsencrypt' }),
      'team-a',
    );

    expect((body.metadata as Record<string, unknown>).annotations).toEqual({
      'cert-manager.io/cluster-issuer': 'letsencrypt',
    });
    // Non-null assertion: HTTPS is enabled, so this listener always carries tls.
    expect(body.spec.listeners[1].tls!.certificateRefs).toEqual([{ name: 'web-tls' }]);
  });
});

describe('validateForm', () => {
  it('fails when the name is empty', () => {
    expect(validateForm(renderForm({ name: '' }))).toBe(false);
  });
  it('fails when HTTPS+secret is chosen but no secret name given', () => {
    expect(validateForm(renderForm({ https: true, tlsMode: 'secret', tlsSecret: '' }))).toBe(false);
  });
  it('fails when HTTPS+certManager is chosen but no issuer given', () => {
    expect(validateForm(renderForm({ https: true, tlsMode: 'certManager', issuer: '' }))).toBe(false);
  });
  it('passes for a valid HTTP-only form', () => {
    expect(validateForm(renderForm({ name: 'web', https: false }))).toBe(true);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd plugins/gateway-api/envoy-gateway/console-ui && bun run test`
Expected: FAIL — `buildGatewayBody`/`validateForm` not exported (module not found).

- [ ] **Step 3: Implement `form.ts`**

Create `plugins/gateway-api/envoy-gateway/console-ui/src/form.ts`:

```ts
// Gateway create-form logic, kept free of module-level DOM lookups so it is
// unit-testable: every function takes a form root rather than reaching for
// `document`. create.ts wires these to the real form and the SDK.

export interface Listener {
  name: string;
  protocol: 'HTTP' | 'HTTPS';
  port: number;
  allowedRoutes: { namespaces: { from: 'All' } };
  tls?: { mode: 'Terminate'; certificateRefs: { name: string }[] };
}

export interface GatewayBody {
  apiVersion: 'gateway.networking.k8s.io/v1';
  kind: 'Gateway';
  metadata: { name: string; namespace: string; annotations?: Record<string, string> };
  spec: { gatewayClassName: 'eg'; listeners: Listener[] };
}

// Reads a control's trimmed value by id. The control may be a native <select>,
// <input>, or an <nldd-text-field> — all expose `.value`.
export function trimmedValue(root: ParentNode, id: string): string {
  const el = root.querySelector(`#${id}`) as { value?: string } | null;
  return (el?.value ?? '').trim();
}

export function isChecked(root: ParentNode, id: string): boolean {
  const el = root.querySelector(`#${id}`) as { checked?: boolean } | null;
  return Boolean(el?.checked);
}

function httpListener(): Listener {
  return {
    name: 'http',
    protocol: 'HTTP',
    port: 80,
    allowedRoutes: { namespaces: { from: 'All' } },
  };
}

// httpsListener resolves the TLS secret name: for the cert-manager path the
// secret does not exist yet, so it is derived as "<gateway-name>-tls" (the
// convention cert-manager provisions from the Gateway annotation).
function httpsListener(root: ParentNode, name: string): Listener {
  const mode = trimmedValue(root, 'tls-mode');
  const secret = mode === 'certManager' ? `${name}-tls` : trimmedValue(root, 'tls-secret');
  return {
    name: 'https',
    protocol: 'HTTPS',
    port: 443,
    allowedRoutes: { namespaces: { from: 'All' } },
    tls: { mode: 'Terminate', certificateRefs: [{ name: secret }] },
  };
}

export function buildGatewayBody(root: ParentNode, namespace: string): GatewayBody {
  const name = trimmedValue(root, 'name');
  const listeners: Listener[] = [httpListener()];

  const metadata: GatewayBody['metadata'] = { name, namespace };

  if (isChecked(root, 'https-enabled')) {
    listeners.push(httpsListener(root, name));
    if (trimmedValue(root, 'tls-mode') === 'certManager') {
      metadata.annotations = { 'cert-manager.io/cluster-issuer': trimmedValue(root, 'cluster-issuer') };
    }
  }

  return {
    apiVersion: 'gateway.networking.k8s.io/v1',
    kind: 'Gateway',
    metadata,
    spec: { gatewayClassName: 'eg', listeners },
  };
}

export function validateForm(root: ParentNode): boolean {
  if (!trimmedValue(root, 'name')) return false;
  if (isChecked(root, 'https-enabled')) {
    const mode = trimmedValue(root, 'tls-mode');
    if (mode === 'certManager') return Boolean(trimmedValue(root, 'cluster-issuer'));
    return Boolean(trimmedValue(root, 'tls-secret'));
  }
  return true;
}

// Renders the namespace control: a dropdown of project namespaces when the host
// supplies them, else a free-text field (org-level route).
export function namespaceFieldHtml(namespaces?: string[]): string {
  if (namespaces && namespaces.length > 0) {
    const options = namespaces.map((n) => `<option value="${n}">${n}</option>`).join('');
    return `<nldd-form-field label="Namespace"><nldd-dropdown><select id="namespace" name="namespace" aria-label="Namespace">${options}</select></nldd-dropdown></nldd-form-field>`;
  }
  return `<nldd-form-field label="Namespace"><nldd-text-field id="namespace" name="namespace" required placeholder="default"></nldd-text-field></nldd-form-field>`;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd plugins/gateway-api/envoy-gateway/console-ui && bun run test`
Expected: PASS (all `buildGatewayBody` + `validateForm` cases).

- [ ] **Step 5: Typecheck**

Run: `cd plugins/gateway-api/envoy-gateway/console-ui && bun run typecheck`
Expected: PASS.

---

## Task 11: Gateway create HTML + `create.ts` wiring

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/console-ui/gateways-create.html`
- Create: `plugins/gateway-api/envoy-gateway/console-ui/src/create.ts`

**Interfaces:**
- Consumes: `buildGatewayBody`, `validateForm`, `namespaceFieldHtml` (Task 10); `loadSdk`, `loadNlddDesignSystem`, `navigateToDetail`, `navigateBack` (Task 9); `window.fundament.k8s.create` (SDK); `NlddButton` type (nldd-design-system.ts).

- [ ] **Step 1: Create the HTML view**

`plugins/gateway-api/envoy-gateway/console-ui/gateways-create.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Create Gateway</title>
  </head>
  <body class="light">
    <div class="plugin-card">
      <h1 class="plugin-heading">Create Gateway</h1>
      <p class="plugin-text" id="intro">Loading…</p>

      <form id="form" class="plugin-form" hidden>
        <div class="plugin-error" id="error" hidden></div>

        <nldd-form-field label="Name">
          <nldd-text-field
            id="name"
            name="name"
            required
            maxlength="63"
            pattern="[a-z0-9]([a-z0-9\-]*[a-z0-9])?"
            placeholder="web"
            error-message="name-error"
          ></nldd-text-field>
          <nldd-form-field-help-text>Lowercase letters, digits and dashes.</nldd-form-field-help-text>
          <nldd-form-field-error-text id="name-error">Enter a valid name.</nldd-form-field-error-text>
        </nldd-form-field>

        <!-- Replaced at runtime with a dropdown (project namespaces) or a text field. -->
        <div id="namespace-field"></div>

        <p class="plugin-hint">An HTTP listener on port 80 is always created. Enable HTTPS to add a TLS listener on port 443.</p>

        <nldd-checkbox-field id="https-enabled" name="https-enabled" label="Enable HTTPS (port 443)"></nldd-checkbox-field>

        <fieldset class="plugin-fieldset" id="https-fieldset" hidden>
          <legend class="plugin-legend">TLS</legend>
          <nldd-form-field label="Certificate source">
            <nldd-dropdown>
              <select id="tls-mode" name="tls-mode" aria-label="Certificate source">
                <option value="secret">Existing TLS secret</option>
                <option value="certManager">cert-manager (cluster issuer)</option>
              </select>
            </nldd-dropdown>
          </nldd-form-field>

          <nldd-form-field label="TLS secret name" id="tls-secret-field">
            <nldd-text-field id="tls-secret" name="tls-secret" placeholder="web-tls" error-message="tls-secret-error"></nldd-text-field>
            <nldd-form-field-help-text>Name of an existing kubernetes.io/tls Secret in the namespace.</nldd-form-field-help-text>
            <nldd-form-field-error-text id="tls-secret-error">This field is required.</nldd-form-field-error-text>
          </nldd-form-field>

          <nldd-form-field label="Cluster issuer" id="cluster-issuer-field" hidden>
            <nldd-text-field id="cluster-issuer" name="cluster-issuer" placeholder="letsencrypt" error-message="cluster-issuer-error"></nldd-text-field>
            <nldd-form-field-help-text>cert-manager provisions a &lt;name&gt;-tls Secret via this ClusterIssuer.</nldd-form-field-help-text>
            <nldd-form-field-error-text id="cluster-issuer-error">This field is required.</nldd-form-field-error-text>
          </nldd-form-field>
        </fieldset>

        <div class="plugin-actions">
          <nldd-button id="submit" type="button" variant="primary" text="Create Gateway"></nldd-button>
          <nldd-button id="back" type="button" variant="secondary" text="Back"></nldd-button>
        </div>
      </form>
    </div>

    <script type="module" src="/src/create.ts"></script>
  </body>
</html>
```

- [ ] **Step 2: Create `create.ts`**

`plugins/gateway-api/envoy-gateway/console-ui/src/create.ts`:

```ts
import { loadSdk, loadNlddDesignSystem, navigateToDetail, navigateBack } from './shared.ts';
import { buildGatewayBody, namespaceFieldHtml, trimmedValue, validateForm } from './form.ts';
import type { NlddButton } from './nldd-design-system.ts';
import type { InitContext } from './sdk.ts';

const intro = document.getElementById('intro') as HTMLElement;
const form = document.getElementById('form') as HTMLFormElement;
const errorBox = document.getElementById('error') as HTMLElement;
const submitButton = document.getElementById('submit') as NlddButton;

document.getElementById('back')!.addEventListener('click', () => navigateBack());

let ctx: InitContext | null;
try {
  await Promise.all([loadSdk(), loadNlddDesignSystem()]);
  ctx = await window.fundament.init;
} catch (err) {
  intro.textContent = `Failed to load the plugin SDK: ${err instanceof Error ? err.message : err}`;
  ctx = null;
}

if (ctx) {
  intro.textContent = 'Configure a Gateway. It uses the eg GatewayClass and accepts routes from all namespaces.';
  renderNamespaceControl(ctx.namespaces);
  wireHttpsToggle();
  wireTlsModeToggle();
  form.hidden = false;
}

function resyncDropdown(dropdown: HTMLElement | null): void {
  const apply = () =>
    dropdown?.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  (dropdown as (HTMLElement & { updateComplete?: Promise<unknown> }) | null)?.updateComplete?.then?.(apply);
  requestAnimationFrame(apply);
}

function renderNamespaceControl(namespaces: string[] | undefined): void {
  const field = document.getElementById('namespace-field') as HTMLElement;
  field.innerHTML = namespaceFieldHtml(namespaces);
  resyncDropdown(field.querySelector('nldd-dropdown'));
}

// Show the TLS fieldset only when HTTPS is enabled.
function wireHttpsToggle(): void {
  const checkbox = document.getElementById('https-enabled') as HTMLElement & { checked?: boolean };
  const fieldset = document.getElementById('https-fieldset') as HTMLElement;
  const apply = () => (fieldset.hidden = !checkbox.checked);
  checkbox.addEventListener('change', apply);
  apply();
}

// Swap the secret-name vs cluster-issuer field with the certificate source.
function wireTlsModeToggle(): void {
  const select = document.getElementById('tls-mode') as HTMLSelectElement;
  const secretField = document.getElementById('tls-secret-field') as HTMLElement;
  const issuerField = document.getElementById('cluster-issuer-field') as HTMLElement;
  const apply = () => {
    const certManager = select.value === 'certManager';
    secretField.hidden = certManager;
    issuerField.hidden = !certManager;
  };
  select.addEventListener('change', apply);
  apply();
}

// nldd-text-field's inner <input> is in shadow DOM and can't reach the light-DOM
// form, so route Enter to the submit button to restore native Enter-to-submit.
form.addEventListener('keydown', (e) => {
  const target = e.target as HTMLElement | null;
  if (e.key === 'Enter' && target?.tagName === 'NLDD-TEXT-FIELD') {
    e.preventDefault();
    submitButton.click();
  }
});

submitButton.addEventListener('click', async () => {
  if (submitButton.disabled) return;
  errorBox.hidden = true;
  if (!validateForm(form)) {
    errorBox.textContent = 'Please fill in the required fields.';
    errorBox.hidden = false;
    return;
  }

  submitButton.disabled = true;
  try {
    const namespace = trimmedValue(form, 'namespace');
    const body = buildGatewayBody(form, namespace);
    const created = await window.fundament.k8s.create<{ metadata?: { name?: string } }>(
      { group: 'gateway.networking.k8s.io', version: 'v1', resource: 'gateways', namespace },
      body,
    );
    navigateToDetail(created?.metadata?.name ?? body.metadata.name, namespace);
  } catch (err) {
    errorBox.textContent = `Failed to create: ${err instanceof Error ? err.message : err}`;
    errorBox.hidden = false;
    submitButton.disabled = false;
  }
});
```

- [ ] **Step 3: Typecheck, test, and build the console-ui**

Run:
```bash
cd plugins/gateway-api/envoy-gateway/console-ui && bun run typecheck && bun run test && bun run build
```
Expected: typecheck PASS; tests PASS; `bun run build` emits `plugins/gateway-api/envoy-gateway/console/gateways-create.html` (+ `assets/`). Confirm with `ls ../console`.

---

## Task 12: Dockerfile and .dockerignore

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/Dockerfile`
- Modify: `.dockerignore`

**Interfaces:**
- Produces: an image building the console-ui (bun) → embedding into the Go binary → runtime with `helm`.

- [ ] **Step 1: Create the Dockerfile**

`plugins/gateway-api/envoy-gateway/Dockerfile`:

```dockerfile
# Build the console UI (Vite, vanilla TS) → plugins/gateway-api/envoy-gateway/console,
# which console.go embeds via go:embed. The NLDD Design System is NOT bundled here:
# the app loads it at runtime from the Console's shared /plugin-ui bundle.
FROM oven/bun:1-alpine AS console-ui
WORKDIR /ui
COPY plugins/gateway-api/envoy-gateway/console-ui/package.json plugins/gateway-api/envoy-gateway/console-ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY plugins/gateway-api/envoy-gateway/console-ui/ ./
RUN bun run build

FROM golang:1.26.5 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overlay the freshly built console assets. The build context excludes the built
# console dir (.dockerignore), so this stage is the only source of the embedded UI.
COPY --from=console-ui /console ./plugins/gateway-api/envoy-gateway/console
RUN CGO_ENABLED=0 go build -o /bin/gateway-api-envoy ./plugins/gateway-api/envoy-gateway

FROM alpine:3.22

RUN apk add --no-cache helm

COPY --from=build /bin/gateway-api-envoy /usr/local/bin/gateway-api-envoy
COPY plugins/gateway-api/envoy-gateway/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["gateway-api-envoy"]
```

- [ ] **Step 2: Add .dockerignore entries**

Append to `.dockerignore`:

```
plugins/gateway-api/envoy-gateway/console-ui/node_modules/
# Built console UI is produced by the console-ui build stage and COPY'd in, so
# exclude the local build output from the context (Docker rebuilds it).
plugins/gateway-api/envoy-gateway/console/
```

- [ ] **Step 3: Verify the image builds**

Run (from repo root):
```bash
docker build --provenance=false --sbom=false -t gateway-api-envoy:plan-check -f plugins/gateway-api/envoy-gateway/Dockerfile .
```
Expected: build succeeds through all three stages (console-ui build, go build, runtime).

---

## Task 13: Justfile module + sandbox verification recipe

**Files:**
- Create: `plugins/gateway-api/envoy-gateway/Justfile`
- Modify: `plugins/Justfile`

**Interfaces:**
- Produces: `just envoy-gateway::test` / `::test-cleanup` and console-ui `::typecheck`/`::ui-test` recipes.

- [ ] **Step 1: Create the plugin Justfile**

`plugins/gateway-api/envoy-gateway/Justfile`:

```just
kube_context := env("kube_context", "k3d-fundament-plugin")

# Typecheck the console UI.
typecheck:
    cd console-ui && bun install && bun run typecheck

# Run the console UI unit tests.
ui-test:
    cd console-ui && bun install && bun run test

# Verify the install: PluginInstallation Running, the eg GatewayClass Accepted,
# and a sample Gateway is programmed by the controller.
test:
    #!/usr/bin/env bash
    set -euo pipefail
    k="kubectl --context {{ kube_context }}"
    echo "Waiting for PluginInstallation/gateway-api-envoy to be Running (first install pulls images)..."
    $k wait --for=jsonpath='{.status.phase}'=Running plugininstallation/gateway-api-envoy --timeout=600s
    echo "Waiting for the eg GatewayClass to be Accepted..."
    $k wait --for=jsonpath='{.status.conditions[?(@.type=="Accepted")].status}'=True gatewayclass/eg --timeout=120s
    echo "Creating a sample Gateway..."
    $k create namespace eg-demo --dry-run=client -o yaml | $k apply -f -
    $k apply -f - <<'EOF'
    apiVersion: gateway.networking.k8s.io/v1
    kind: Gateway
    metadata:
      name: demo
      namespace: eg-demo
    spec:
      gatewayClassName: eg
      listeners:
        - name: http
          protocol: HTTP
          port: 80
          allowedRoutes:
            namespaces:
              from: All
    EOF
    echo "Waiting for the Gateway to be Programmed..."
    $k wait --for=jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'=True gateway/demo -n eg-demo --timeout=300s
    $k get gateway,gatewayclass -A

# Remove the sample Gateway and its namespace.
test-cleanup:
    kubectl --context {{ kube_context }} delete gateway/demo -n eg-demo --ignore-not-found
    kubectl --context {{ kube_context }} delete namespace eg-demo --ignore-not-found
```

- [ ] **Step 2: Register the module in the plugins Justfile**

In `plugins/Justfile`, add alongside the existing `mod` lines (`mod cert-manager` / `mod openfsc`):

```just
mod envoy-gateway 'gateway-api/envoy-gateway'
```

- [ ] **Step 3: Verify Just recognizes the module**

Run: `cd plugins && just --list envoy-gateway`
Expected: lists `typecheck`, `ui-test`, `test`, `test-cleanup`.

---

## Task 14: End-to-end sandbox verification (manual)

**Files:** none (verification only).

- [ ] **Step 1: Ensure the sandbox cluster is up**

Run:
```bash
cd plugins && just cluster-start && just deploy
```
Expected: k3d cluster running; plugin-controller deployed.

- [ ] **Step 2: Publish + install the Envoy Gateway plugin**

Set the publish env vars (`PLUGIN_REGISTRY`, `FUNDAMENT_ORG_API_URL`, `FUNDAMENT_ORGANIZATION_ID`, `FUNDAMENT_TOKEN` — see `plugins/Justfile` `plugin-publish`), then:
```bash
cd plugins && just plugin-publish gateway-api/envoy-gateway
```
Then create a `PluginInstallation` referencing the published `gateway-api-envoy` definition (mirror how other plugins are installed in the sandbox; the old `plugin-install` recipe was removed).

- [ ] **Step 3: Run the verification recipe**

Run:
```bash
cd plugins && just envoy-gateway::test
```
Expected: PluginInstallation Running; `gatewayclass/eg` Accepted=True; sample `gateway/demo` Programmed=True.

- [ ] **Step 4: Verify the console surface**

In the console: the plugin's project menu lists Gateway, the four routes, and the three policies (list/detail render). Opening **Create** on Gateway shows the guided form; submitting creates a Gateway via the SDK and navigates to its detail. Confirm the created Gateway appears in `kubectl get gateway -A`.

- [ ] **Step 5: Clean up**

Run:
```bash
cd plugins && just envoy-gateway::test-cleanup
```

---

## Task 15: Final commit (after user approval)

- [ ] **Step 1: Show the full diff and get explicit approval**

Run: `git status && git diff --stat`
Then ask the user to approve committing. **Do not commit without explicit approval.**

- [ ] **Step 2: Commit once (no Co-Authored-By trailer)**

```bash
git add -A
git commit -m "feat: add Envoy Gateway plugin and split gateway-api into istio/envoy-gateway"
```

---

## Self-Review Notes

- **Spec coverage:** restructure+rename (T1) ✓; `InstallFromOCI` helper (T2) ✓; platform-only install/ensure GatewayClass/health/reconcile/uninstall guard (T5–T6) ✓; operator-only config (T3) ✓; CRD-verify list incl. Envoy CRDs (T6) ✓; definition RBAC/menu/customComponents/allowedResources/uiHints (T8) ✓; generic list/detail via menu (T8) ✓; custom guided Gateway create form incl. HTTP-only/HTTPS-secret/cert-manager (T9–T11) ✓; Dockerfile+build (T12) ✓; Justfile+sandbox verify (T13–T14) ✓; Go+console-ui tests throughout ✓.
- **Deferred per spec (out of v1 scope):** custom create forms for routes/policies; EnvoyProxy menu surface and per-Gateway service-type UI. Not tasked — intentional.
- **Verify during execution:** the `pluginruntime.Plugin`/`Host` interface method set and `crd.VerifyAll`/`pluginerrors` signatures against `plugins/gateway-api/istio/plugin.go` (noted in T6); the exact `@nldd/design-system` version and `oven/bun`/`golang`/`alpine` base tags against openfsc in case they have advanced since this plan was written.
