package dcim_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/dcim-api/pkg/dcim"
	"github.com/fundament-oss/fundament/dcim-api/pkg/metal"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

// fakeMetalStore answers the capacity query with one partition, every other
// instant query with nothing and every range query with two points.
func fakeMetalStore(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		query := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1764000000,"4200"],[1764000300,"4300"]]}]}}`))
		case strings.HasPrefix(query, `{__name__=~"metal_partition_capacity_`):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"metal_partition_capacity_total","partition":"fire","size":"c1-large-x86"},"value":[1764000000,"12"]}]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func metalClient(t *testing.T, opts ...dcim.Option) dcimv1connect.MetalServiceClient {
	t.Helper()
	env := newTestAPI(t, opts...)
	return dcimv1connect.NewMetalServiceClient(env.client(), env.server.URL)
}

func withFakeStore(t *testing.T, status int) dcim.Option {
	t.Helper()
	return dcim.WithMetal(metal.NewReader(prom.NewHTTPClient(fakeMetalStore(t, status))))
}

func TestGetMetalSnapshot(t *testing.T) {
	msg, err := metalClient(t, withFakeStore(t, 0)).GetMetalSnapshot(context.Background(), &dcimv1.GetMetalSnapshotRequest{})
	require.NoError(t, err)

	assert.True(t, msg.HasCollectedAt())
	capacity := msg.GetResults()["capacity"].GetSamples()
	require.Len(t, capacity, 1)
	assert.Equal(t, "fire", capacity[0].GetLabels()["partition"])
	assert.InDelta(t, 12, capacity[0].GetValue(), 0)

	power := msg.GetResults()["power"].GetSeries()
	require.Len(t, power, 1)
	require.Len(t, power[0].GetPoints(), 2)
	assert.InDelta(t, 4300, power[0].GetPoints()[1].GetValue(), 0)
	assert.Equal(t, int64(1764000300), power[0].GetPoints()[1].GetTime().GetSeconds())
}

func TestGetMetalSnapshotErrors(t *testing.T) {
	tests := []struct {
		name string
		opts []dcim.Option
		code connect.Code
	}{
		{name: "not configured", code: connect.CodeFailedPrecondition},
		{name: "store down", opts: []dcim.Option{withFakeStore(t, http.StatusBadGateway)}, code: connect.CodeUnavailable},
		{name: "query rejected", opts: []dcim.Option{withFakeStore(t, http.StatusBadRequest)}, code: connect.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := metalClient(t, tt.opts...).GetMetalSnapshot(context.Background(), &dcimv1.GetMetalSnapshotRequest{})
			require.Error(t, err)
			assert.Equal(t, tt.code, connect.CodeOf(err))
		})
	}
}
