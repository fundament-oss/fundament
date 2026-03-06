package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockChecker struct {
	ready bool
}

func (m *mockChecker) IsReady() bool {
	return m.ready
}

func TestLivenessHandler(t *testing.T) {
	handler := LivenessHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected 'ok', got %q", rec.Body.String())
	}
}

func TestReadinessHandlerNotReady(t *testing.T) {
	checker := &mockChecker{ready: false}
	handler := ReadinessHandler(checker)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadinessHandlerReady(t *testing.T) {
	checker := &mockChecker{ready: true}
	handler := ReadinessHandler(checker)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected 'ok', got %q", rec.Body.String())
	}
}
