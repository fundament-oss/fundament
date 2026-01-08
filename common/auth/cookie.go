package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// VerifyCookieValue verifies a signed cookie value and returns the original value.
// Format is: <value>.<signature> where value may contain dots (e.g., JWT).
func VerifyCookieValue(signedValue string, secret []byte) (string, error) {
	// Find the last dot to separate value from signature
	lastDot := strings.LastIndexByte(signedValue, '.')
	if lastDot == -1 {
		return "", fmt.Errorf("invalid signed value format: no signature separator")
	}

	value := signedValue[:lastDot]
	signature := signedValue[lastDot+1:]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	expectedSig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid signature")
	}

	return value, nil
}
