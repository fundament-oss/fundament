package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The request logger's writer must pass flushes through, or watch events
// through the reverse proxy arrive one event late.
func TestStatusRecorder_PassesFlushThrough(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	w := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	_, err := w.Write([]byte(`{"type":"MODIFIED"}`))
	require.NoError(t, err)
	require.NoError(t, http.NewResponseController(w).Flush())
	assert.True(t, rec.Flushed)
}
