package authz

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusServer stands in for the provision sidecar's status endpoint.
type statusServer struct {
	body  atomic.Value // string
	code  atomic.Int32
	calls atomic.Int32
}

// Model ids are ULIDs; the SDK rejects anything else before it reaches the wire.
const (
	modelOne = "01JMZ0000000000000000000M1"
	modelTwo = "01JMZ0000000000000000000M2"
)

func newStatusServer(t *testing.T, storeID, modelID string) (*statusServer, string) {
	t.Helper()

	s := &statusServer{}
	s.set(storeID, modelID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.calls.Add(1)

		if code := s.code.Load(); code != 0 {
			w.WriteHeader(int(code))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s.body.Load().(string)))
	}))
	t.Cleanup(srv.Close)

	return s, srv.URL + "/status.json"
}

func (s *statusServer) set(storeID, modelID string) {
	s.body.Store(fmt.Sprintf(`{"generation":"1","store":"fundament","id":%q,"model_id":%q}`, storeID, modelID))
}

func TestModelPinUsesThePublishedModel(t *testing.T) {
	_, url := newStatusServer(t, storeOldest, modelOne)
	pin := NewModelPin(url)

	id, err := pin.ID(t.Context(), storeOldest)

	require.NoError(t, err)
	assert.Equal(t, modelOne, id)
}

// Without a status URL the pin is inert and OpenFGA evaluates the latest model,
// which is the behaviour for any caller outside the chart.
func TestModelPinWithoutStatusURLDoesNotPin(t *testing.T) {
	id, err := NewModelPin("").ID(t.Context(), storeOldest)

	require.NoError(t, err)
	assert.Empty(t, id)
}

func TestNilModelPinDoesNotPin(t *testing.T) {
	var pin *ModelPin

	id, err := pin.ID(t.Context(), storeOldest)

	require.NoError(t, err)
	assert.Empty(t, id)
}

// A model id belongs to the store it was written in. Pinning one from a different
// store would name a model that store never had, so the caller fails closed until
// the provisioner catches up.
func TestModelPinRejectsAModelFromAnotherStore(t *testing.T) {
	_, url := newStatusServer(t, storeNewer, modelOne)
	pin := NewModelPin(url)

	_, err := pin.ID(t.Context(), storeOldest)

	require.ErrorIs(t, err, ErrModelUnknown)
	assert.Contains(t, err.Error(), storeOldest)
}

func TestModelPinRejectsAnEmptyModelID(t *testing.T) {
	_, url := newStatusServer(t, storeOldest, "")
	pin := NewModelPin(url)

	_, err := pin.ID(t.Context(), storeOldest)

	require.ErrorIs(t, err, ErrModelUnknown)
}

func TestModelPinRejectsANonOKStatus(t *testing.T) {
	srv, url := newStatusServer(t, storeOldest, modelOne)
	srv.code.Store(http.StatusServiceUnavailable)
	pin := NewModelPin(url)

	_, err := pin.ID(t.Context(), storeOldest)

	require.ErrorIs(t, err, ErrModelUnknown)
}

func TestModelPinFetchesOncePerStore(t *testing.T) {
	srv, url := newStatusServer(t, storeOldest, modelOne)
	pin := NewModelPin(url)

	for range 5 {
		_, err := pin.ID(t.Context(), storeOldest)
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), srv.calls.Load())
}

// A reset replaces the store, so the pin must not keep serving the model id of
// the store that is gone.
func TestModelPinRefreshesWhenTheStoreChanges(t *testing.T) {
	srv, url := newStatusServer(t, storeOldest, modelOne)
	pin := NewModelPin(url)

	first, err := pin.ID(t.Context(), storeOldest)
	require.NoError(t, err)

	srv.set(storeNewer, modelTwo)

	second, err := pin.ID(t.Context(), storeNewer)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Equal(t, modelTwo, second)
	assert.Equal(t, int32(2), srv.calls.Load())
}

// The pinned id has to reach OpenFGA, not merely be resolved: an unpinned check
// evaluates whatever model is latest, which is the thing pinning exists to stop.
func TestClientEvaluateSendsThePinnedModel(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	_, statusURL := newStatusServer(t, storeOldest, modelOne)

	c, err := New(Config{APIURL: srv.url(t), StoreName: "fundament", StatusURL: statusURL})
	require.NoError(t, err)

	_, err = c.Evaluate(t.Context(), EvaluationRequest{
		Subject:  User(uuid.New()),
		Action:   CanView(),
		Resource: Cluster(uuid.New()),
	})
	require.NoError(t, err)

	assert.Contains(t, srv.lastCheckBody(), modelOne)
}

func TestClientEvaluateWithoutAStatusURLSendsNoModel(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}

	c, err := New(Config{APIURL: srv.url(t), StoreName: "fundament"})
	require.NoError(t, err)

	_, err = c.Evaluate(t.Context(), EvaluationRequest{
		Subject:  User(uuid.New()),
		Action:   CanView(),
		Resource: Cluster(uuid.New()),
	})
	require.NoError(t, err)

	// The SDK always emits the field; empty is what leaves the model unpinned.
	assert.Contains(t, srv.lastCheckBody(), `"authorization_model_id":""`)
}
