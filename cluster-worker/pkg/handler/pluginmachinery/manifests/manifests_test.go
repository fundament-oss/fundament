package manifests

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

const chartDir = "../../../../../charts/fundament"

// The embedded CRD is a generated copy of the chart file (go:generate cp).
// This test fails when the chart CRD changes without `just generate` being
// re-run, so the two can never silently drift apart.
func TestCRDMatchesChart(t *testing.T) {
	t.Parallel()
	chartCRD, err := os.ReadFile(chartDir + "/crds/plugininstallations.plugins.fundament.io.yaml")
	require.NoError(t, err)
	require.Equal(t, string(chartCRD), string(CRD),
		"embedded CRD differs from charts/fundament/crds/ — run `just generate` to refresh the copy")
}

// ClusterRoleRules must stay equivalent to the rules block of the chart's
// plugin-controller ClusterRole. The rules block in the Helm template is pure
// YAML (no templating), so it can be parsed straight out of the template file
// and compared: drift in either copy fails this test.
func TestClusterRoleRulesMatchChart(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(chartDir + "/templates/plugin-controller.yaml")
	require.NoError(t, err)

	var clusterRoleDoc string
	for _, doc := range strings.Split(string(content), "\n---") {
		if strings.Contains(doc, "kind: ClusterRole\n") {
			clusterRoleDoc = doc
			break
		}
	}
	require.NotEmpty(t, clusterRoleDoc, "no ClusterRole document found in the chart template")

	idx := strings.Index(clusterRoleDoc, "\nrules:")
	require.GreaterOrEqual(t, idx, 0, "no rules block found in the chart's ClusterRole")

	var parsed struct {
		Rules []rbacv1.PolicyRule `json:"rules"`
	}
	err = yaml.Unmarshal([]byte(clusterRoleDoc[idx+1:]), &parsed)
	require.NoError(t, err)

	require.Equal(t, parsed.Rules, ClusterRoleRules(),
		"ClusterRoleRules() differs from the chart template's rules block — keep charts/fundament/templates/plugin-controller.yaml and manifests.go in sync")
}

func TestDeploymentEnv(t *testing.T) {
	t.Parallel()
	d := Deployment(&DeploymentParams{
		Image:              "ghcr.io/fundament-oss/fundament/plugin-controller:v1",
		ClusterID:          "0f2be2a1-59b3-4a4b-96a5-77567d67ed49",
		OrganizationID:     "b25e3543-38fa-4f9a-92eb-a53b45b0a19d",
		OrganizationAPIURL: "https://api.fundament.example.com",
	})

	require.Len(t, d.Spec.Template.Spec.Containers, 1)
	c := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "ghcr.io/fundament-oss/fundament/plugin-controller:v1", c.Image)

	env := map[string]string{}
	for _, e := range c.Env {
		if e.ValueFrom != nil {
			env[e.Name] = "(fieldRef)"
			continue
		}
		env[e.Name] = e.Value
	}
	assert.Equal(t, map[string]string{
		"NAMESPACE":                 "(fieldRef)",
		"FUNDAMENT_CLUSTER_ID":      "0f2be2a1-59b3-4a4b-96a5-77567d67ed49",
		"FUNDAMENT_INSTALL_ID":      "0f2be2a1-59b3-4a4b-96a5-77567d67ed49",
		"FUNDAMENT_ORGANIZATION_ID": "b25e3543-38fa-4f9a-92eb-a53b45b0a19d",
		"ORGANIZATION_API_URL":      "https://api.fundament.example.com",
	}, env)
}

func TestDeploymentOptionalEnv(t *testing.T) {
	t.Parallel()
	d := Deployment(&DeploymentParams{
		Image:              "img",
		ClusterID:          "c",
		OrganizationID:     "o",
		OrganizationAPIURL: "u",
		AllowUnpinnedHash:  true,
		LogLevel:           "debug",
	})

	env := map[string]string{}
	for _, e := range d.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	assert.Equal(t, "true", env["PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH"])
	assert.Equal(t, "debug", env["LOG_LEVEL"])
}

func TestDeploymentShape(t *testing.T) {
	t.Parallel()
	d := Deployment(&DeploymentParams{Image: "img", ClusterID: "c", OrganizationID: "o", OrganizationAPIURL: "u"})

	assert.Equal(t, DeploymentName, d.Name)
	assert.Equal(t, Namespace, d.Namespace)
	assert.Equal(t, ManagedByValue, d.Labels[LabelManagedBy])
	assert.Equal(t, DeploymentName, d.Spec.Template.Spec.ServiceAccountName)
	// Selector mirrors the chart and must stay stable: it is immutable, and a
	// change forces EnsureDeployment into delete+recreate on every shoot.
	assert.Equal(t, map[string]string{"app.kubernetes.io/component": "plugin-controller"}, d.Spec.Selector.MatchLabels)

	c := d.Spec.Template.Spec.Containers[0]
	require.NotNil(t, c.LivenessProbe)
	assert.Equal(t, "/livez", c.LivenessProbe.HTTPGet.Path)
	require.NotNil(t, c.ReadinessProbe)
	assert.Equal(t, "/readyz", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, int32(8097), c.LivenessProbe.HTTPGet.Port.IntVal)
}

func TestDeploymentParamsValidate(t *testing.T) {
	t.Parallel()
	valid := DeploymentParams{Image: "img", ClusterID: "c", OrganizationID: "o", OrganizationAPIURL: "u"}
	require.NoError(t, valid.Validate())

	cases := []struct {
		name   string
		mutate func(*DeploymentParams)
	}{
		{"missing image", func(p *DeploymentParams) { p.Image = "" }},
		{"missing cluster id", func(p *DeploymentParams) { p.ClusterID = "" }},
		{"missing organization id", func(p *DeploymentParams) { p.OrganizationID = "" }},
		{"missing org-api URL", func(p *DeploymentParams) { p.OrganizationAPIURL = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := valid
			tc.mutate(&p)
			require.Error(t, p.Validate())
		})
	}
}

// The embedded CRD must actually parse as a CRD for the resource this feature
// is about — guards against an empty or corrupted generated copy.
func TestCRDParses(t *testing.T) {
	t.Parallel()
	var crd struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Group string `json:"group"`
			Scope string `json:"scope"`
		} `json:"spec"`
	}
	err := yaml.Unmarshal(CRD, &crd)
	require.NoError(t, err)
	assert.Equal(t, "CustomResourceDefinition", crd.Kind)
	assert.Equal(t, "plugininstallations.plugins.fundament.io", crd.Metadata.Name)
	assert.Equal(t, "plugins.fundament.io", crd.Spec.Group)
	assert.Equal(t, "Cluster", crd.Spec.Scope)
}

// Compile-time-ish sanity that Labels() carries the component label the
// Deployment selector relies on.
func TestLabels(t *testing.T) {
	t.Parallel()
	l := Labels()
	assert.Equal(t, "cluster-worker", l[LabelManagedBy])
	assert.Equal(t, "plugin-controller", l["app.kubernetes.io/component"])
}
