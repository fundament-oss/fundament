package assets

import (
	"context"
	"fmt"
	"io"
	"net/http"

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
	url := fmt.Sprintf("%s/api/v1/namespaces/plugin-%s/services/runtime:8080/proxy/console/%s",
		host, pluginName, assetPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = guessContentType(assetPath)
	}
	return body, ct, nil
}
