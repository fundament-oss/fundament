package dcimauthn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The body is decoded with encoding/json/v2, which rejects input the v1
// decoder let through: duplicate names, invalid UTF-8 and trailing data. The
// body is also capped at maxLoginBodyBytes. All of these must fail before any
// credentials reach dex.
func TestHandlePasswordLogin_RejectsMalformedBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body string
	}{
		{"duplicate name", `{"email":"a@example.com","email":"b@example.com","password":"x"}`},
		{"invalid utf-8", "{\"email\":\"\xff\",\"password\":\"x\"}"},
		{"trailing data", `{"email":"a@example.com","password":"x"} {}`},
		{"empty body", ""},
		{"oversized body", `{"email":"` + strings.Repeat("a", maxLoginBodyBytes) + `","password":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := refreshTestServer()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			s.HandlePasswordLogin(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "invalid request body")
		})
	}
}
