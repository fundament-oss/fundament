package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
)

type recordingHandler struct {
	calls *[]string
	name  string
}

func (h recordingHandler) Sync(_ context.Context, _ uuid.UUID, _ SyncContext) error {
	*h.calls = append(*h.calls, h.name)
	return nil
}

func TestSyncHandlersFor_DefaultEntity(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var calls []string
	r.RegisterSync(EntityNamespace, recordingHandler{calls: &calls, name: "ns"})

	hs, err := r.SyncHandlersFor(EntityNamespace, dbconst.ClusterOutboxEvent_Created)
	require.NoError(t, err)
	require.Len(t, hs, 1)
}

func TestSyncHandlersFor_EventFansOutToMultiple(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var calls []string
	// Default handler for the entity (e.g. cluster create/update/delete).
	r.RegisterSync(EntityCluster, recordingHandler{calls: &calls, name: "cluster-default"})
	// Two independent subscribers to the ready event (usersync + namespace-sync).
	r.RegisterSyncForEvent(EntityCluster, dbconst.ClusterOutboxEvent_Ready, recordingHandler{calls: &calls, name: "usersync"})
	r.RegisterSyncForEvent(EntityCluster, dbconst.ClusterOutboxEvent_Ready, recordingHandler{calls: &calls, name: "namespace"})

	// A non-ready event resolves to just the default handler.
	created, err := r.SyncHandlersFor(EntityCluster, dbconst.ClusterOutboxEvent_Created)
	require.NoError(t, err)
	require.Len(t, created, 1)

	// The ready event fans out to both subscribers (not the default).
	ready, err := r.SyncHandlersFor(EntityCluster, dbconst.ClusterOutboxEvent_Ready)
	require.NoError(t, err)
	require.Len(t, ready, 2)
	for _, h := range ready {
		require.NoError(t, h.Sync(context.Background(), uuid.New(), SyncContext{}))
	}
	require.ElementsMatch(t, []string{"usersync", "namespace"}, calls)
}

func TestSyncHandlersFor_Unregistered(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.SyncHandlersFor(EntityNamespace, dbconst.ClusterOutboxEvent_Created)
	require.Error(t, err)
}
