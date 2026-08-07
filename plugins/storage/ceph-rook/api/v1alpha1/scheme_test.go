package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToScheme(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, AddToScheme(s))
	assert.True(t, s.Recognizes(GroupVersion.WithKind("Disk")))
	assert.True(t, s.Recognizes(GroupVersion.WithKind("StoragePool")))
}

func TestDiskDeepCopy(t *testing.T) {
	d := &Disk{Status: DiskStatus{Node: "n1", SizeBytes: 100}}
	got := d.DeepCopy()
	got.Status.Node = "n2"
	assert.Equal(t, "n1", d.Status.Node) // original unchanged
}
