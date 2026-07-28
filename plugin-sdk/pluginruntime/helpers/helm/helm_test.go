package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRBACForbidden(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "secrets forbidden during install",
			output: `Error: query: failed to query with labels: secrets is forbidden: User "system:serviceaccount:plugin-cert-manager:plugin-cert-manager" cannot list resource "secrets" in API group "" in the namespace "cert-manager"`,
			want:   true,
		},
		{
			name:   "chart not found is not an RBAC error",
			output: `Error: failed to download "cert-manager" (hint: running helm repo update may help)`,
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRBACForbidden(tc.output))
		})
	}
}
