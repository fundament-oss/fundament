package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
)

func TestKubernetesMinorAtLeast(t *testing.T) {
	cases := []struct {
		gitVersion string
		want       bool
	}{
		{"v1.31.0", true},
		{"v1.31.5-eks-1234", true},
		{"v1.33.1+k3s1", true},
		{"v2.0.0", true},
		{"v1.30.6+k3s1", false},
		{"v1.29.0", false},
		{"1.31.0", true}, // missing leading "v"
	}
	for _, tc := range cases {
		t.Run(tc.gitVersion, func(t *testing.T) {
			got, err := kubernetesMinorAtLeast(tc.gitVersion, minKubernetesMinor)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestKubernetesMinorAtLeastInvalid(t *testing.T) {
	_, err := kubernetesMinorAtLeast("not-a-version", minKubernetesMinor)
	require.Error(t, err)
}

// fakeVersioner returns a canned ServerVersion for testing checkKubernetesVersion.
type fakeVersioner struct {
	info *version.Info
	err  error
}

func (f fakeVersioner) ServerVersion() (*version.Info, error) {
	return f.info, f.err
}

func TestCheckKubernetesVersionTooOld(t *testing.T) {
	err := checkKubernetesVersion(fakeVersioner{info: &version.Info{GitVersion: "v1.30.6+k3s1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires Kubernetes >= 1.31")
	assert.Contains(t, err.Error(), "v1.30.6+k3s1") // reports the actual cluster version
}

func TestCheckKubernetesVersionOK(t *testing.T) {
	err := checkKubernetesVersion(fakeVersioner{info: &version.Info{GitVersion: "v1.31.5+k3s1"}})
	assert.NoError(t, err)
}

func TestCheckKubernetesVersionDiscoveryError(t *testing.T) {
	err := checkKubernetesVersion(fakeVersioner{err: errors.New("boom")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server version")
}
