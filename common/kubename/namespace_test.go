package kubename

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestValidateNamespace_Accepts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"team-a", "billing", "a", "ns123", "0abc", "a-b-c"} {
		require.NoError(t, ValidateNamespace("proj", name), "expected %q to be valid", name)
	}
}

func TestValidateNamespace_Rejects(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"empty":            "",
		"uppercase":        "TeamA",
		"underscore":       "team_a",
		"leading hyphen":   "-team",
		"trailing hyphen":  "team-",
		"dot":              "team.a",
		"reserved default": "default",
		"reserved system":  "kube-system",
		"fundament-system": "fundament-system",
		"kube- prefix":     "kube-anything",
		"too long":         strings.Repeat("a", 64),
	}
	for desc, name := range tests {
		t.Run(desc, func(t *testing.T) {
			t.Parallel()
			require.Error(t, ValidateNamespace("proj", name), "expected %q (%s) to be rejected", name, desc)
		})
	}
}

// The budget is what's left of 63 chars after "tnt-<project>--".
func TestValidateNamespace_MaxLengthDependsOnProject(t *testing.T) {
	t.Parallel()
	for _, project := range []string{"a", "proj", strings.Repeat("p", 30), strings.Repeat("p", 56)} {
		limit := MaxNamespaceNameLength(project)
		assert.Equal(t, 63-len("tnt-")-len(project)-len("--"), limit)
		atLimit := strings.Repeat("n", limit)
		require.NoError(t, ValidateNamespace(project, atLimit), "project %q", project)
		err := ValidateNamespace(project, atLimit+"n")
		require.Error(t, err, "project %q", project)
		assert.Contains(t, err.Error(), "project name + namespace name may be at most 57")
	}
}

func TestGenerateNamespace_Readable(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "tnt-proj-a--web", GenerateNamespace("proj-a", "web"))
	assert.Equal(t, "tnt-billing--prod", GenerateNamespace("billing", "prod"))
}

// The prefix keeps project namespaces clear of system and plugin namespaces,
// the separator keeps projects clear of each other.
func TestGenerateNamespace_NoCollisions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "tnt-cert--manager", GenerateNamespace("cert", "manager"))
	assert.Equal(t, "tnt-kube--system", GenerateNamespace("kube", "system"))
	assert.NotEqual(t, GenerateNamespace("a-b", "c"), GenerateNamespace("a", "b-c"))
}

// Every name that passes ValidateNamespace for a DNS-1123 project name yields a
// valid DNS-1123 label.
func TestGenerateNamespace_StaysWithinDNS1123(t *testing.T) {
	t.Parallel()
	for _, project := range []string{"a", "0p", "platform-team", strings.Repeat("p", 30), strings.Repeat("p", 56)} {
		longest := strings.Repeat("n", MaxNamespaceNameLength(project))
		require.NoError(t, ValidateNamespace(project, longest))
		got := GenerateNamespace(project, longest)
		require.Len(t, got, validation.DNS1123LabelMaxLength, "project %q", project)
		require.Empty(t, validation.IsDNS1123Label(got), "generated %q is not a valid DNS-1123 label", got)
	}
}
