// Command logs-live-probe verifies the per-shoot Vali path (ADR-0027)
// against a real shoot, going through the SAME production code path org-api
// uses: gardener.RealClient.Monitoring → logs.DiscoverValiProxyBase →
// logs.NewLokiClientWithAuth on the Plutono datasource proxy.
//
// On-demand tooling for openspec change logs-per-shoot-vali; not part of any
// build or CI.
//
// Runbook:
//
//	GARDENER_KUBECONFIG=<virtual-garden kubeconfig> \
//	PROBE_CLUSTER_ID=<fundament cluster uuid> \
//	PROMETHEUS_CA_FILE=<optional extra PEM bundle; the shoot's own CA is read automatically> \
//	go run ./tools/cmd/logs-live-probe
//
// Expected on a healthy shoot: probe 2 shows exactly one id answering
// vali-labels=200 and the admin API 403; probe 4 lists namespaces incl.
// kube-system and non-empty origin values (task 6.4); probe 5 returns real
// system log lines. Labels and queries are TIME-SCOPED — an 1h window on a
// quiet shoot is legitimately empty, which is why the probe uses 48h.
// Wrong-creds rejection in probe 3 must produce a 401, not a scan-through.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"log/slog"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
)

func main() {
	kubeconfig := os.Getenv("GARDENER_KUBECONFIG")
	clusterID := uuid.MustParse(os.Getenv("PROBE_CLUSTER_ID"))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("== probe 0: gardener.Monitoring (production lookup path) ==")
	gc, err := gardener.NewReal(kubeconfig, logger)
	fatal("gardener client", err)
	info, err := gc.Monitoring(ctx, clusterID)
	fatal("Monitoring()", err)
	fmt.Printf("plutono-url: %s\nusername: %s (password: %d chars)\n\n", info.URL, info.Username, len(info.Password))
	if info.URL == "" {
		fatal("plutono-url", fmt.Errorf("annotation missing — per-shoot logs cannot work on this Gardener version"))
	}

	fmt.Println("== probe 1: TLS with default transport ==")
	// The URL comes from the shoot's monitoring secret — reaching
	// operator-supplied endpoints is this probe's entire purpose.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, http.NoBody) //nolint:gosec // probe target is operator-supplied
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)                   //nolint:gosec // probe target is operator-supplied
	if err != nil {
		fmt.Printf("FINDING: default transport fails: %v\n(expected: the ingress cert is signed by this shoot's own CA)\n\n", err)
	} else {
		_ = resp.Body.Close()
		fmt.Printf("default transport OK (status %d)\n\n", resp.StatusCode)
	}

	// Same trust model as production; no insecure fallback.
	trust, err := catrust.New(os.Getenv("PROMETHEUS_CA_FILE"))
	fatal("PROMETHEUS_CA_FILE", err)
	transport := trust.TransportFor(info.CABundle)
	fmt.Printf("== probe 1b: TLS with production trust (shoot CA: %d bytes) ==\n", len(info.CABundle))
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, info.URL, http.NoBody)     //nolint:gosec // probe target is operator-supplied
	resp, err = (&http.Client{Timeout: 15 * time.Second, Transport: transport}).Do(req) //nolint:gosec // probe target is operator-supplied
	fatal("tls: neither the shoot CA nor PROMETHEUS_CA_FILE verifies "+info.URL, err)
	_ = resp.Body.Close()
	fmt.Printf("production trust OK (status %d)\n\n", resp.StatusCode)
	opts := []logs.Option{logs.WithTransport(transport)}

	fmt.Println("== probe 2: datasource inventory (proxy ids 1..5) ==")
	httpClient := &http.Client{Timeout: 15 * time.Second, Transport: transport}
	for id := 1; id <= 5; id++ {
		vali := rawStatus(ctx, httpClient, info, "/api/datasources/proxy/"+strconv.Itoa(id)+"/vali/api/v1/labels")
		promStatus := rawStatus(ctx, httpClient, info, "/api/datasources/proxy/"+strconv.Itoa(id)+"/api/v1/status/buildinfo")
		fmt.Printf("  id=%d vali-labels=%d prom-buildinfo=%d\n", id, vali, promStatus)
	}
	fmt.Printf("  admin API (must be 403): /api/datasources=%d\n\n", rawStatus(ctx, httpClient, info, "/api/datasources"))

	fmt.Println("== probe 3: discovery + auth model (production path) ==")
	if _, err := logs.DiscoverValiProxyBase(ctx, info.URL, "wrong", "creds", opts...); err != nil {
		fmt.Printf("  wrong creds rejected: %v\n", err)
	} else {
		fmt.Println("  FINDING: wrong creds ACCEPTED — auth not enforced?!")
	}
	base, err := logs.DiscoverValiProxyBase(ctx, info.URL, info.Username, info.Password, opts...)
	fatal("DiscoverValiProxyBase", err)
	fmt.Printf("  discovered base: %s\n\n", base)
	client := logs.NewLokiClientWithAuth(base, info.Username, info.Password, opts...)

	end := time.Now()
	start := end.Add(-48 * time.Hour)

	fmt.Println("== probe 4: labels over 48h (time-scoped!) ==")
	labels, err := client.Labels(ctx, clusterID.String(), "", start, end)
	fatal("Labels", err)
	fmt.Printf("  namespaces: %v\n  pods: %d values\n  containers: %d values\n", labels.Namespaces, len(labels.Pods), len(labels.Containers))
	origins := rawLabelValues(ctx, httpClient, info, base+"/vali/api/v1/label/origin/values", start, end)
	fmt.Printf("  origin values (for the default-view question, task 6.4): %v\n\n", origins)

	fmt.Println("== probe 5: query_range over 48h (production client) ==")
	entries, err := client.Query(ctx, &logs.QueryParams{ClusterID: clusterID.String(), Start: start, End: end, Limit: 5})
	fatal("Query", err)
	fmt.Printf("  %d entries (limit 5)\n", len(entries))
	for i := range entries {
		e := &entries[i]
		fmt.Printf("  %s [%s] %s/%s/%s: %.100s\n", e.Timestamp.Format(time.RFC3339), e.Level, e.Namespace, e.Pod, e.Container, e.Message)
	}

	fmt.Println("\n== probe 6: 30s tail (polling) ==")
	tailCtx, tailCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tailCancel()
	ch, err := client.Tail(tailCtx, &logs.QueryParams{ClusterID: clusterID.String()})
	fatal("Tail", err)
	tailed := 0
	for range ch {
		tailed++
	}
	fmt.Printf("  %d entries tailed in 30s\n", tailed)
}

func rawStatus(ctx context.Context, c *http.Client, info *gardener.MonitoringInfo, path string) int {
	// The target URL comes from the shoot's monitoring secret — reaching
	// operator-supplied endpoints is this probe's entire purpose.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL+path, http.NoBody) //nolint:gosec // probe target is operator-supplied
	if err != nil {
		return -1
	}
	req.SetBasicAuth(info.Username, info.Password)
	resp, err := c.Do(req) //nolint:gosec // probe target is operator-supplied
	if err != nil {
		return -1
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func rawLabelValues(ctx context.Context, c *http.Client, info *gardener.MonitoringInfo, rawURL string, start, end time.Time) []string {
	u := fmt.Sprintf("%s?start=%d&end=%d", rawURL, start.UnixNano(), end.UnixNano())
	// Same as rawStatus: the URL is the probe's operator-supplied target.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody) //nolint:gosec // probe target is operator-supplied
	if err != nil {
		return nil
	}
	req.SetBasicAuth(info.Username, info.Password)
	resp, err := c.Do(req) //nolint:gosec // probe target is operator-supplied
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return out.Data
}

func fatal(what string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", what, err)
		os.Exit(1)
	}
}
