package gardener

import (
	"testing"

	"github.com/google/uuid"
)

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestProjectName(t *testing.T) {
	tests := []struct {
		name       string
		orgName    string
		wantPrefix string
		wantLen    int
	}{
		{
			name:       "normal org name",
			orgName:    "Acme Corp",
			wantPrefix: "acmeco",
			wantLen:    10,
		},
		{
			name:       "short org name gets padded",
			orgName:    "abc",
			wantPrefix: "abc",
			wantLen:    10,
		},
		{
			name:       "long org name gets truncated",
			orgName:    "very-long-organization-name",
			wantPrefix: "verylo",
			wantLen:    10,
		},
		{
			name:       "special chars removed",
			orgName:    "My-Org!@#$",
			wantPrefix: "myorg",
			wantLen:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectName(tt.orgName)

			if len(got) != tt.wantLen {
				t.Errorf("expected length %d, got %d (%q)", tt.wantLen, len(got), got)
			}

			if !hasPrefix(got, tt.wantPrefix) {
				t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
			}
		})
	}
}

func TestProjectName_Deterministic(t *testing.T) {
	name1 := ProjectName("Test Organization")
	name2 := ProjectName("Test Organization")

	if name1 != name2 {
		t.Errorf("ProjectName is not deterministic: %q != %q", name1, name2)
	}
}

func TestProjectName_DifferentOrgsProduceDifferentNames(t *testing.T) {
	name1 := ProjectName("Organization A")
	name2 := ProjectName("Organization B")

	if name1 == name2 {
		t.Errorf("different orgs produced same project name: %q", name1)
	}
}

func TestGenerateShootName(t *testing.T) {
	clusterID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tests := []struct {
		name        string
		clusterName string
		wantPrefix  string
		wantLen     int
	}{
		{
			name:        "normal cluster name",
			clusterName: "production",
			wantPrefix:  "producti",
			wantLen:     11,
		},
		{
			name:        "short cluster name gets padded",
			clusterName: "dev",
			wantPrefix:  "dev",
			wantLen:     11,
		},
		{
			name:        "long cluster name gets truncated",
			clusterName: "very-long-cluster-name",
			wantPrefix:  "verylong",
			wantLen:     11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateShootName(tt.clusterName, clusterID)

			if len(got) != tt.wantLen {
				t.Errorf("expected length %d, got %d (%q)", tt.wantLen, len(got), got)
			}

			if !hasPrefix(got, tt.wantPrefix) {
				t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
			}
		})
	}
}

func TestGenerateShootName_Deterministic(t *testing.T) {
	clusterID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	name1 := GenerateShootName("cluster", clusterID)
	name2 := GenerateShootName("cluster", clusterID)

	if name1 != name2 {
		t.Errorf("GenerateShootName is not deterministic: %q != %q", name1, name2)
	}
}

func TestGenerateShootName_DifferentIDsProduceDifferentNames(t *testing.T) {
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	name1 := GenerateShootName("cluster", id1)
	name2 := GenerateShootName("cluster", id2)

	if name1 == name2 {
		t.Errorf("different cluster IDs produced same shoot name: %q", name1)
	}
}

func TestNamingLengthConstraints(t *testing.T) {
	orgs := []string{"Acme Corp", "Very Long Organization Name", "a", "123 Corp"}
	clusters := []string{"production", "very-long-cluster-name", "a", "123-cluster"}

	for _, org := range orgs {
		for _, cluster := range clusters {
			clusterID := uuid.New()
			projectName := ProjectName(org)
			shootName := GenerateShootName(cluster, clusterID)
			combined := len(projectName) + len(shootName)

			if combined != MaxCombinedLength {
				t.Errorf("combined length should be exactly %d, got %d (project=%q [%d], shoot=%q [%d])",
					MaxCombinedLength, combined, projectName, len(projectName), shootName, len(shootName))
			}
		}
	}
}
