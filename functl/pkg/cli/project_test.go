package cli

import (
	"testing"
)

func TestParseProjectIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		identifier  string
		wantOrg     string
		wantProject string
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "valid identifier",
			identifier:  "acme/myproject",
			wantOrg:     "acme",
			wantProject: "myproject",
			wantErr:     false,
		},
		{
			name:        "valid identifier with dashes",
			identifier:  "my-org/my-project",
			wantOrg:     "my-org",
			wantProject: "my-project",
			wantErr:     false,
		},
		{
			name:        "multiple slashes takes first as separator",
			identifier:  "org/project/extra",
			wantOrg:     "org",
			wantProject: "project/extra",
			wantErr:     false,
		},
		{
			name:       "empty string",
			identifier: "",
			wantErr:    true,
			wantErrMsg: "invalid project identifier '': expected format <organization>/<project>",
		},
		{
			name:       "no slash",
			identifier: "orgproject",
			wantErr:    true,
			wantErrMsg: "invalid project identifier 'orgproject': expected format <organization>/<project>",
		},
		{
			name:       "empty organization",
			identifier: "/project",
			wantErr:    true,
			wantErrMsg: "invalid project identifier '/project': expected format <organization>/<project>",
		},
		{
			name:       "empty project",
			identifier: "org/",
			wantErr:    true,
			wantErrMsg: "invalid project identifier 'org/': expected format <organization>/<project>",
		},
		{
			name:       "only slash",
			identifier: "/",
			wantErr:    true,
			wantErrMsg: "invalid project identifier '/': expected format <organization>/<project>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, project, err := parseProjectIdentifier(tt.identifier)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseProjectIdentifier(%q) expected error, got nil", tt.identifier)
					return
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("parseProjectIdentifier(%q) error = %q, want %q", tt.identifier, err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("parseProjectIdentifier(%q) unexpected error: %v", tt.identifier, err)
				return
			}

			if org != tt.wantOrg {
				t.Errorf("parseProjectIdentifier(%q) org = %q, want %q", tt.identifier, org, tt.wantOrg)
			}
			if project != tt.wantProject {
				t.Errorf("parseProjectIdentifier(%q) project = %q, want %q", tt.identifier, project, tt.wantProject)
			}
		})
	}
}
