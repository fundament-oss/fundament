// Package connectlimit holds the request size cap shared by every Connect
// handler.
package connectlimit

import "connectrpc.com/connect"

// DefaultReadMaxBytes caps a request message both as read off the wire and
// after decompression, so a small gzip body cannot inflate past it.
const DefaultReadMaxBytes = 1 << 20

// ReadMaxBytes returns the default request cap. Pass it to every handler,
// gRPC reflection included; an RPC that needs more sets its own cap.
func ReadMaxBytes() connect.HandlerOption {
	return connect.WithReadMaxBytes(DefaultReadMaxBytes)
}
