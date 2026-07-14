package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
)

// Asset endpoint is unauthenticated and returns the full body to the caller,
// so cap both wall-clock and buffered size defensively against a hostile or
// misbehaving upstream pod.
const (
	fetchTimeout = 30 * time.Second
	maxAssetSize = 32 << 20 // 32 MiB
)

// ErrAssetTooLarge is returned when the upstream body exceeds maxAssetSize.
var ErrAssetTooLarge = errors.New("asset exceeds max size")

// PodFetcher fetches asset files from a plugin runtime pod via the target
// cluster's API-server service proxy. The handler resolves clusterID before
// calling; PodFetcher just builds the URL and forwards the request.
type PodFetcher struct {
	AdminKubeconfig *kube.AdminKubeconfigCache
}

func (f *PodFetcher) Fetch(ctx context.Context, clusterID uuid.UUID, pluginName, _ /* pluginVersion */, assetPath string) ([]byte, string, error) {
	transport, host, err := f.AdminKubeconfig.HTTPClientFor(ctx, clusterID.String())
	if err != nil {
		return nil, "", fmt.Errorf("admin kubeconfig: %w", err)
	}

	// plugin-controller names both the namespace and the Service `plugin-<name>`
	// (see plugin-controller/pkg/controller/reconciler.go). The service exposes
	// port 8080; kube-api-proxy's `http:name:port` selector picks the http
	// scheme regardless of TLS on the API server.
	escaped := url.PathEscape(pluginName)
	ns := "plugin-" + escaped
	svc := "http:plugin-" + escaped + ":8080"
	asset := (&url.URL{Path: assetPath}).EscapedPath()
	upstream := fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s/proxy/console/%s", host, ns, svc, asset)
	//nolint:gosec // host comes from the trusted admin kubeconfig cache; pluginName and assetPath are URL-escaped above.
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
