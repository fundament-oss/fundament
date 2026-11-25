package analyze

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrRefusedAddress is returned when a registry resolves to an address the
// worker must not reach: the image reference is the publisher's choice, and
// the worker runs inside the cluster (FUN-24).
var ErrRefusedAddress = errors.New("refused to connect to a non-public address")

// ErrInsecureScheme is returned for a plain-http request to a registry that is
// not listed as insecure.
var ErrInsecureScheme = errors.New("refused a plain-http registry request")

// refusedPrefixes are the ranges a publisher-chosen registry must not resolve
// into: loopback, private, link-local (which holds the cloud metadata
// endpoint), carrier-grade NAT, benchmarking, multicast and reserved space,
// and the IPv6 ranges that carry an IPv4 address on to it (NAT64, 6to4,
// Teredo), which Unmap does not catch.
var refusedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// isRefused reports whether ip is outside public unicast space.
func isRefused(ip netip.Addr) bool {
	// Prefix.Contains never matches an address with a zone, so fe80::1%eth0
	// would pass fe80::/10: drop the zone first.
	ip = ip.WithZone("").Unmap()
	for _, prefix := range refusedPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// refuseNonPublic is a net.Dialer Control hook. It runs on the address
// actually being dialed, after DNS resolution, so a name that resolves (or
// rebinds) to an internal address is refused as well.
func refuseNonPublic(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrRefusedAddress, address)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || isRefused(ip) {
		return fmt.Errorf("%w: %s", ErrRefusedAddress, address)
	}
	return nil
}

// hostSet matches "host:port" or a bare "host".
type hostSet map[string]struct{}

func newHostSet(hosts []string) hostSet {
	set := make(hostSet, len(hosts))
	for _, host := range hosts {
		if host != "" {
			set[host] = struct{}{}
		}
	}
	return set
}

func (s hostSet) contains(hostport string) bool {
	if _, ok := s[hostport]; ok {
		return true
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	_, ok := s[host]
	return ok
}

// newTransport is the only way the worker talks to a registry. It refuses
// non-public addresses except for allowPrivate hosts (local development),
// plain http except for insecure hosts, and ignores proxy settings, which
// would move the address check onto the proxy.
func newTransport(allowPrivate, insecure []string) http.RoundTripper {
	allowed := newHostSet(allowPrivate)
	guarded := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: refuseNonPublic}
	plain := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

	return &schemeGuard{
		insecure: newHostSet(insecure),
		next: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if allowed.contains(addr) {
					return plain.DialContext(ctx, network, addr) //nolint:wrapcheck // dial errors are classified by type
				}
				return guarded.DialContext(ctx, network, addr) //nolint:wrapcheck // dial errors are classified by type
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: time.Minute,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// schemeGuard refuses plain http unless the host is listed as insecure. It
// sees every request, redirects included.
type schemeGuard struct {
	insecure hostSet
	next     http.RoundTripper
}

func (g *schemeGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" && !g.insecure.contains(req.URL.Host) {
		return nil, fmt.Errorf("%w: %s", ErrInsecureScheme, req.URL.Redacted())
	}
	return g.next.RoundTrip(req) //nolint:wrapcheck // errors are classified by type
}
