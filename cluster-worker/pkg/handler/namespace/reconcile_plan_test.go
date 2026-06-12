package namespace

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
)

func labelled(name string, id uuid.UUID) shoot.ResourceInfo {
	return shoot.ResourceInfo{Name: name, Labels: map[string]string{LabelNamespaceID: id.String()}}
}

func TestBuildPlan_EqualSets(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()

	plan := BuildPlan([]uuid.UUID{a, b}, []shoot.ResourceInfo{labelled("a", a), labelled("b", b)})

	require.Empty(t, plan.CreateIDs)
	require.Empty(t, plan.DeleteNames)
}

func TestBuildPlan_MissingOnCluster(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()

	plan := BuildPlan([]uuid.UUID{a, b}, []shoot.ResourceInfo{labelled("a", a)})

	require.Equal(t, []uuid.UUID{b}, plan.CreateIDs)
	require.Empty(t, plan.DeleteNames)
}

func TestBuildPlan_OrphanOnCluster(t *testing.T) {
	t.Parallel()
	a, orphan := uuid.New(), uuid.New()

	plan := BuildPlan([]uuid.UUID{a}, []shoot.ResourceInfo{labelled("a", a), labelled("orphan-ns", orphan)})

	require.Empty(t, plan.CreateIDs)
	require.Equal(t, []string{"orphan-ns"}, plan.DeleteNames)
}

func TestBuildPlan_SoftDeletedRowIsOrphan(t *testing.T) {
	t.Parallel()
	// A soft-deleted DB row is simply absent from the active set; its still-live
	// cluster namespace must be flagged for deletion.
	live := uuid.New()
	deletedRow := uuid.New()

	plan := BuildPlan([]uuid.UUID{live}, []shoot.ResourceInfo{labelled("live", live), labelled("deleted", deletedRow)})

	require.Empty(t, plan.CreateIDs)
	require.Equal(t, []string{"deleted"}, plan.DeleteNames)
}

func TestBuildPlan_UntaggedNamespacesIgnored(t *testing.T) {
	t.Parallel()
	a := uuid.New()

	cluster := []shoot.ResourceInfo{
		labelled("a", a),
		{Name: "kube-system"}, // no labels at all
		{Name: "operator", Labels: map[string]string{"foo": "bar"}}, // unrelated label
		{Name: "broken", Labels: map[string]string{LabelNamespaceID: "not-a-uuid"}},
	}

	plan := BuildPlan([]uuid.UUID{a}, cluster)

	require.Empty(t, plan.CreateIDs)
	require.Empty(t, plan.DeleteNames, "untagged or malformed namespaces must never be deleted")
}

func TestBuildPlan_EmptyDB_AllOrphans(t *testing.T) {
	t.Parallel()
	x, y := uuid.New(), uuid.New()

	plan := BuildPlan(nil, []shoot.ResourceInfo{labelled("x", x), labelled("y", y)})

	require.Empty(t, plan.CreateIDs)
	require.ElementsMatch(t, []string{"x", "y"}, plan.DeleteNames)
}
