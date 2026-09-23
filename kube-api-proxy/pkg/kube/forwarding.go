package kube

import (
	"net"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Both proxies use ReverseProxy.Rewrite rather than Director. Rewrite runs
// after the hop-by-hop headers are removed, so a client naming a header in
// Connection can't strip one the proxy sets: with Director, "Connection:
// Impersonate-User" removed the proxy's impersonation after the Director had
// set it, and the request ran as the proxy's own ServiceAccount
// (capsule-proxy CVE-2022-23652).

// setTarget points the outbound request at target: scheme and host only.
// ProxyRequest.SetURL would also join target's path onto the request path.
func setTarget(pr *httputil.ProxyRequest, target *url.URL) {
	pr.Out.URL.Scheme = target.Scheme
	pr.Out.URL.Host = target.Host
	pr.Out.Host = target.Host
}

// keepForwardingHeaders forwards what the Director-based proxy did: the inbound
// Forwarded and X-Forwarded-* headers, with the client IP appended to
// X-Forwarded-For so the apiserver's audit log keeps the chain of source IPs.
// Rewrite starts from an outbound request without them.
func keepForwardingHeaders(pr *httputil.ProxyRequest) {
	for _, name := range []string{"Forwarded", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if v, ok := pr.In.Header[name]; ok {
			pr.Out.Header[name] = v
		}
	}
	clientIP, _, err := net.SplitHostPort(pr.In.RemoteAddr)
	if err != nil {
		if v, ok := pr.In.Header["X-Forwarded-For"]; ok {
			pr.Out.Header["X-Forwarded-For"] = v
		}
		return
	}
	if prior := pr.In.Header["X-Forwarded-For"]; len(prior) > 0 {
		clientIP = strings.Join(prior, ", ") + ", " + clientIP
	}
	pr.Out.Header.Set("X-Forwarded-For", clientIP)
}
