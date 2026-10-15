package cnpg

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	pg16 = "ghcr.io/cloudnative-pg/postgresql:16.15-202610120828-standard-bookworm@sha256:1fe5"
	pg17 = "ghcr.io/cloudnative-pg/postgresql:17.11-202610120828-standard-bookworm@sha256:fe22"
	// The same minor, rebuilt a week later.
	pg17Rebuilt = "ghcr.io/cloudnative-pg/postgresql:17.11-202610190828-standard-bookworm@sha256:aaaa"
	pg17Older   = "ghcr.io/cloudnative-pg/postgresql:17.9-202605110828-standard-bookworm@sha256:bbbb"
	pg18        = "ghcr.io/cloudnative-pg/postgresql:18.6-202610120828-standard-bookworm@sha256:a008"
)

// testdata holds CloudNativePG's catalog as published, which the Dockerfile
// fetches at build time.
func TestParseCatalog(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/catalog-standard-bookworm.yaml")
	require.NoError(t, err)

	images, err := ParseCatalog(data)
	require.NoError(t, err)
	majors := make([]int, 0, len(images))
	for _, image := range images {
		majors = append(majors, image.Major)
		assert.Contains(t, image.Image, "@sha256:", "digest-pinned")
	}
	// 18 needs operator 1.27.
	assert.Equal(t, []int{13, 14, 15, 16, 17}, majors)
}

func TestParseCatalogWithoutMaxMajor(t *testing.T) {
	t.Parallel()
	_, err := ParseCatalog([]byte("spec:\n  images:\n  - major: 16\n    image: " + pg16 + "\n"))
	assert.Error(t, err)
}

func TestMergeCatalog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current []CatalogImage
		shipped []CatalogImage
		want    []CatalogImage
	}{
		{"new", nil, []CatalogImage{{17, pg17}, {16, pg16}}, []CatalogImage{{16, pg16}, {17, pg17}}},
		{"raises a minor", []CatalogImage{{17, pg17Older}}, []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17}}},
		{"takes a rebuild", []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17Rebuilt}}, []CatalogImage{{17, pg17Rebuilt}}},
		// A plugin rollback ships an older catalog.
		{"keeps a newer minor", []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17Older}}, []CatalogImage{{17, pg17}}},
		{"keeps a newer build", []CatalogImage{{17, pg17Rebuilt}}, []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17Rebuilt}}},
		// A newer plugin's major that databases may refer to.
		{"keeps an extra major", []CatalogImage{{18, pg18}}, []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17}, {18, pg18}}},
		{"replaces a hand edit", []CatalogImage{{17, "postgres:latest"}}, []CatalogImage{{17, pg17}}, []CatalogImage{{17, pg17}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mergeCatalog(tt.current, tt.shipped))
		})
	}
}

func TestApplyCatalog(t *testing.T) {
	t.Parallel()
	kube := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	ctx := context.Background()

	require.NoError(t, ApplyCatalog(ctx, kube, []CatalogImage{{16, pg16}, {17, pg17}}))
	// A rollback: 17 stays at the newer image.
	require.NoError(t, ApplyCatalog(ctx, kube, []CatalogImage{{16, pg16}, {17, pg17Older}}))

	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(CatalogGVK)
	require.NoError(t, kube.Get(ctx, client.ObjectKey{Name: CatalogName}, catalog))
	images, _, err := unstructured.NestedSlice(catalog.Object, "spec", "images")
	require.NoError(t, err)
	assert.Equal(t, []any{
		map[string]any{"major": int64(16), "image": pg16},
		map[string]any{"major": int64(17), "image": pg17},
	}, images)
	assert.Equal(t, "fundament-cloudnativepg", catalog.GetLabels()["app.kubernetes.io/managed-by"])

	require.NoError(t, DeleteCatalog(ctx, kube))
	require.NoError(t, DeleteCatalog(ctx, kube), "deleting twice")
}
