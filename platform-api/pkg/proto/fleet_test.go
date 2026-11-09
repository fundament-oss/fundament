package proto_test

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func TestListClustersRequestValidation(t *testing.T) {
	t.Run("accepts an empty request", func(t *testing.T) {
		require.NoError(t, protovalidate.Validate(&fleetv1.ListClustersRequest{}))
	})

	t.Run("accepts an organization and a filter", func(t *testing.T) {
		org := "0198f3a1-7c2e-7b3a-9f1d-2c4e6a8b0d22"
		req := fleetv1.ListClustersRequest_builder{
			OrganizationId: &org,
			Filter:         fleetv1.ClusterFilter_CLUSTER_FILTER_STALE,
		}.Build()
		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid organization", func(t *testing.T) {
		org := "acme-corp"
		req := fleetv1.ListClustersRequest_builder{OrganizationId: &org}.Build()
		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects an undefined filter", func(t *testing.T) {
		req := fleetv1.ListClustersRequest_builder{Filter: fleetv1.ClusterFilter(99)}.Build()
		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestGetClusterRequestValidation(t *testing.T) {
	assert.Error(t, protovalidate.Validate(fleetv1.GetClusterRequest_builder{ClusterId: "nope"}.Build()))
	assert.Error(t, protovalidate.Validate(&fleetv1.GetClusterRequest{}), "the id is required")
}
