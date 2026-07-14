package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/fundament-oss/fundament/common/gardener"
)

type fakeAccessProvider struct {
	access *gardener.ShootAccess
	err    error
}

func (f *fakeAccessProvider) AccessFor(_ context.Context, _ string) (*gardener.ShootAccess, error) {
	return f.access, f.err
}

func TestGardenerClusterAccess_MissingOrganizationLabel(t *testing.T) {
	access, err := NewGardenerClusterAccess(nil)
	require.NoError(t, err)
	access.cache = &fakeAccessProvider{access: &gardener.ShootAccess{OrganizationID: ""}}

	_, err = access.ForCluster(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), gardener.LabelOrganizationID)
}

func TestGardenerClusterAccess_ReturnsClientAndOrg(t *testing.T) {
	orgID := uuid.New()
	access, err := NewGardenerClusterAccess(nil)
	require.NoError(t, err)
	access.cache = &fakeAccessProvider{access: &gardener.ShootAccess{
		OrganizationID: orgID.String(),
		RESTConfig:     &rest.Config{Host: "https://shoot.example"},
	}}

	target, err := access.ForCluster(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, orgID, target.OrganizationID)
	assert.NotNil(t, target.Client)
}
