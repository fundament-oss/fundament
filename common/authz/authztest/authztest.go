// Package authztest serves an in-process OpenFGA over the two HTTP calls that
// Client.Evaluations needs (ListStores and BatchCheck), so tests can run real
// checks against the real model. Other routes answer 404 naming the route.
package authztest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/fundament-oss/fundament/common/authz"
)

// StoreName is the name of the store Serve provisions.
const StoreName = "fundament"

// Tuple is one relationship written to the store.
type Tuple struct {
	User, Relation, Object string
}

// Server is an HTTP front for an in-process OpenFGA.
type Server struct {
	*httptest.Server

	batchCheckCalls atomic.Int32
}

// BatchCheckCalls returns how many batch-check requests the server received.
func (s *Server) BatchCheckCalls() int {
	return int(s.batchCheckCalls.Load())
}

// Serve starts an OpenFGA holding the authorization model and tuples under StoreName.
func Serve(t *testing.T, tuples ...Tuple) *Server {
	t.Helper()
	ctx := t.Context()

	fga := server.MustNewServerWithOpts(server.WithDatastore(memory.New()))
	t.Cleanup(fga.Close)

	store, err := fga.CreateStore(ctx, &openfgav1.CreateStoreRequest{Name: StoreName})
	require.NoError(t, err)

	model, err := transformer.TransformDSLToProto(authz.ModelDSL)
	require.NoError(t, err)

	_, err = fga.WriteAuthorizationModel(ctx, &openfgav1.WriteAuthorizationModelRequest{
		StoreId:         store.GetId(),
		TypeDefinitions: model.GetTypeDefinitions(),
		SchemaVersion:   model.GetSchemaVersion(),
		Conditions:      model.GetConditions(),
	})
	require.NoError(t, err)

	if len(tuples) > 0 {
		keys := make([]*openfgav1.TupleKey, 0, len(tuples))
		for _, tuple := range tuples {
			keys = append(keys, &openfgav1.TupleKey{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object})
		}

		_, err = fga.Write(ctx, &openfgav1.WriteRequest{
			StoreId: store.GetId(),
			Writes:  &openfgav1.WriteRequestWrites{TupleKeys: keys},
		})
		require.NoError(t, err)
	}

	srv := &Server{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/stores":
			resp, err := fga.ListStores(r.Context(), &openfgav1.ListStoresRequest{Name: r.URL.Query().Get("name")})
			reply(w, resp, err)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/batch-check"):
			srv.batchCheckCalls.Add(1)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var req openfgav1.BatchCheckRequest
			if err := protojson.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.StoreId = store.GetId()

			resp, err := fga.BatchCheck(r.Context(), &req)
			reply(w, resp, err)
		default:
			http.Error(w, "authztest: unsupported route "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

func reply(w http.ResponseWriter, resp proto.Message, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out, err := protojson.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(out)
}
