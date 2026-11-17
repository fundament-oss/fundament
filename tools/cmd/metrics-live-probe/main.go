// Command metrics-live-probe verifies the per-shoot Prometheus path
// (ADR-0026) against a real shoot, going through the SAME production code
// path org-api uses: gardener.RealClient.Monitoring →
// prometheus.NewHTTPClientWithAuth on the prometheus-url annotation.
//
// On-demand tooling for openspec change metrics-per-shoot-prometheus; not
// part of any build or CI.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
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
	fmt.Printf("plutono-url:    %s\nprometheus-url: %s\nusername: %s (password: %d chars)\n\n", info.URL, info.PrometheusURL, info.Username, len(info.Password))
	if info.PrometheusURL == "" {
		fatal("prometheus-url", fmt.Errorf("annotation missing — per-shoot metrics cannot work on this Gardener version"))
	}

	// Probe 4: does the DEFAULT transport (what production uses) verify TLS?
	fmt.Println("== probe 4: TLS with default transport ==")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.PrometheusURL, http.NoBody) //nolint:gosec // probe target is operator-supplied
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)                             //nolint:gosec // probe target is operator-supplied
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
	fmt.Printf("== probe 4b: TLS with production trust (shoot CA: %d bytes) ==\n", len(info.CABundle))
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, info.PrometheusURL, http.NoBody) //nolint:gosec // probe target is operator-supplied
	resp, err = (&http.Client{Timeout: 15 * time.Second, Transport: transport}).Do(req)       //nolint:gosec // probe target is operator-supplied
	fatal("tls: neither the shoot CA nor PROMETHEUS_CA_FILE verifies "+info.PrometheusURL, err)
	_ = resp.Body.Close()
	fmt.Printf("production trust OK (status %d)\n\n", resp.StatusCode)
	opts := []prom.Option{prom.WithTransport(transport)}
	client := prom.NewHTTPClientWithAuth(info.PrometheusURL, info.Username, info.Password, opts...)
	now := time.Now()

	fmt.Println("== probe 1: auth model ==")
	if _, err := prom.NewHTTPClientWithAuth(info.PrometheusURL, "wrong", "creds", opts...).Query(ctx, "up", now); err != nil {
		fmt.Printf("  wrong creds rejected: %v\n", err)
	} else {
		fmt.Println("  FINDING: wrong creds ACCEPTED — auth not enforced?!")
	}
	if _, err := client.Query(ctx, "up", now); err != nil {
		fmt.Printf("  FINDING: monitoring creds rejected: %v\n", err)
	} else {
		fmt.Println("  monitoring-secret creds accepted")
	}

	fmt.Println("\n== probe 2: metric availability (via production client) ==")
	// Only the shoot's own series (type="shoot"): the control plane federated
	// from the seed (type="seed") never reaches the console.
	metrics := []string{
		"node_cpu_seconds_total",
		"node_memory_MemAvailable_bytes",
		"kubelet_running_pods",
		"kube_node_info",
		"kube_node_status_capacity",
		"container_network_receive_bytes_total",
		"container_network_transmit_bytes_total",
	}
	for _, m := range metrics {
		samples, err := client.Query(ctx, "count("+m+`{type="shoot"})`, now)
		if err != nil {
			fmt.Printf("  %-42s ERROR: %v\n", m, err)
			continue
		}
		n := float64(0)
		if len(samples) > 0 {
			n = samples[0].Value
		}
		fmt.Printf("  %-42s %v series\n", m, n)
	}
	nodeSamples, err := client.Query(ctx, `count by (node) (kube_node_status_capacity{type="shoot",resource="cpu"})`, now)
	fatal("node label probe", err)
	fmt.Printf("  node label survives: %d node(s)", len(nodeSamples))
	for _, s := range nodeSamples {
		fmt.Printf(" [node=%s]", s.Labels["node"])
	}
	fmt.Println()

	// Copies of organization-api's queries (pkg/organization/metrics.go); keep in step.
	fmt.Println("\n== probe 2b: dashboard query set (org-api queries) ==")
	instant := map[string]string{
		"cpu used":     `sum(rate(node_cpu_seconds_total{type="shoot",mode!~"idle|iowait|steal"}[5m]))`,
		"cpu total":    `sum(kube_node_status_capacity{type="shoot",resource="cpu"})`,
		"mem used":     `sum(node_memory_MemTotal_bytes{type="shoot"} - node_memory_MemAvailable_bytes{type="shoot"})`,
		"pods used":    `sum(max by (instance) (kubelet_running_pods{type="shoot"}))`,
		"pods by node": `max by (node) (label_replace(kubelet_running_pods{type="shoot"}, "internal_ip", "$1", "instance", "(.+)") * on (internal_ip) group_left (node) topk by (internal_ip) (1, kube_node_info{type="shoot"}))`,
		"net rx":       `sum(rate(container_network_receive_bytes_total{type="shoot",host_network="true",interface!="tunl0"}[5m]))`,
	}
	for label, q := range instant {
		samples, err := client.Query(ctx, q, now)
		if err != nil {
			fmt.Printf("  %-12s ERROR: %v\n", label, err)
			continue
		}
		if len(samples) == 0 {
			fmt.Printf("  %-12s empty\n", label)
			continue
		}
		fmt.Printf("  %-12s %.4g (%d series)\n", label, samples[0].Value, len(samples))
	}
	series, err := client.QueryRange(ctx, `sum(rate(node_cpu_seconds_total{type="shoot",mode!~"idle|iowait|steal"}[5m]))`, now.Add(-time.Hour), now, 5*time.Minute)
	switch {
	case err != nil:
		fmt.Printf("  range 1h    ERROR: %v\n", err)
	case len(series) > 0:
		fmt.Printf("  range 1h    %d points\n", len(series[0].Samples))
	default:
		fmt.Println("  range 1h    empty")
	}

	fmt.Println("\n== probe 5: load sanity (21 concurrent queries = one dashboard load) ==")
	startT := time.Now()
	errs := 0
	done := make(chan error, 21)
	for range 21 {
		go func() {
			_, err := client.Query(ctx, `sum(kube_node_status_capacity{type="shoot",resource="cpu"})`, now)
			done <- err
		}()
	}
	for range 21 {
		if err := <-done; err != nil {
			errs++
		}
	}
	fmt.Printf("  21 queries in %v, %d errors\n", time.Since(startT), errs)
}

func fatal(what string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", what, err)
		os.Exit(1)
	}
}
