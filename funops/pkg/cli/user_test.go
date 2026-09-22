package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUserRef(t *testing.T) {
	id := uuid.MustParse("019b4000-1000-7000-8000-000000000001")

	tests := []struct {
		name    string
		ref     string
		want    userRef
		wantErr string
	}{
		{
			name: "user id",
			ref:  id.String(),
			want: userRef{id: id},
		},
		{
			name: "email address",
			ref:  "alice@acme-corp.com",
			want: userRef{email: "alice@acme-corp.com"},
		},
		{
			name: "surrounding whitespace is ignored",
			ref:  "  alice@acme-corp.com ",
			want: userRef{email: "alice@acme-corp.com"},
		},
		{
			name:    "empty",
			ref:     "",
			wantErr: "user is required: pass a user ID or an email address",
		},
		{
			name:    "a name is neither",
			ref:     "alice",
			wantErr: `invalid user "alice": expected a user ID or an email address`,
		},
		{
			name:    "the old organization/user form",
			ref:     "acme/alice",
			wantErr: `invalid user "acme/alice": expected a user ID or an email address`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUserRef(tt.ref)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePermission(t *testing.T) {
	got, err := parsePermission("admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", string(got))

	got, err = parsePermission("viewer")
	require.NoError(t, err)
	assert.Equal(t, "viewer", string(got))

	_, err = parsePermission("owner")
	require.EqualError(t, err, `invalid permission "owner": expected viewer or admin`)
}

func TestFormatOrganizationNames(t *testing.T) {
	assert.Equal(t, "(none)", formatOrganizationNames(nil))
	assert.Equal(t, "acme-corp,globex", formatOrganizationNames([]string{"acme-corp", "globex"}))
}
