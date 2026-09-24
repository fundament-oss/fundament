package kube

import (
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/net/http/httpguts"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client connects to a real Kubernetes API server using a kubeconfig.
// Auth is handled by the transport created via rest.HTTPClientFor, which supports
// bearer tokens, client certificates, and basic auth from the kubeconfig.
// Exec-based credential plugins (e.g. aws-iam-authenticator) are not supported.
type Client struct {
	transport http.RoundTripper
	host      *url.URL
}

// NewFromBytes creates a Client whose transport uses the kubeconfig's own
// credentials (bearer token, client cert, basic auth).
func NewFromBytes(kubeconfigData []byte) (*Client, error) {
	return newFromBytes(kubeconfigData, false)
}

// NewAnonymousFromBytes creates a Client whose transport carries no client
// credentials, keeping only the server TLS/CA settings. Use it when the caller
// supplies the identity per request (e.g. a bearer token injected by a reverse
// proxy), so a client certificate in the transport can't override it.
func NewAnonymousFromBytes(kubeconfigData []byte) (*Client, error) {
	return newFromBytes(kubeconfigData, true)
}

func newFromBytes(kubeconfigData []byte, anonymous bool) (*Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	if anonymous {
		cfg = rest.AnonymousClientConfig(cfg)
	}

	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build http client: %w", err)
	}

	upgrade, err := http1TransportFor(cfg)
	if err != nil {
		return nil, err
	}

	host, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig host: %w", err)
	}
	host.Path = ""
	host.RawQuery = ""

	return &Client{
		transport: &upgradeAwareTransport{regular: httpClient.Transport, upgrade: upgrade},
		host:      host,
	}, nil
}

// http1TransportFor builds a transport for cfg that never negotiates HTTP/2,
// with the same TLS settings and credentials as rest.HTTPClientFor.
func http1TransportFor(cfg *rest.Config) (http.RoundTripper, error) {
	tlsConfig, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}
	if tlsConfig != nil {
		tlsConfig.NextProtos = []string{"http/1.1"}
	}
	t := utilnet.SetOldTransportDefaults(&http.Transport{TLSClientConfig: tlsConfig})
	rt, err := rest.HTTPWrappersForConfig(cfg, t)
	if err != nil {
		return nil, fmt.Errorf("build HTTP/1.1 transport: %w", err)
	}
	return rt, nil
}

// upgradeAwareTransport sends protocol upgrades (exec, attach, port-forward)
// over HTTP/1.1 and everything else over the regular, HTTP/2-capable transport.
// net/http only keeps WebSocket upgrades off a pooled HTTP/2 connection, and
// HTTP/2 rejects every other Upgrade: kubectl's SPDY/3.1 fallback failed with
// "http2: invalid Upgrade request header".
type upgradeAwareTransport struct {
	regular http.RoundTripper
	upgrade http.RoundTripper
}

func (t *upgradeAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if httpguts.HeaderValuesContainsToken(req.Header["Connection"], "upgrade") {
		return t.upgrade.RoundTrip(req) //nolint:wrapcheck // a RoundTripper passes its transport's errors through
	}
	return t.regular.RoundTrip(req) //nolint:wrapcheck // a RoundTripper passes its transport's errors through
}

// Host returns the parsed base URL of the Kubernetes API server.
func (c *Client) Host() *url.URL {
	return c.host
}

// Transport returns the http.RoundTripper configured for the Kubernetes API server.
func (c *Client) Transport() http.RoundTripper {
	return c.transport
}
