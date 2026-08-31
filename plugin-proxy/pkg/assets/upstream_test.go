package assets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpstreamTarget_ConstantServiceInDerivedNamespace(t *testing.T) {
	ns, svc := upstreamTarget("acme--cert-manager")
	assert.Equal(t, "plugin-acme--cert-manager", ns)
	assert.Equal(t, "http:plugin:8080", svc)
}

func TestUpstreamTarget_LongNameUsesDerivedNamespace(t *testing.T) {
	// 57 chars: one past what "plugin-" + name can fit in a 63-char label.
	name := "acme--" + strings.Repeat("c", 51)
	ns, _ := upstreamTarget(name)
	assert.LessOrEqual(t, len(ns), 63)
	assert.NotEqual(t, "plugin-"+name, ns)
}
