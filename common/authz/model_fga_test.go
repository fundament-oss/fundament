package authz

import (
	"context"
	"os"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// storeFile is the subset of the OpenFGA store-file format this package uses:
// fixture tuples plus Check assertions.
type storeFile struct {
	Tuples []struct {
		User     string `yaml:"user"`
		Relation string `yaml:"relation"`
		Object   string `yaml:"object"`
	} `yaml:"tuples"`
	Tests []struct {
		Name  string `yaml:"name"`
		Check []struct {
			User       string          `yaml:"user"`
			Object     string          `yaml:"object"`
			Assertions map[string]bool `yaml:"assertions"`
		} `yaml:"check"`
	} `yaml:"tests"`
}

// TestModelSatisfiesStoreFile runs model_test.fga.yaml against an in-process
// OpenFGA, so a relation the services call cannot be removed or widened without
// a local test failing.
func TestModelSatisfiesStoreFile(t *testing.T) {
	raw, err := os.ReadFile("model_test.fga.yaml")
	require.NoError(t, err)

	var spec storeFile
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	require.NotEmpty(t, spec.Tests, "a store file with no tests would pass silently")

	ctx := context.Background()

	fga := server.MustNewServerWithOpts(server.WithDatastore(memory.New()))
	t.Cleanup(fga.Close)

	store, err := fga.CreateStore(ctx, &openfgav1.CreateStoreRequest{Name: "model-test"})
	require.NoError(t, err)

	model, err := transformer.TransformDSLToProto(string(ModelDSL))
	require.NoError(t, err)

	written, err := fga.WriteAuthorizationModel(ctx, &openfgav1.WriteAuthorizationModelRequest{
		StoreId:         store.GetId(),
		TypeDefinitions: model.GetTypeDefinitions(),
		SchemaVersion:   model.GetSchemaVersion(),
		Conditions:      model.GetConditions(),
	})
	require.NoError(t, err)

	keys := make([]*openfgav1.TupleKey, 0, len(spec.Tuples))
	for _, tuple := range spec.Tuples {
		keys = append(keys, &openfgav1.TupleKey{
			User: tuple.User, Relation: tuple.Relation, Object: tuple.Object,
		})
	}

	_, err = fga.Write(ctx, &openfgav1.WriteRequest{
		StoreId: store.GetId(),
		Writes:  &openfgav1.WriteRequestWrites{TupleKeys: keys},
	})
	require.NoError(t, err)

	for _, tc := range spec.Tests {
		t.Run(tc.Name, func(t *testing.T) {
			for _, check := range tc.Check {
				for relation, want := range check.Assertions {
					resp, err := fga.Check(ctx, &openfgav1.CheckRequest{
						StoreId:              store.GetId(),
						AuthorizationModelId: written.GetAuthorizationModelId(),
						TupleKey: &openfgav1.CheckRequestTupleKey{
							User: check.User, Relation: relation, Object: check.Object,
						},
					})
					require.NoError(t, err, "%s %s %s", check.User, relation, check.Object)
					assert.Equal(t, want, resp.GetAllowed(),
						"check(%s, %s, %s)", check.User, relation, check.Object)
				}
			}
		})
	}
}
