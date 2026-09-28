package auth

import (
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationFromHeader(t *testing.T) {
	member := uuid.New()
	claims := &Claims{OrganizationIDs: []uuid.UUID{member}}

	header := func(value string) http.Header {
		h := http.Header{}
		if value != "" {
			h.Set(OrganizationHeader, value)
		}
		return h
	}

	got, err := OrganizationFromHeader(claims, header(member.String()))
	require.NoError(t, err)
	assert.Equal(t, member, got)

	for name, tc := range map[string]struct {
		value string
		code  connect.Code
	}{
		"missing":    {"", connect.CodeInvalidArgument},
		"malformed":  {"not-a-uuid", connect.CodeInvalidArgument},
		"not member": {uuid.NewString(), connect.CodePermissionDenied},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := OrganizationFromHeader(claims, header(tc.value))
			require.Error(t, err)
			assert.Equal(t, tc.code, connect.CodeOf(err))
		})
	}
}
