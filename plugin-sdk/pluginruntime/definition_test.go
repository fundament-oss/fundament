package pluginruntime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDefinition_ImageAndRBAC(t *testing.T) {
	data := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  imagePullPolicy: IfNotPresent
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates]
        verbs: [get, list]
`)
	def, err := ParseDefinition(data)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", def.Spec.Image)
	assert.Equal(t, "IfNotPresent", def.Spec.ImagePullPolicy)
	require.Len(t, def.Spec.Permissions.RBAC, 1)
	assert.Equal(t, []string{"cert-manager.io"}, def.Spec.Permissions.RBAC[0].APIGroups)
}

func TestParseDefinition_RejectsWrongKind(t *testing.T) {
	_, err := ParseDefinition([]byte("apiVersion: fundament.io/v1\nkind: Nope\n"))
	require.Error(t, err)
}

func TestParseDefinition_RejectsMutableTagImage(t *testing.T) {
	// A published definition must pin an immutable digest, not a mutable tag.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller:v1.17.2
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseDefinition_RejectsWrongLengthDigest(t *testing.T) {
	// A sha256 digest is exactly 64 hex chars; a truncated one must be rejected.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeef
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseDefinition_RejectsInvalidImagePullPolicy(t *testing.T) {
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  imagePullPolicy: always
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "imagePullPolicy")
}

func TestHashManifest_Stable(t *testing.T) {
	manifest := []byte("hello")
	assert.Equal(t, HashManifest(manifest), HashManifest(manifest))
	assert.True(t, len(HashManifest(manifest)) == len("sha256:")+64)
	assert.NotEqual(t, HashManifest([]byte("a")), HashManifest([]byte("b")))
}

func TestParseDefinition_RejectsMissingImage(t *testing.T) {
	// The image-free source template is NOT a valid PluginDefinition.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.image")
}

func TestParseSourceDefinition_AcceptsMissingImage(t *testing.T) {
	// The authored source template has no image; publish injects the digest.
	def, err := ParseSourceDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
`))
	require.NoError(t, err)
	assert.Equal(t, "cert-manager", def.Metadata.Name)
	assert.Empty(t, def.Spec.Image)
}

