package cli

import (
	"testing"
)

func TestParseUserIdentifier(t *testing.T) {
	tests := []struct {
		name         string
		identifier   string
		wantOrg      string
		wantUser     string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:       "valid identifier",
			identifier: "acme/alice",
			wantOrg:    "acme",
			wantUser:   "alice",
			wantErr:    false,
		},
		{
			name:       "valid identifier with dashes",
			identifier: "my-org/my-user",
			wantOrg:    "my-org",
			wantUser:   "my-user",
			wantErr:    false,
		},
		{
			name:       "multiple slashes takes first as separator",
			identifier: "org/user/extra",
			wantOrg:    "org",
			wantUser:   "user/extra",
			wantErr:    false,
		},
		{
			name:       "empty string",
			identifier: "",
			wantErr:    true,
			wantErrMsg: "invalid user identifier '': expected format <organization>/<user>",
		},
		{
			name:       "no slash",
			identifier: "orguser",
			wantErr:    true,
			wantErrMsg: "invalid user identifier 'orguser': expected format <organization>/<user>",
		},
		{
			name:       "empty organization",
			identifier: "/user",
			wantErr:    true,
			wantErrMsg: "invalid user identifier '/user': expected format <organization>/<user>",
		},
		{
			name:       "empty user",
			identifier: "org/",
			wantErr:    true,
			wantErrMsg: "invalid user identifier 'org/': expected format <organization>/<user>",
		},
		{
			name:       "only slash",
			identifier: "/",
			wantErr:    true,
			wantErrMsg: "invalid user identifier '/': expected format <organization>/<user>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, user, err := parseUserIdentifier(tt.identifier)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseUserIdentifier(%q) expected error, got nil", tt.identifier)
					return
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("parseUserIdentifier(%q) error = %q, want %q", tt.identifier, err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("parseUserIdentifier(%q) unexpected error: %v", tt.identifier, err)
				return
			}

			if org != tt.wantOrg {
				t.Errorf("parseUserIdentifier(%q) org = %q, want %q", tt.identifier, org, tt.wantOrg)
			}
			if user != tt.wantUser {
				t.Errorf("parseUserIdentifier(%q) user = %q, want %q", tt.identifier, user, tt.wantUser)
			}
		})
	}
}
