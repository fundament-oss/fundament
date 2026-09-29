package organization

import (
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/common/kubename"
)

func TestKubeconfigEntryName(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("01a0cf34-278e-7bef-9d93-916ff8bdea4f")
	suffix := kubename.HashHex(id[:])[:4]

	tests := []struct {
		cluster string
		want    string
	}{
		{"production", "acme-corp--production"},
		{"eu-west-2", "acme-corp--eu-west-2"},
		{"Production", "acme-corp--production-" + suffix},
		{"my cluster/v2", "acme-corp--my-cluster-v2-" + suffix},
		{"  --Edge__Case--  ", "acme-corp--edge-case-" + suffix},
		{"prod--eu", "acme-corp--prod-eu-" + suffix},
		{"!!!", "acme-corp--" + suffix},
		{"Ünïcode", "acme-corp--n-code-" + suffix},
	}
	// URL- and path-safe, with "--" exactly once, between organization and cluster.
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*--[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, tt := range tests {
		got := kubeconfigEntryName("acme-corp", tt.cluster, id)
		assert.Equal(t, tt.want, got, tt.cluster)
		assert.Regexp(t, valid, got, "URL- and path-safe: %q", tt.cluster)
	}
}

// Two clusters whose free-form names reduce to the same slug must still get
// distinct entry names.
func TestKubeconfigEntryName_DistinctForCollidingSlugs(t *testing.T) {
	t.Parallel()
	a := kubeconfigEntryName("acme", "prod", uuid.New())
	b := kubeconfigEntryName("acme", "Prod", uuid.New())
	c := kubeconfigEntryName("acme", "PROD", uuid.New())
	assert.Equal(t, "acme--prod", a)
	assert.NotEqual(t, a, b)
	assert.NotEqual(t, b, c)
}

// A user in two organizations merges both kubeconfigs: pairs that would join
// to the same string with a single dash must stay apart.
func TestKubeconfigEntryName_DistinctAcrossOrganizations(t *testing.T) {
	t.Parallel()
	a := kubeconfigEntryName("acme", "prod-eu", uuid.New())
	b := kubeconfigEntryName("acme-prod", "eu", uuid.New())
	assert.Equal(t, "acme--prod-eu", a)
	assert.Equal(t, "acme-prod--eu", b)
	assert.NotEqual(t, a, b)
}
