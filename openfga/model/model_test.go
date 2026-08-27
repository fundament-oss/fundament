package model

import (
	"testing"

	language "github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/typesystem"
	"github.com/stretchr/testify/require"
)

// TestModelIsValid runs OpenFGA's own type checker over the model.
//
// Writing the model is the only validation OpenFGA offers, and it happens in a
// sidecar during a release: a model the server rejects surfaces as a crash-looping
// pod that takes openfga out of its Service, with the store left on the previous
// model. This keeps that out of a cluster.
func TestModelIsValid(t *testing.T) {
	parsed, err := language.TransformDSLToProto(DSL)
	require.NoError(t, err, "model.fga is not valid DSL")

	_, err = typesystem.NewAndValidate(t.Context(), parsed)
	require.NoError(t, err, "OpenFGA would reject model.fga")
}
