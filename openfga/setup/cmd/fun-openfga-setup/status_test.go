package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeStatusPublishesTheGeneration(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- serveStatus(ctx, addr, Status{Generation: "7", Store: "fundament", StoreID: "01ABC"}) }()

	var got Status
	for range 50 {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+statusPath, http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
			_ = resp.Body.Close()

			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, Status{Generation: "7", Store: "fundament", StoreID: "01ABC"}, got)

	cancel()
	require.NoError(t, <-done, "shutdown must not be reported as a failure")
}

func TestServeStatusAnswersOnlyItsOwnPath(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = serveStatus(ctx, addr, Status{Generation: "7"}) }()

	var code int
	for range 50 {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/etc/passwd", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			code = resp.StatusCode
			_ = resp.Body.Close()

			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, http.StatusNotFound, code, "it is not a file server")
}

func TestSameModelComparesContentNotIdentity(t *testing.T) {
	a := &openfga.AuthorizationModel{Id: "01A", SchemaVersion: "1.1", TypeDefinitions: []openfga.TypeDefinition{{Type: "user"}}}
	b := &openfga.AuthorizationModel{Id: "01B", SchemaVersion: "1.1", TypeDefinitions: []openfga.TypeDefinition{{Type: "user"}}}
	c := &openfga.AuthorizationModel{Id: "01C", SchemaVersion: "1.1", TypeDefinitions: []openfga.TypeDefinition{{Type: "organization"}}}

	same, err := sameModel(a, b)
	require.NoError(t, err)
	assert.True(t, same, "a differing id must not force a rewrite")

	same, err = sameModel(a, c)
	require.NoError(t, err)
	assert.False(t, same)
}

// OpenFGA returns a model with every optional field materialised: absent metadata
// comes back as null, unset modules as "", omitted user-type lists as []. Comparing
// the decoded structures therefore never matches, and the model is rewritten on
// every pod start, rotating its id under consumers that pin nothing.
func TestSameModelIgnoresServerMaterialisedZeroValues(t *testing.T) {
	shipped := &openfga.AuthorizationModel{
		Id:            "01A",
		SchemaVersion: "1.1",
		TypeDefinitions: []openfga.TypeDefinition{
			{Type: "user"},
			{Type: "organization", Relations: &map[string]openfga.Userset{
				"admin": {This: &map[string]any{}},
			}},
		},
	}

	readBack := materialiseZeroValues(t, shipped)

	require.False(t, reflect.DeepEqual(shipped.TypeDefinitions, readBack.TypeDefinitions),
		"the structures must actually differ, or this test proves nothing")

	same, err := sameModel(shipped, readBack)
	require.NoError(t, err)
	assert.True(t, same, "the same model read back from the server must compare equal")
}

// materialiseZeroValues rewrites a model the way the server does on read.
func materialiseZeroValues(t *testing.T, model *openfga.AuthorizationModel) *openfga.AuthorizationModel {
	t.Helper()

	raw, err := json.Marshal(model)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))

	for _, def := range generic["type_definitions"].([]any) {
		typeDef := def.(map[string]any)
		if _, ok := typeDef["relations"]; !ok {
			typeDef["relations"] = map[string]any{}
			typeDef["metadata"] = nil

			continue
		}

		relations := map[string]any{}
		for name := range typeDef["relations"].(map[string]any) {
			relations[name] = map[string]any{
				"directly_related_user_types": []any{},
				"module":                      "",
				"source_info":                 nil,
			}
		}

		typeDef["metadata"] = map[string]any{"module": "", "relations": relations}
	}

	raw, err = json.Marshal(generic)
	require.NoError(t, err)

	var out openfga.AuthorizationModel
	require.NoError(t, json.Unmarshal(raw, &out))

	return &out
}
