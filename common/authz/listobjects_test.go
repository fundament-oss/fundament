package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListObjects_SendsTupleAndStripsTypePrefix(t *testing.T) {
	t.Parallel()
	userID := uuid.New()

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/stores":
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"stores": []map[string]any{{"id": "01HZX5B9N3T7Q2W8E4R6Y1V0KM", "name": "fundament",
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}},
			}))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stores/01HZX5B9N3T7Q2W8E4R6Y1V0KM/list-objects"):
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"objects": []string{"project:p1", "project:p2"},
			}))
		default:
			assert.Failf(t, "unexpected request", "%s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(newRef(t, srv.URL))
	ids, err := c.ListObjects(context.Background(), User(userID), CanView(), ObjectTypeProject)
	require.NoError(t, err)

	assert.Equal(t, []string{"p1", "p2"}, ids)
	assert.Equal(t, "user:"+userID.String(), got["user"])
	assert.Equal(t, "can_view", got["relation"])
	assert.Equal(t, "project", got["type"])
}
