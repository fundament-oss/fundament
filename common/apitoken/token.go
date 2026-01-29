package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"hash/crc32"
	"math/big"
	"strings"
)

const (
	// Prefix is the token prefix that identifies Fundament API tokens.
	Prefix = "fun_"
	// RandomLength is the number of random base62 characters in the token.
	RandomLength = 30
	// ChecksumLength is the number of base62 characters for the CRC32 checksum.
	ChecksumLength = 6
	// TotalLength is the total length of a token (prefix + random + checksum).
	TotalLength = 4 + RandomLength + ChecksumLength // = 40 chars
	// PrefixDisplayLength is the length of the prefix shown to users (fun_XXXX).
	PrefixDisplayLength = 8
)

// base62Chars is the alphabet for base62 encoding.
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateToken creates a new API token with format: fun_<30 random base62><6 char CRC32>.
// Returns the token string and its SHA256 hash for database storage.
func GenerateToken() (token string, hash []byte, err error) {
	// Generate 30 random base62 characters
	randomPart, err := generateBase62(RandomLength)
	if err != nil {
		return "", nil, fmt.Errorf("generate random: %w", err)
	}

	// Calculate CRC32 checksum of prefix + random
	partialToken := Prefix + randomPart
	checksum := crc32.ChecksumIEEE([]byte(partialToken))
	checksumStr := encodeBase62(uint64(checksum), ChecksumLength)

	// Full token
	token = partialToken + checksumStr

	// SHA256 hash for storage
	hashArr := sha256.Sum256([]byte(token))
	hash = hashArr[:]

	return token, hash, nil
}

// ValidateFormat checks token format and CRC32 checksum without database lookup.
// Returns nil if the token format is valid.
func ValidateFormat(token string) error {
	if len(token) != TotalLength {
		return fmt.Errorf("invalid token length: expected %d, got %d", TotalLength, len(token))
	}
	if !strings.HasPrefix(token, Prefix) {
		return fmt.Errorf("invalid token prefix")
	}

	// Validate base62 characters
	for i := len(Prefix); i < len(token); i++ {
		if !isBase62(token[i]) {
			return fmt.Errorf("invalid character at position %d", i)
		}
	}

	// Extract parts
	partialToken := token[:len(Prefix)+RandomLength]
	checksumStr := token[len(Prefix)+RandomLength:]

	// Verify CRC32
	expectedChecksum := crc32.ChecksumIEEE([]byte(partialToken))
	expectedStr := encodeBase62(uint64(expectedChecksum), ChecksumLength)

	if checksumStr != expectedStr {
		return fmt.Errorf("invalid checksum")
	}

	return nil
}

// Hash returns the SHA256 hash of a token for database lookup.
func Hash(token string) []byte {
	hashArr := sha256.Sum256([]byte(token))
	return hashArr[:]
}

// GetPrefix returns the displayable prefix (first 8 chars: fun_XXXX).
func GetPrefix(token string) string {
	if len(token) < PrefixDisplayLength {
		return token
	}
	return token[:PrefixDisplayLength]
}

// IsAPIToken checks if a string looks like an API token (starts with fun_).
func IsAPIToken(s string) bool {
	return strings.HasPrefix(s, Prefix)
}

// generateBase62 generates a string of random base62 characters.
// Uses crypto/rand.Int for uniform distribution (no modulo bias).
func generateBase62(length int) (string, error) {
	result := make([]byte, length)
	m := big.NewInt(62)
	for i := range length {
		n, err := rand.Int(rand.Reader, m)
		if err != nil {
			return "", fmt.Errorf("rand int: %w", err)
		}

		result[i] = base62Chars[n.Int64()]
	}
	return string(result), nil
}

// encodeBase62 encodes a uint64 to base62 with minimum length padding.
func encodeBase62(num uint64, minLen int) string {
	if num == 0 {
		return strings.Repeat("0", minLen)
	}
	result := make([]byte, 0, 12)
	for num > 0 {
		result = append(result, base62Chars[num%62])
		num /= 62
	}
	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	// Pad to minimum length
	for len(result) < minLen {
		result = append([]byte{'0'}, result...)
	}
	return string(result)
}

// isBase62 checks if a byte is a valid base62 character.
func isBase62(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
