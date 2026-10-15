package shoot

import (
	"errors"
	"io"
	"net"
	"syscall"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsUnreachable reports whether err means the shoot's API server could not be
// reached or did not answer yet: a DNS or dial failure, a refused or dropped
// connection, a timeout, or the server saying it is unavailable. Errors that
// retrying cannot fix, such as a malformed URL wrapped in a *url.Error, are
// not. Gardener reports the API server available as soon as it answers from
// the seed; from here its external name may still take a little longer.
func IsUnreachable(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*net.OpError](err); ok {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return apierrors.IsServiceUnavailable(err) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err)
}
