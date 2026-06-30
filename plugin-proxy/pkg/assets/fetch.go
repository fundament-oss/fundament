package assets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
)

// PodFetcher fetches asset files from a plugin runtime pod via the target
// cluster's API-server service proxy. The handler resolves clusterID before
// calling; PodFetcher just builds the URL and forwards the request.
type PodFetcher struct {
	AdminKubeconfig *kube.AdminKubeconfigCache
}

func (f *PodFetcher) Fetch(ctx context.Context, clusterID uuid.UUID, pluginName, assetPath string) ([]byte, string, error) {
	transport, host, err := f.AdminKubeconfig.HTTPClientFor(ctx, clusterID.String())
	if err != nil {
		return nil, "", fmt.Errorf("admin kubeconfig: %w", err)
	}

	ns := "plugin-" + url.PathEscape(pluginName)
	asset := (&url.URL{Path: assetPath}).EscapedPath()
	upstream := fmt.Sprintf("%s/api/v1/namespaces/%s/services/runtime:8080/proxy/console/%s", host, ns, asset)
	//nolint:gosec // host comes from the trusted admin kubeconfig cache; pluginName and assetPath are URL-escaped above.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}

	//nolint:gosec // same provenance as the request above — host and path are sanitized at construction.
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch asset: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = guessContentType(assetPath)
	}

	return body, ct, nil
}
