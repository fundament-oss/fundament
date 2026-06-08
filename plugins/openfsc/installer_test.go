package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPEMBlock(t *testing.T) {
	t.Parallel()

	// Mimics values-demo.yaml: PEM blocks indented under a YAML key.
	content := `global:
  certificates:
    group:
      caCertificatePEM: |
        -----BEGIN CERTIFICATE-----
        MIIBfakecertdata
        -----END CERTIFICATE-----
ca:
  issuer:
    keyPEM: |
      -----BEGIN EC PRIVATE KEY-----
      MHcfakekeydata
      -----END EC PRIVATE KEY-----
`

	crt, err := extractPEMBlock(content, "-----BEGIN CERTIFICATE-----", "-----END CERTIFICATE-----")
	require.NoError(t, err)
	assert.Equal(t, "-----BEGIN CERTIFICATE-----\nMIIBfakecertdata\n-----END CERTIFICATE-----\n", crt)
	// Dedented: no leading whitespace remains.
	for _, line := range strings.Split(strings.TrimSpace(crt), "\n") {
		assert.Equal(t, line, strings.TrimLeft(line, " \t"))
	}

	key, err := extractPEMBlock(content, "-----BEGIN EC PRIVATE KEY-----", "-----END EC PRIVATE KEY-----")
	require.NoError(t, err)
	assert.Contains(t, key, "MHcfakekeydata")
}

func TestExtractPEMBlockMissing(t *testing.T) {
	t.Parallel()
	_, err := extractPEMBlock("no pem here", "-----BEGIN CERTIFICATE-----", "-----END CERTIFICATE-----")
	require.Error(t, err)
}
