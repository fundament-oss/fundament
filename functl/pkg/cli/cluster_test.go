package cli

import (
	"testing"
)

func TestParseClusterIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		identifier  string
		wantOrg     string
		wantCluster string
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "valid identifier",
			identifier:  "acme/production",
			wantOrg:     "acme",
			wantCluster: "production",
			wantErr:     false,
		},
		{
			name:        "valid identifier with dashes",
			identifier:  "my-org/my-cluster",
			wantOrg:     "my-org",
			wantCluster: "my-cluster",
			wantErr:     false,
		},
		{
			name:        "multiple slashes takes first as separator",
			identifier:  "org/cluster/extra",
			wantOrg:     "org",
			wantCluster: "cluster/extra",
			wantErr:     false,
		},
		{
			name:       "empty string",
			identifier: "",
			wantErr:    true,
			wantErrMsg: "invalid cluster identifier '': expected format <organization>/<cluster>",
		},
		{
			name:       "no slash",
			identifier: "orgcluster",
			wantErr:    true,
			wantErrMsg: "invalid cluster identifier 'orgcluster': expected format <organization>/<cluster>",
		},
		{
			name:       "empty organization",
			identifier: "/cluster",
			wantErr:    true,
			wantErrMsg: "invalid cluster identifier '/cluster': expected format <organization>/<cluster>",
		},
		{
			name:       "empty cluster",
			identifier: "org/",
			wantErr:    true,
			wantErrMsg: "invalid cluster identifier 'org/': expected format <organization>/<cluster>",
		},
		{
			name:       "only slash",
			identifier: "/",
			wantErr:    true,
			wantErrMsg: "invalid cluster identifier '/': expected format <organization>/<cluster>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, cluster, err := parseClusterIdentifier(tt.identifier)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseClusterIdentifier(%q) expected error, got nil", tt.identifier)
					return
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("parseClusterIdentifier(%q) error = %q, want %q", tt.identifier, err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("parseClusterIdentifier(%q) unexpected error: %v", tt.identifier, err)
				return
			}

			if org != tt.wantOrg {
				t.Errorf("parseClusterIdentifier(%q) org = %q, want %q", tt.identifier, org, tt.wantOrg)
			}
			if cluster != tt.wantCluster {
				t.Errorf("parseClusterIdentifier(%q) cluster = %q, want %q", tt.identifier, cluster, tt.wantCluster)
			}
		})
	}
}
