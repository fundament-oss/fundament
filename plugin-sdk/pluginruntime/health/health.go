package health

import "net/http"

// ReadinessChecker reports whether the plugin is ready to serve traffic.
type ReadinessChecker interface {
	IsReady() bool
}

// LivenessHandler returns an HTTP handler that always responds 200 OK.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// ReadinessHandler returns an HTTP handler that responds 200 OK when the
// checker reports ready, and 503 Service Unavailable otherwise.
func ReadinessHandler(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if checker.IsReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	}
}
