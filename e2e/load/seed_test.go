package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// A seeded session must pass the validator the services use, so a change to
// the claims shows here instead of as refused requests in a load run.
func TestSessionPassesTheServicesValidator(t *testing.T) {
	s := seeder{secret: []byte("test-secret")}
	userID, orgID := uuid.New(), uuid.New()
	token, err := s.session(userID, orgID, "load-01-1@fieldlab.test", time.Now())
	require.NoError(t, err)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	claims, err := auth.NewValidatorForAudience(s.secret, "fundament_auth", auth.ConsoleIssuer, auth.TokenTypeUser, nil).Validate(header)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID())
	assert.Equal(t, []uuid.UUID{orgID}, claims.OrganizationIDs)
}

func TestPickOfferTakesTheNewestVersion(t *testing.T) {
	region := organizationv1.Region_builder{
		Name:               "local",
		KubernetesVersions: []string{"1.33.10", "v1.34.1", "1.9.0", "1.34.1-rc.1", "latest"},
		MachineTypes:       []*organizationv1.RegionMachineType{organizationv1.RegionMachineType_builder{Name: "small"}.Build()},
	}.Build()
	o, err := pickOffer([]*organizationv1.Region{region})
	require.NoError(t, err)
	assert.Equal(t, &offer{region: "local", machineType: "small", version: "v1.34.1"}, o)

	_, err = pickOffer(nil)
	require.Error(t, err)
}
