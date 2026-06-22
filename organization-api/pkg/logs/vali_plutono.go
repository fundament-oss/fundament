package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValiDatasourceName is the name of the Vali datasource provisioned in each
// shoot's Plutono. It is used to resolve the datasource uid for the proxy route.
//
// NOTE: the provisioned name may differ by Gardener version (e.g. "vali" vs
// "Vali") — verify against a live Plutono (plan Step 0).
const ValiDatasourceName = "vali"

// ResolveValiProxyBase returns the base URL through which a shoot's Vali can be
// queried via its Plutono datasource proxy, e.g.
//
//	https://<plutono-host>/api/datasources/proxy/uid/<uid>
//
// Gardener does not expose Vali directly; it is reachable only as a Plutono
// datasource (https://gardener.cloud/docs/getting-started/observability/), so we
// look the datasource up by name to discover its uid. The caller (LokiClient)
// appends the Loki API paths ("/loki/api/v1/...") to the returned base.
//
// plutonoURL is the per-shoot Plutono ingress URL and username/password the
// basic-auth credentials — both from the "<shoot>.monitoring" secret.
func ResolveValiProxyBase(ctx context.Context, plutonoURL, datasource, username, password string) (string, error) {
	base := strings.TrimRight(plutonoURL, "/")
	lookupURL := base + "/api/datasources/name/" + url.PathEscape(datasource)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("lookup vali datasource: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("lookup vali datasource %q: status %d", datasource, resp.StatusCode)
	}

	var ds struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ds); err != nil {
		return "", fmt.Errorf("decode datasource: %w", err)
	}
	if ds.UID == "" {
		return "", fmt.Errorf("vali datasource %q has no uid", datasource)
	}
	return base + "/api/datasources/proxy/uid/" + ds.UID, nil
}
