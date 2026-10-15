package shoot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsUnreachable(t *testing.T) {
	t.Parallel()

	dns := &url.Error{Op: "Get", URL: "https://api.example", Err: &net.DNSError{Err: "no such host", Name: "api.example", IsNotFound: true}}
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

	assert.True(t, IsUnreachable(fmt.Errorf("ensure namespace: %w", dns)))
	assert.True(t, IsUnreachable(fmt.Errorf("create sa: %w", refused)))
	assert.True(t, IsUnreachable(errors.Join(errors.New("other"), io.EOF)))
	assert.True(t, IsUnreachable(apierrors.NewServiceUnavailable("etcd not ready")))

	assert.False(t, IsUnreachable(nil))
	assert.False(t, IsUnreachable(context.Canceled))
	assert.False(t, IsUnreachable(apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "x", errors.New("no"))))
	assert.False(t, IsUnreachable(errors.New("parse kubeconfig: bad")))
	assert.False(t, IsUnreachable(&url.Error{Op: "Get", URL: "ftp://api.example", Err: errors.New("unsupported protocol scheme")}),
		"a *url.Error wrapping a permanent error is not a reachability problem")
}
