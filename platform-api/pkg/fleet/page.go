package fleet

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"
)

// Page sizes of the list RPCs: a request for more than maxPageSize gets
// maxPageSize, and one that names none gets defaultPageSize.
const (
	defaultPageSize = 100
	maxPageSize     = 500
)

func pageSize(requested int32) int32 {
	switch {
	case requested <= 0:
		return defaultPageSize
	case requested > maxPageSize:
		return maxPageSize
	default:
		return requested
	}
}

// A page token is the sort key of the last item on the page, base64 of its
// JSON: opaque to clients, and the next page reads the rows after it.
func encodePageToken(key any) string {
	b, err := json.Marshal(key)
	if err != nil {
		panic(err) // the keys are plain structs of strings
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodePageToken reads a token into key; an empty token leaves key alone and
// reports false.
func decodePageToken(token string, key any) (bool, error) {
	if token == "" {
		return false, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || json.Unmarshal(b, key) != nil {
		return false, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	return true, nil
}
