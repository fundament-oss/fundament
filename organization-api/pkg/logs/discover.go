package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrValiNotFound is returned when no probed Plutono datasource proxy id
// answers the Vali API. Expected while a shoot's logging stack is still
// provisioning; persistent occurrences on a healthy shoot suggest Gardener no
// longer provisions a Vali datasource (e.g. the VictoriaLogs migration,
// ADR-0027) and this backend needs a new implementation.
var ErrValiNotFound = errors.New("no Plutono datasource proxies the Vali API")

// maxDatasourceProbeID bounds the discovery scan. Gardener provisions a
// handful of datasources per shoot Plutono (Prometheus, Vali, sometimes
// Alertmanager); live inventory 2026-08-04 saw ids 1 (Prometheus) and 2
// (Vali), with ids 3+ answering "Unable to load datasource meta data".
const maxDatasourceProbeID = 5

// DiscoverValiProxyBase finds the Plutono datasource-proxy base URL for the
// shoot's Vali by probing numeric proxy ids with a cheap labels call, and
// returns "{plutonoURL}/api/datasources/proxy/{id}".
//
// Probing is the only discovery that works: behind the ingress basic auth the
// caller is an anonymous Plutono Viewer, and the datasource list/name/uid APIs
// are admin-only (403) — verified live 2026-08-04 (ADR-0027). A 401 during
// probing is reported as a StatusError immediately (wrong credentials fail
// every id; callers re-read the monitoring secret rather than scanning on).
func DiscoverValiProxyBase(ctx context.Context, plutonoURL, username, password string, opts ...Option) (string, error) {
	for id := 1; id <= maxDatasourceProbeID; id++ {
		base := plutonoURL + "/api/datasources/proxy/" + strconv.Itoa(id)
		probe := NewLokiClientWithAuth(base, username, password, opts...)
		probe.httpClient.Timeout = 10 * time.Second

		_, err := probe.labelValues(ctx, labelNamespace, "")
		if err == nil {
			return base, nil
		}

		var statusErr *StatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("probe datasource id %d: %w", id, err)
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			// Transport-level errors (DNS, TLS, refused) won't improve for
			// higher ids — the ingress itself is unreachable.
			return "", fmt.Errorf("probe datasource id %d: %w", id, err)
		}
		// Everything else means "not Vali behind this id, keep scanning":
		// 404 (path not proxied), 500 ("Unable to load datasource meta data"
		// for nonexistent ids), or a 200 whose body isn't the Loki envelope.
		continue
	}
	return "", ErrValiNotFound
}
