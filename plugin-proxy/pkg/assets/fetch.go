package assets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/kubename"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
)

// Asset endpoint is unauthenticated and returns the full body to the caller,
// so cap both wall-clock and buffered size defensively against a hostile or
// misbehaving upstream pod.
const (
	fetchTimeout = 30 * time.Second
	maxAssetSize = 32 << 20 // 32 MiB
	maxCRSize    = 1 << 20  // 1 MiB — a PluginInstallation CR is tiny.
)

// ErrAssetTooLarge is returned when the upstream body exceeds maxAssetSize.
var ErrAssetTooLarge = errors.New("asset exceeds max size")

// ErrVersionMismatch is returned when the URL's {version} segment does not
// match the installed PluginInstallation's spec.definitionRef.pluginVersion.
// The handler maps it to 404 so a stale/rolled-back version URL neither serves
// mismatched bytes nor leaks which versions exist.
var ErrVersionMismatch = errors.New("plugin version does not match installed version")

// ErrInstallationNotFound is returned when no PluginInstallation named
// pluginName exists on the target cluster. The handler maps it to 404.
var ErrInstallationNotFound = errors.New("plugin installation not found")

// PodFetcher fetches asset files from a plugin runtime pod via the target
// cluster's API-server service proxy. The handler resolves clusterID before
// calling; PodFetcher just builds the URL and forwards the request.
type PodFetcher struct {
	AdminKubeconfig kube.KubeconfigSource
}

func (f *PodFetcher) Fetch(ctx context.Context, clusterID uuid.UUID, pluginName, pluginVersion, assetPath string) ([]byte, string, error) {
	transport, host, err := f.AdminKubeconfig.HTTPClientFor(ctx, clusterID.String())
	if err != nil {
		return nil, "", fmt.Errorf("admin kubeconfig: %w", err)
	}

	// Verify the URL's {version} against the installed CR before serving. This
	// keeps a given versioned URL content-stable — so the handler can cache it
	// immutably — and stops a rolled-back / stale version URL from silently
	// serving the currently-running pod's (different-version) bytes.
	installed, err := f.installedVersion(ctx, transport, host, pluginName)
	if err != nil {
		return nil, "", err
	}
	if installed != pluginVersion {
		return nil, "", fmt.Errorf("%w: url %q, installed %q", ErrVersionMismatch, pluginVersion, installed)
	}

	ns, svc := upstreamTarget(pluginName)
	asset := (&url.URL{Path: assetPath}).EscapedPath()
	upstream := fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s/proxy/console/%s", host, ns, svc, asset)
	//nolint:gosec // host comes from the trusted admin kubeconfig cache; assetPath is URL-escaped above.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}

	//nolint:gosec // same provenance as the request above — host and path are sanitized at construction.
	resp, err := (&http.Client{Transport: transport, Timeout: fetchTimeout}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch asset: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	if len(body) > maxAssetSize {
		return nil, "", ErrAssetTooLarge
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = guessContentType(assetPath)
	}

	return body, ct, nil
}

// upstreamTarget returns the namespace and the kube service-proxy selector for an
// installation. plugin-controller names the Service "plugin" inside the plugin's
// namespace; the `http:name:port` selector picks the http scheme regardless of TLS
// on the API server.
func upstreamTarget(installationName string) (namespace, service string) {
	return kubename.PluginNamespace(installationName), "http:" + kubename.PluginChildName + ":8080"
}

// installedVersion reads the cluster-scoped PluginInstallation named pluginName
// and returns its spec.definitionRef.pluginVersion. The CR name matches the
// plugin's child-resource name (plugin-controller derives both), which is what
// the asset URL now carries. Returns ErrInstallationNotFound when the CR is
// absent.
func (f *PodFetcher) installedVersion(ctx context.Context, transport http.RoundTripper, host, pluginName string) (string, error) {
	crURL := fmt.Sprintf("%s/apis/plugins.fundament.io/v1/plugininstallations/%s", host, url.PathEscape(pluginName))
	//nolint:gosec // host comes from the trusted admin kubeconfig cache; pluginName is URL-escaped.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, crURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build PluginInstallation request: %w", err)
	}

	//nolint:gosec // same provenance as the request above.
	resp, err := (&http.Client{Transport: transport, Timeout: fetchTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("get PluginInstallation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrInstallationNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get PluginInstallation: upstream returned %s", resp.Status)
	}

	var cr struct {
		Spec struct {
			DefinitionRef struct {
				PluginVersion string `json:"pluginVersion"`
			} `json:"definitionRef"`
		} `json:"spec"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCRSize)).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode PluginInstallation: %w", err)
	}

	return cr.Spec.DefinitionRef.PluginVersion, nil
}
