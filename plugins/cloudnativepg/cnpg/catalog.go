package cnpg

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

// CatalogName is the ClusterImageCatalog the plugin keeps, and the one every
// database it creates refers to by major version. CloudNativePG restarts a
// database when its catalog entry changes, which is how databases get a new
// minor: on a plugin upgrade that ships a newer catalog.
const CatalogName = "fundament-postgresql"

// MaxMajor is the newest PostgreSQL major operator 1.26 runs; 18 needs 1.27.
const MaxMajor = 17

// CatalogGVK is the kind of CatalogName.
var CatalogGVK = schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterImageCatalog"}

// CatalogImage is one entry of a ClusterImageCatalog: the image a database of
// that PostgreSQL major runs.
type CatalogImage struct {
	Major int    `json:"major"`
	Image string `json:"image"`
}

// ParseCatalog reads CloudNativePG's published ClusterImageCatalog (the image
// carries the one fetched at build time) and keeps the majors operator 1.26
// runs.
func ParseCatalog(data []byte) ([]CatalogImage, error) {
	var catalog struct {
		Spec struct {
			Images []CatalogImage `json:"images"`
		} `json:"spec"`
	}
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse image catalog: %w", err)
	}
	images := slices.DeleteFunc(catalog.Spec.Images, func(i CatalogImage) bool { return i.Major > MaxMajor })
	if !slices.ContainsFunc(images, func(i CatalogImage) bool { return i.Major == MaxMajor }) {
		return nil, fmt.Errorf("image catalog has no PostgreSQL %d", MaxMajor)
	}
	return images, nil
}

// ApplyCatalog creates or updates CatalogName with images, but only raises an
// entry: a plugin rollback must not move every database back to an older
// minor, so an entry newer than the one shipped stays, as do majors the
// shipped catalog lacks.
func ApplyCatalog(ctx context.Context, kube client.Client, images []CatalogImage) error {
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(CatalogGVK)
	catalog.SetName(CatalogName)
	if _, err := controllerutil.CreateOrUpdate(ctx, kube, catalog, func() error {
		var current []CatalogImage
		if raw, ok, _ := unstructured.NestedSlice(catalog.Object, "spec", "images"); ok {
			for _, entry := range raw {
				var image CatalogImage
				if m, ok := entry.(map[string]any); ok && runtime.DefaultUnstructuredConverter.FromUnstructured(m, &image) == nil {
					current = append(current, image)
				}
			}
		}
		entries := []any{}
		for _, image := range mergeCatalog(current, images) {
			entries = append(entries, map[string]any{"major": int64(image.Major), "image": image.Image})
		}
		catalog.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "fundament-cloudnativepg"})
		return unstructured.SetNestedSlice(catalog.Object, entries, "spec", "images")
	}); err != nil {
		return fmt.Errorf("apply ClusterImageCatalog %s: %w", CatalogName, err)
	}
	return nil
}

// DeleteCatalog deletes CatalogName, if it exists.
func DeleteCatalog(ctx context.Context, kube client.Client) error {
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(CatalogGVK)
	catalog.SetName(CatalogName)
	if err := kube.Delete(ctx, catalog); err != nil && !apierrors.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return fmt.Errorf("delete ClusterImageCatalog %s: %w", CatalogName, err)
	}
	return nil
}

// mergeCatalog returns shipped, with an entry of current kept where it is
// newer, plus the majors only current has, ordered by major.
func mergeCatalog(current, shipped []CatalogImage) []CatalogImage {
	byMajor := map[int]CatalogImage{}
	for _, image := range current {
		byMajor[image.Major] = image
	}
	for _, image := range shipped {
		if existing, ok := byMajor[image.Major]; ok && newerImage(existing.Image, image.Image) {
			continue
		}
		byMajor[image.Major] = image
	}
	merged := make([]CatalogImage, 0, len(byMajor))
	for _, image := range byMajor {
		merged = append(merged, image)
	}
	slices.SortFunc(merged, func(a, b CatalogImage) int { return cmp.Compare(a.Major, b.Major) })
	return merged
}

// newerImage reports whether image a is a later build than b. CloudNativePG
// tags them <major>.<minor>-<build timestamp>-<type>-<os>, e.g.
// 17.11-202610120828-standard-bookworm. A tag that does not parse is never
// newer, so a hand-edited entry is replaced.
func newerImage(a, b string) bool {
	va, errA := imageVersion(a)
	vb, errB := imageVersion(b)
	if errA != nil || errB != nil {
		return false
	}
	return slices.Compare(va, vb) > 0
}

// imageVersion returns the minor and build timestamp from an image's tag.
func imageVersion(image string) ([]int, error) {
	ref, _, _ := strings.Cut(image, "@")
	colon := strings.LastIndex(ref, ":")
	if colon < 0 || colon < strings.LastIndex(ref, "/") {
		return nil, errors.New("no tag")
	}
	fields := strings.Split(ref[colon+1:], "-")
	majorMinor := strings.Split(fields[0], ".")
	if len(fields) < 2 || len(majorMinor) != 2 {
		return nil, fmt.Errorf("tag %q is not <major>.<minor>-<build>", ref[colon+1:])
	}
	version := make([]int, 0, 3)
	for _, part := range []string{majorMinor[0], majorMinor[1], fields[1]} {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("tag %q: %w", ref[colon+1:], err)
		}
		version = append(version, n)
	}
	return version, nil
}
