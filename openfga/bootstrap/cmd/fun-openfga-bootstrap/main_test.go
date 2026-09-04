package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
)

// A model that decodes to zero types is still valid and writes without error,
// creating a store with no usable types.
func TestDecodeShippedModel(t *testing.T) {
	model, err := decodeModel(authz.ModelDSL)
	require.NoError(t, err)

	assert.Equal(t, "1.1", model.SchemaVersion)
	assert.NotEmpty(t, model.TypeDefinitions, "the decoded model must carry type definitions")

	types := make([]string, 0, len(model.TypeDefinitions))
	for _, td := range model.TypeDefinitions {
		types = append(types, td.Type)
	}

	assert.Contains(t, types, "organization")
	assert.Contains(t, types, "cluster")
}

// A model must compare equal to itself once it has been through the server's
// representation, or the bootstrap writes a new version on every deploy.
func TestSameModelIsStableAcrossEncoding(t *testing.T) {
	a, err := decodeModel(authz.ModelDSL)
	require.NoError(t, err)

	b, err := decodeModel(authz.ModelDSL)
	require.NoError(t, err)

	same, err := sameModel(a, b)
	require.NoError(t, err)
	assert.True(t, same)
}
