package notify

import (
	"net/http"
	"time"
)

// SetTransport lets tests trust an httptest TLS server while keeping the
// client's redirect policy.
func SetTransport(w *Webhooks, rt http.RoundTripper) { w.client.Transport = rt }

// SetClock lets tests move time past the cooldown.
func SetClock(w *Webhooks, now func() time.Time) { w.now = now }
