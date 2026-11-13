package controller

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/config"
)

// The CRD rejects any change to spec.definitionRef, and an absent definitionHash
// differs from "" there. Adding and removing the finalizer must therefore never send
// the spec: an Update re-serialises it from the typed struct, which writes an absent
// hash back as "". The fake client does not evaluate the CRD's CEL rules, so this
// test checks the requests instead.
func TestFinalizer_PatchesOnlyMetadata(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()

	var updates int
	var patches []map[string]any
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*pluginsv1.PluginInstallation); ok {
					updates++
				}
				return c.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if _, ok := obj.(*pluginsv1.PluginInstallation); ok {
					data, err := patch.Data(obj)
					require.NoError(t, err)
					var body map[string]any
					require.NoError(t, json.Unmarshal(data, &body))
					patches = append(patches, body)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := &Reconciler{
		client:              c,
		logger:              slog.Default(),
		cfg:                 config.Config{AllowUnpinnedHash: true},
		uninstallHTTPClient: unreachableHTTPClient(),
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cr.Name}}
	ctx := context.Background()

	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	var got pluginsv1.PluginInstallation
	require.NoError(t, c.Get(ctx, req.NamespacedName, &got))
	assert.Contains(t, got.Finalizers, finalizerName)

	require.NoError(t, c.Delete(ctx, &got))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	err = c.Get(ctx, req.NamespacedName, &got)
	assert.True(t, apierrors.IsNotFound(err), "installation should be gone, got %v", err)

	assert.Zero(t, updates, "the installation must not be updated as a whole")
	require.Len(t, patches, 2, "one patch to add the finalizer, one to remove it")
	for _, body := range patches {
		assert.Equal(t, []string{"metadata"}, slices.Sorted(maps.Keys(body)), "a finalizer patch carries only metadata")
		metadata, ok := body["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, metadata, "finalizers")
		assert.Contains(t, metadata, "resourceVersion", "the patch is guarded by an optimistic lock")
	}
}
