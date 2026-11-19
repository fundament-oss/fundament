// Package controller holds the Gardener extension that makes each shoot's
// Prometheus scrape the containers in tenant namespaces.
package controller

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fundament-oss/fundament/common/kubename"
)

const (
	// ExtensionType is the type of the Extension resources this controller owns.
	ExtensionType = "fundament-tenant-metrics"

	// sourceName is Gardener's cAdvisor job for a shoot, which keeps kube-system only.
	sourceName = "shoot-cadvisor"
	// targetName is the copy this controller keeps for tenant namespaces.
	targetName = "fundament-tenant-cadvisor"

	namespaceLabel = "namespace"
)

// errUnknownShape is returned when Gardener's cAdvisor job does not look the
// way this controller expects, typically after a Gardener upgrade.
var errUnknownShape = errors.New("unknown shape of " + sourceName)

var scrapeConfigGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1alpha1", Kind: "ScrapeConfig"}

// tenantNamespaceRegex matches the namespaces fundament creates for tenants.
var tenantNamespaceRegex = "^" + kubename.TenantNamespacePrefix + ".+$"

// tenantSpec derives the tenant scrape job from the spec of Gardener's own
// cAdvisor job: the same target, credentials and metric selection, with its own
// job name and the namespace filter moved from kube-system to tenant namespaces.
// Copying follows whatever the installed Gardener version configures.
func tenantSpec(source map[string]any) (map[string]any, error) {
	spec := runtime.DeepCopyJSON(source)

	jobs, err := rewrite(spec, "relabelings", func(rule map[string]any) bool {
		if rule["targetLabel"] != "job" {
			return false
		}
		rule["replacement"] = targetName
		return true
	})
	if err != nil {
		return nil, err
	}
	if jobs != 1 {
		return nil, fmt.Errorf("%w: expected one job relabeling, found %d", errUnknownShape, jobs)
	}

	filters, err := rewrite(spec, "metricRelabelings", func(rule map[string]any) bool {
		labels, _, _ := unstructured.NestedStringSlice(rule, "sourceLabels")
		if rule["action"] != "keep" || len(labels) != 1 || labels[0] != namespaceLabel {
			return false
		}
		rule["regex"] = tenantNamespaceRegex
		return true
	})
	if err != nil {
		return nil, err
	}
	if filters != 1 {
		return nil, fmt.Errorf("%w: expected one namespace filter, found %d", errUnknownShape, filters)
	}

	return spec, nil
}

// rewrite applies change to every rule in spec[field] and counts the changed ones.
func rewrite(spec map[string]any, field string, change func(rule map[string]any) bool) (int, error) {
	rules, found, err := unstructured.NestedSlice(spec, field)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", field, err)
	}
	if !found {
		return 0, fmt.Errorf("%w: %s missing", errUnknownShape, field)
	}

	var changed int
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			return 0, fmt.Errorf("%w: %s holds a %T", errUnknownShape, field, item)
		}
		if change(rule) {
			changed++
		}
	}

	err = unstructured.SetNestedSlice(spec, rules, field)
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", field, err)
	}
	return changed, nil
}
