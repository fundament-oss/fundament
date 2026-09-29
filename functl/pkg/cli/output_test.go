package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// API responses are protobuf messages. encoding/json cannot see their fields
// and printed {} for each, so -o json gave nothing to script against.

func newTestOrganization() *organizationv1.Organization {
	return organizationv1.Organization_builder{
		Id:      "019b4000-0000-7000-8000-000000000001",
		Name:    "acme-corp",
		Alias:   "Acme Corp",
		Created: timestamppb.New(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)),
	}.Build()
}

func decodeJSON(t *testing.T, v any) any {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, writeJSON(&buf, v))

	var decoded any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	return decoded
}

func TestWriteJSON_Message(t *testing.T) {
	got := decodeJSON(t, newTestOrganization())

	assert.Equal(t, map[string]any{
		"id":      "019b4000-0000-7000-8000-000000000001",
		"name":    "acme-corp",
		"alias":   "Acme Corp",
		"created": "2026-09-29T12:00:00Z",
	}, got)
}

func TestWriteJSON_SliceOfMessages(t *testing.T) {
	got := decodeJSON(t, []*organizationv1.Organization{newTestOrganization()})

	require.IsType(t, []any{}, got)
	require.Len(t, got, 1)
	assert.Equal(t, "acme-corp", got.([]any)[0].(map[string]any)["name"])
}

func TestWriteJSON_EmptySliceIsAnArray(t *testing.T) {
	assert.Equal(t, []any{}, decodeJSON(t, []*organizationv1.Organization(nil)))
}

func TestWriteJSON_PlainValue(t *testing.T) {
	assert.Equal(t, map[string]any{"id": "x"}, decodeJSON(t, map[string]string{"id": "x"}))
}
