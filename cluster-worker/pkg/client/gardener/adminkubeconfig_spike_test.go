package gardener_test

import (
	"context"
	"fmt"
	"log/slog"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// TestSpike_AdminKubeconfigRequest validates that we can:
// 1. Call AdminKubeconfigRequest on the Gardener API to get a short-lived admin kubeconfig
// 2. Use that kubeconfig to make API calls to the shoot's kube-apiserver
//
// Prerequisites:
// - Local Gardener running: `just local-gardener`
// - At least one shoot cluster in ready state
// - GARDENER_KUBECONFIG env var pointing to Gardener kubeconfig
//
// Run with:
//
//	SPIKE_TEST=1 GARDENER_KUBECONFIG=<path> SHOOT_API_IP=172.18.255.1 \
//	  go test -v -run TestSpike_AdminKubeconfigRequest ./cluster-worker/pkg/client/gardener/
func TestSpike_AdminKubeconfigRequest(t *testing.T) {
	if os.Getenv("SPIKE_TEST") == "" {
		t.Skip("skipping spike test: set SPIKE_TEST=1 to run")
	}

	kubeconfigPath := os.Getenv("GARDENER_KUBECONFIG")
	if kubeconfigPath == "" {
		t.Fatal("GARDENER_KUBECONFIG must be set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider := gardener.NewProviderConfig()

	client, err := gardener.NewReal(kubeconfigPath, provider, logger)
	if err != nil {
		t.Fatalf("failed to create Gardener client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Find an existing shoot
	shoots, err := client.ListShoots(ctx)
	if err != nil {
		t.Fatalf("failed to list shoots: %v", err)
	}
	if len(shoots) == 0 {
		t.Fatal("no shoots found — create a cluster first via the API")
	}

	shoot := shoots[0]
	t.Logf("Using shoot: %s/%s (cluster ID: %s)", shoot.Namespace, shoot.Name, shoot.ClusterID)

	// Task 0.1: Request AdminKubeconfig
	t.Run("0.1_AdminKubeconfigRequest", func(t *testing.T) {
		adminKC, err := client.RequestAdminKubeconfig(ctx, shoot.ClusterID, 600) // 10 min
		if err != nil {
			t.Fatalf("AdminKubeconfigRequest failed: %v", err)
		}

		if len(adminKC.Kubeconfig) == 0 {
			t.Fatal("admin kubeconfig is empty")
		}
		t.Logf("Got admin kubeconfig (%d bytes), expires at: %s", len(adminKC.Kubeconfig), adminKC.ExpiresAt)

		// Task 0.2: Use the admin kubeconfig to make an API call to the shoot
		t.Run("0.2_ShootAPICall", func(t *testing.T) {
			restConfig, err := clientcmd.RESTConfigFromKubeConfig(adminKC.Kubeconfig)
			if err != nil {
				t.Fatalf("failed to parse admin kubeconfig: %v", err)
			}

			// Task 0.5: Document the shoot's API server URL
			t.Logf("Shoot API server URL: %s", restConfig.Host)

			// In local dev, the shoot DNS name doesn't resolve on the host.
			// Use SHOOT_API_IP to route traffic to the istio ingress gateway IP
			// while preserving the original hostname for SNI-based routing.
			if shootIP := os.Getenv("SHOOT_API_IP"); shootIP != "" {
				u, err := url.Parse(restConfig.Host)
				if err != nil {
					t.Fatalf("failed to parse shoot URL: %v", err)
				}
				originalHost := u.Hostname()
				port := u.Port()
				if port == "" {
					port = "443"
				}

				// Build TLS config from the rest config's CA and client certs.
				tlsConfig := &tls.Config{
					ServerName: originalHost,
				}
				if len(restConfig.TLSClientConfig.CAData) > 0 {
					pool := x509.NewCertPool()
					pool.AppendCertsFromPEM(restConfig.TLSClientConfig.CAData)
					tlsConfig.RootCAs = pool
				}
				if len(restConfig.TLSClientConfig.CertData) > 0 && len(restConfig.TLSClientConfig.KeyData) > 0 {
					cert, err := tls.X509KeyPair(restConfig.TLSClientConfig.CertData, restConfig.TLSClientConfig.KeyData)
					if err != nil {
						t.Fatalf("failed to load client cert: %v", err)
					}
					tlsConfig.Certificates = []tls.Certificate{cert}
				}

				// Use a custom dialer that resolves the shoot hostname to the provided IP.
				// This preserves the hostname in the URL (needed for TLS SNI routing
				// through istio ingress) while connecting to the actual IP.
				dialer := &net.Dialer{Timeout: 10 * time.Second}
				restConfig.Transport = &http.Transport{
					TLSClientConfig: tlsConfig,
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						_, addrPort, _ := net.SplitHostPort(addr)
						if addrPort == "" {
							addrPort = port
						}
						return dialer.DialContext(ctx, network, net.JoinHostPort(shootIP, addrPort))
					},
				}
				// Clear rest-level TLS config since we handle it via transport
				restConfig.TLSClientConfig.CAData = nil
				restConfig.TLSClientConfig.CertData = nil
				restConfig.TLSClientConfig.KeyData = nil
				t.Logf("Routing %s → %s:%s (SNI: %s)", restConfig.Host, shootIP, port, originalHost)
			}

			shootClient, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				t.Fatalf("failed to create shoot k8s client: %v", err)
			}

			// List namespaces as a simple connectivity test
			nsList, err := shootClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("failed to list namespaces on shoot: %v", err)
			}

			t.Logf("Successfully listed %d namespaces on shoot:", len(nsList.Items))
			for _, ns := range nsList.Items {
				t.Logf("  - %s", ns.Name)
			}

			// Verify we can also create/delete resources (needed for SA management)
			testNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("spike-test-%s", uuid.New().String()[:8]),
				},
			}
			_, err = shootClient.CoreV1().Namespaces().Create(ctx, testNS, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("failed to create test namespace on shoot: %v", err)
			}
			t.Logf("Created test namespace: %s", testNS.Name)

			err = shootClient.CoreV1().Namespaces().Delete(ctx, testNS.Name, metav1.DeleteOptions{})
			if err != nil {
				t.Fatalf("failed to delete test namespace: %v", err)
			}
			t.Logf("Deleted test namespace: %s", testNS.Name)
		})
	})
}
