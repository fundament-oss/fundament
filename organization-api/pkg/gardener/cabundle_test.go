package gardener

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust/certtest"
)

func caClusterConfigMap(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-shoot" + caClusterSuffix,
			Namespace: "garden-proj",
		},
		Data: data,
	}
}

func caClusterSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-shoot" + caClusterSuffix,
			Namespace: "garden-proj",
		},
		Data: data,
	}
}

// realClientWithConfigMapError builds a RealClient whose ConfigMap reads fail.
func realClientWithConfigMapError(t *testing.T, err error, objs ...client.Object) *RealClient {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isConfigMap := obj.(*corev1.ConfigMap); isConfigMap {
					return err
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	return &RealClient{client: c, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func forbidden() error {
	return apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "my-shoot.ca-cluster", errors.New("no"))
}

func TestRealClient_MonitoringCABundle(t *testing.T) {
	id := uuid.New()
	annotations := map[string]string{plutonoURLAnnotation: "https://plutono.example"}
	ca := certtest.NewCA(t).PEM

	t.Run("reads the ca-cluster config map", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterConfigMap(map[string]string{caBundleKey: string(ca)}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})

	t.Run("config map wins over the deprecated secret", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterConfigMap(map[string]string{caBundleKey: string(ca)}),
			caClusterSecret(map[string][]byte{caBundleKey: certtest.NewCA(t).PEM}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})

	t.Run("falls back to the deprecated secret", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterSecret(map[string][]byte{caBundleKey: ca}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})

	t.Run("config map forbidden falls back to the secret", func(t *testing.T) {
		c := realClientWithConfigMapError(t, forbidden(),
			shoot(id),
			monitoringSecret(annotations),
			caClusterSecret(map[string][]byte{caBundleKey: ca}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})

	t.Run("no CA published is not an error", func(t *testing.T) {
		c := realClientWith(t, shoot(id), monitoringSecret(annotations))

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, info.CABundle)
		assert.Equal(t, "https://plutono.example", info.URL)
	})

	// RBAC gaps do not heal by retrying; the operator bundle may still verify.
	t.Run("forbidden with no fallback degrades to no CA", func(t *testing.T) {
		c := realClientWithConfigMapError(t, forbidden(), shoot(id), monitoringSecret(annotations))

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, info.CABundle)
	})

	// Any other API error is not "no CA"; see caBundle.
	t.Run("API error with no fallback fails resolution", func(t *testing.T) {
		c := realClientWithConfigMapError(t, apierrors.NewServiceUnavailable("etcd"), shoot(id), monitoringSecret(annotations))

		_, err := c.Monitoring(context.Background(), id)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
		assert.True(t, apierrors.IsServiceUnavailable(err))
	})

	t.Run("a bundle holding no certificate falls through to the secret", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterConfigMap(map[string]string{caBundleKey: "not a certificate"}),
			caClusterSecret(map[string][]byte{caBundleKey: ca}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})

	t.Run("a bundle holding no certificate anywhere is dropped", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterConfigMap(map[string]string{caBundleKey: "not a certificate"}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, info.CABundle)
	})

	t.Run("empty config map key falls through to the secret", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(annotations),
			caClusterConfigMap(map[string]string{}),
			caClusterSecret(map[string][]byte{caBundleKey: ca}),
		)

		info, err := c.Monitoring(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, ca, info.CABundle)
	})
}
