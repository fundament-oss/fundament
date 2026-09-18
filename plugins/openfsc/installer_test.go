package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitImageRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref, repository, tag string
	}{
		{"ghcr.io/fundament-oss/openfsc-operator:1.2.3", "ghcr.io/fundament-oss/openfsc-operator", "1.2.3"},
		{"ghcr.io/fundament-oss/openfsc-operator", "ghcr.io/fundament-oss/openfsc-operator", ""},
		// A registry port is not a tag: the colon sits before the last slash.
		{"localhost:5112/openfsc-operator", "localhost:5112/openfsc-operator", ""},
		{"localhost:5112/openfsc-operator:dev", "localhost:5112/openfsc-operator", "dev"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()
			repository, tag := splitImageRef(tt.ref)
			assert.Equal(t, tt.repository, repository)
			assert.Equal(t, tt.tag, tag)
		})
	}
}
