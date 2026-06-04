package kubename

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestValidateNamespace_Accepts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"team-a", "billing", "a", "ns123", "0abc", "a-b-c"} {
		require.NoError(t, ValidateNamespace(name), "expected %q to be valid", name)
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
		"too long":         "this-namespace-name-is-far-too-long-to-fit-within-the-fifty-char-budget",
	}
	for desc, name := range tests {
		t.Run(desc, func(t *testing.T) {
			t.Parallel()
			require.Error(t, ValidateNamespace(name), "expected %q (%s) to be rejected", name, desc)
		})
	}
}

func TestValidateNamespace_MaxLengthBoundary(t *testing.T) {
	t.Parallel()
	atLimit := strings.Repeat("a", MaxNamespaceNameLength)
	require.NoError(t, ValidateNamespace(atLimit))
	require.Error(t, ValidateNamespace(atLimit+"a"))
}

func TestGenerateNamespace_Deterministic(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	require.Equal(t, GenerateNamespace("Platform Team", id, "team-a"), GenerateNamespace("Platform Team", id, "team-a"))
}

func TestGenerateNamespace_DistinctPerProject(t *testing.T) {
	t.Parallel()
	// Two projects whose names sanitize identically must still produce distinct
	// cluster-side names for the same namespace name — this is the cross-project
	// collision guard.
	a := GenerateNamespace("Team A", uuid.New(), "billing")
	b := GenerateNamespace("team-a", uuid.New(), "billing")
	require.NotEqual(t, a, b)
}

// TestGenerateNamespace_StaysWithinDNS1123 verifies the budget: the longest
// accepted name combined with any project name yields a valid DNS-1123 label.
func TestGenerateNamespace_StaysWithinDNS1123(t *testing.T) {
	t.Parallel()
	longestName := strings.Repeat("a", MaxNamespaceNameLength)
	require.NoError(t, ValidateNamespace(longestName))

	for _, projectName := range []string{"", "x", "A Very Long Organization Project Name Indeed", "!!!"} {
		got := GenerateNamespace(projectName, uuid.New(), longestName)
		require.LessOrEqual(t, len(got), validation.DNS1123LabelMaxLength, "generated %q exceeds limit", got)
		require.Empty(t, validation.IsDNS1123Label(got), "generated %q is not a valid DNS-1123 label", got)
	}
}
