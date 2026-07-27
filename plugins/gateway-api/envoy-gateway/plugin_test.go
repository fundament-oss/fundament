package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifiedCRDsCoverStandardAndEnvoy(t *testing.T) {
	all := verifiedCRDs()

	// The 5 standard Gateway API resources.
	for _, name := range []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"grpcroutes.gateway.networking.k8s.io",
		"tcproutes.gateway.networking.k8s.io",
		"tlsroutes.gateway.networking.k8s.io",
	} {
		assert.Contains(t, all, name)
	}
	// Envoy policy CRDs + EnvoyProxy.
	for _, name := range []string{
		"envoyproxies.gateway.envoyproxy.io",
		"securitypolicies.gateway.envoyproxy.io",
		"backendtrafficpolicies.gateway.envoyproxy.io",
		"clienttrafficpolicies.gateway.envoyproxy.io",
	} {
		assert.Contains(t, all, name)
	}
}

func TestNewEnvoyGatewayPlugin(t *testing.T) {
	p, err := NewEnvoyGatewayPlugin()
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "eg", p.cfg.GatewayClassName)
}
