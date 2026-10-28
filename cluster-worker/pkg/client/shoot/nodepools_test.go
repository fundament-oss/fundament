package shoot

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

var nodeCreated = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func poolNode(name, pool string, ready corev1.ConditionStatus) *corev1.Node {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(nodeCreated)}}
	if pool != "" {
		node.Labels = map[string]string{"worker.gardener.cloud/pool": pool}
	}
	if ready != "" {
		node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}
	}
	return node
}

func TestGroupPoolNodes(t *testing.T) {
	t.Parallel()

	nodes := []corev1.Node{
		*poolNode("a1", "smal1a2b", corev1.ConditionTrue),
		*poolNode("a2", "smal1a2b", corev1.ConditionTrue),
		*poolNode("b1", "medi456a", corev1.ConditionFalse),
		*poolNode("b2", "medi456a", corev1.ConditionTrue),
		*poolNode("c1", "gpu0c3d4", ""), // reports no Ready condition: not ready
		*poolNode("x1", "", corev1.ConditionTrue),
	}

	assert.Equal(t, map[string][]PoolNode{
		"smal1a2b": {{Created: nodeCreated, Ready: true}, {Created: nodeCreated, Ready: true}},
		"medi456a": {{Created: nodeCreated, Ready: false}, {Created: nodeCreated, Ready: true}},
		"gpu0c3d4": {{Created: nodeCreated, Ready: false}},
	}, groupPoolNodes(nodes))
}

func TestGroupPoolNodes_NoNodes(t *testing.T) {
	t.Parallel()
	assert.Empty(t, groupPoolNodes(nil))
}

func TestCountPoolNodes(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	nodes := []PoolNode{
		{Created: since.Add(-time.Hour), Ready: true}, // a predecessor's
		{Created: since, Ready: true},
		{Created: since.Add(time.Minute), Ready: false},
	}

	assert.Equal(t, PoolNodes{Nodes: 3, Ready: 2}, CountPoolNodes(nodes, time.Time{}))
	assert.Equal(t, PoolNodes{Nodes: 2, Ready: 1}, CountPoolNodes(nodes, since))
	assert.Equal(t, PoolNodes{}, CountPoolNodes(nil, since))
}

func TestListNodePools_ReadsFromWatchCache(t *testing.T) {
	t.Parallel()

	cs := fake.NewClientset(
		poolNode("a1", "smal1a2b", corev1.ConditionTrue),
		poolNode("b1", "medi456a", corev1.ConditionFalse),
	)
	var opts []metav1.ListOptions
	cs.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListActionImpl)
		require.True(t, ok)
		opts = append(opts, list.ListOptions)
		return false, nil, nil
	})
	r := realAccessWith(t, cs)

	pools, err := r.ListNodePools(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, map[string][]PoolNode{
		"smal1a2b": {{Created: nodeCreated, Ready: true}},
		"medi456a": {{Created: nodeCreated, Ready: false}},
	}, pools)

	require.Len(t, opts, 1)
	assert.Equal(t, "0", opts[0].ResourceVersion)
	assert.Equal(t, "worker.gardener.cloud/pool", opts[0].LabelSelector)
}

func TestListNodePools_ErrorsPassThrough(t *testing.T) {
	t.Parallel()

	t.Run("client", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("no kubeconfig")
		r := &RealShootAccess{
			logger:    slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})),
			newClient: func(context.Context, uuid.UUID) (kubernetes.Interface, error) { return nil, boom },
		}
		_, err := r.ListNodePools(context.Background(), uuid.New())
		require.ErrorIs(t, err, boom)
	})

	t.Run("list", func(t *testing.T) {
		t.Parallel()
		cs := fake.NewClientset()
		boom := errors.New("api server gone")
		cs.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, boom
		})
		_, err := realAccessWith(t, cs).ListNodePools(context.Background(), uuid.New())
		require.ErrorIs(t, err, boom)
	})
}

// ListNodePools reads through the long-lived credential cache, and drops a
// cluster's entry when a read fails: the shoot may have new credentials.
func TestListNodePools_ReusesCredentials(t *testing.T) {
	t.Parallel()

	cs := fake.NewClientset()
	clusterID := uuid.New()
	r := realAccessWith(t, cs)
	r.nodeCredentials = newTTLClientCache(nodeCredentialTTL)
	r.nodeCredentials.put(clusterID, &rest.Config{})
	var cached []bool
	r.newClient = func(ctx context.Context, _ uuid.UUID) (kubernetes.Interface, error) {
		cached = append(cached, clientCacheFrom(ctx) == r.nodeCredentials)
		return cs, nil
	}

	_, err := r.ListNodePools(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, cached)
	_, ok := r.nodeCredentials.get(clusterID)
	assert.True(t, ok)

	cs.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("unauthorized")
	})
	_, err = r.ListNodePools(context.Background(), clusterID)
	require.Error(t, err)
	_, ok = r.nodeCredentials.get(clusterID)
	assert.False(t, ok)
}

func TestMockListNodePools(t *testing.T) {
	t.Parallel()

	m := NewMockShootAccess(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	clusterID := uuid.New()

	pools, err := m.ListNodePools(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Empty(t, pools)

	set := []PoolNode{{Created: nodeCreated, Ready: true}, {Created: nodeCreated}}
	m.SetNodePools(clusterID, map[string][]PoolNode{"smal1a2b": set})
	pools, err = m.ListNodePools(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Equal(t, map[string][]PoolNode{"smal1a2b": set}, pools)

	// The caller gets a copy.
	pools["smal1a2b"][0].Ready = false
	again, err := m.ListNodePools(context.Background(), clusterID)
	require.NoError(t, err)
	assert.True(t, again["smal1a2b"][0].Ready)

	m.ListNodePoolsError = errors.New("unreachable")
	_, err = m.ListNodePools(context.Background(), clusterID)
	require.Error(t, err)
}