func TestParseSourceDefinition_RejectsMutableTag(t *testing.T) {
	// An image that IS present must still be digest-pinned, so a hand-written
	// mutable tag fails here rather than at publish time.
	_, err := ParseSourceDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  image: quay.io/jetstack/cert-manager-controller:v1.17.2
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseSourceDefinition_StillValidatesEnvelope(t *testing.T) {
	for name, manifest := range map[string]string{
		"apiVersion": `apiVersion: fundament.io/v2
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  permissions:
    rbac: []
`,
		"kind": `apiVersion: fundament.io/v1
kind: Plugin
metadata:
  name: cert-manager
spec:
  permissions:
    rbac: []
`,
		"metadata.name": `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  version: v1.0.0
spec:
  permissions:
    rbac: []
`,
		"unknown field": `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  permissionz:
    rbac: []
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSourceDefinition([]byte(manifest))
			require.Error(t, err)
		})
	}
}

// configSchemaManifest wraps a configSchema block in a minimal valid source
// definition, so each case below states only the schema under test.
func configSchemaManifest(schema string) []byte {
	return []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
  configSchema:
` + schema)
}

func TestParseSourceDefinition_AcceptsValidConfigSchema(t *testing.T) {
	def, err := ParseSourceDefinition(configSchemaManifest(`    - name: MON_COUNT
      displayName: Monitor count
      type: int
      default: "3"
      description: Number of Ceph monitors
    - name: ALLOW_MULTIPLE_PER_NODE
      type: bool
      default: "false"
      advanced: true
    - name: FAILURE_DOMAIN
      type: enum
      values: [host, osd]
      required: true
`))
	require.NoError(t, err)
	require.Len(t, def.Spec.ConfigSchema, 3)
	assert.Equal(t, "MON_COUNT", def.Spec.ConfigSchema[0].Name)
	assert.Equal(t, "Monitor count", def.Spec.ConfigSchema[0].DisplayName)
	assert.Equal(t, "int", def.Spec.ConfigSchema[0].Type)
	assert.Equal(t, "3", def.Spec.ConfigSchema[0].Default)
	assert.True(t, def.Spec.ConfigSchema[1].Advanced)
	assert.Equal(t, []string{"host", "osd"}, def.Spec.ConfigSchema[2].Values)
	assert.True(t, def.Spec.ConfigSchema[2].Required)
}

func TestParseSourceDefinition_RejectsBadConfigSchema(t *testing.T) {
	for name, tc := range map[string]struct {
		schema  string
		wantErr string
	}{
		"lowercase key name": {
			schema:  "    - name: mon_count\n      type: int\n",
			wantErr: "mon_count",
		},
		"duplicate key": {
			schema:  "    - name: MON_COUNT\n      type: int\n    - name: MON_COUNT\n      type: int\n",
			wantErr: "twice",
		},
		"unknown type": {
			schema:  "    - name: MON_COUNT\n      type: integer\n",
			wantErr: "integer",
		},
		"enum without values": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n",
			wantErr: "no values",
		},
		"values on non-enum": {
			schema:  "    - name: MON_COUNT\n      type: int\n      values: [\"1\", \"3\"]\n",
			wantErr: "not an enum",
		},
		"non-integer int default": {
			schema:  "    - name: MON_COUNT\n      type: int\n      default: three\n",
			wantErr: "not an integer",
		},
		"non-boolean bool default": {
			schema:  "    - name: DEV_LOOP_DEVICES\n      type: bool\n      default: yes\n",
			wantErr: "not a boolean",
		},
		"enum default outside values": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n      values: [host, osd]\n      default: rack\n",
			wantErr: "rack",
		},
		"required with default": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n      values: [host, osd]\n      default: host\n      required: true\n",
			wantErr: "mutually exclusive",
		},
		"bool without default": {
			schema:  "    - name: DEV_LOOP_DEVICES\n      type: bool\n",
			wantErr: "without a default",
		},
		"required bool": {
			schema:  "    - name: DEV_LOOP_DEVICES\n      type: bool\n      required: true\n",
			wantErr: "cannot be required",
		},
		"bool default spelled True": {
			schema:  "    - name: DEV_LOOP_DEVICES\n      type: bool\n      default: \"True\"\n",
			wantErr: "must be exactly",
		},
		"int default with explicit plus": {
			schema:  "    - name: MON_COUNT\n      type: int\n      default: \"+3\"\n",
			wantErr: "explicit '+'",
		},
		"required and advanced": {
			schema:  "    - name: FAILURE_DOMAIN\n      type: enum\n      values: [host, osd]\n      required: true\n      advanced: true\n",
			wantErr: "required and advanced",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSourceDefinition(configSchemaManifest(tc.schema))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateConfig(t *testing.T) {
	schema := []ConfigSchemaEntry{
		{Name: "MON_COUNT", Type: "int", Default: "3"},
		{Name: "DEV_LOOP_DEVICES", Type: "bool", Default: "false"},
		{Name: "FAILURE_DOMAIN", Type: "enum", Values: []string{"host", "osd"}, Required: true},
		{Name: "CEPH_IMAGE", Type: "string"},
	}

	for name, tc := range map[string]struct {
		config  map[string]string
		schema  []ConfigSchemaEntry
		wantErr string // empty means valid
	}{
		"no schema accepts anything (pre-schema back-compat)": {
			config: map[string]string{"WHATEVER": "yes"}, schema: nil,
		},
		"valid config": {
			config: map[string]string{"MON_COUNT": "1", "FAILURE_DOMAIN": "osd"}, schema: schema,
		},
		"undeclared key rejected": {
			config:  map[string]string{"MON_CONUT": "1", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: `"MON_CONUT" is not declared`,
		},
		"non-integer int rejected": {
			config:  map[string]string{"MON_COUNT": "three", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not an integer",
		},
		"empty int value rejected": {
			// Present-but-empty is a value, not an omission: "" would crash the
			// plugin's own env parsing, so it must fail here first.
			config:  map[string]string{"MON_COUNT": "", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not an integer",
		},
		"bad bool rejected": {
			config:  map[string]string{"DEV_LOOP_DEVICES": "yes", "FAILURE_DOMAIN": "host"},
			schema:  schema,
			wantErr: "not a boolean",
		},
		"enum outside values rejected": {
			config:  map[string]string{"FAILURE_DOMAIN": "rack"},
			schema:  schema,
			wantErr: "not one of",
		},
		"missing required rejected": {
			config:  map[string]string{"MON_COUNT": "1"},
			schema:  schema,
			wantErr: `"FAILURE_DOMAIN" is required`,
		},
		"whitespace-only required string rejected": {
			// A string type accepts any value at the per-key check, so this
			// exercises the dedicated post-presence whitespace check.
			config:  map[string]string{"CEPH_IMAGE": "   "},
			schema:  []ConfigSchemaEntry{{Name: "CEPH_IMAGE", Type: "string", Required: true}},
			wantErr: `"CEPH_IMAGE" is required`,
		},
		"empty config with only-default schema is valid": {
			config: nil,
			schema: []ConfigSchemaEntry{{Name: "MON_COUNT", Type: "int", Default: "3"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateConfig(tc.config, tc.schema)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}
