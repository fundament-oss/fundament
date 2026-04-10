package health

import (
	"context"
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/livez", http.NoBody)
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadinessHandlerReady(t *testing.T) {
	checker := &mockChecker{ready: true}
	handler := ReadinessHandler(checker)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected 'ok', got %q", rec.Body.String())
	}
}
