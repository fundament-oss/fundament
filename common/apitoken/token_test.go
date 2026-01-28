package apitoken

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Check token length
	if len(token) != TotalLength {
		t.Errorf("token length = %d, want %d", len(token), TotalLength)
	}

	// Check prefix
	if !strings.HasPrefix(token, Prefix) {
		t.Errorf("token does not start with prefix %q", Prefix)
	}

	// Check hash length (SHA256 = 32 bytes)
	if len(hash) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash))
	}

	// Validate the generated token passes format validation
	if err := ValidateFormat(token); err != nil {
		t.Errorf("ValidateFormat() on generated token failed: %v", err)
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	hashes := make(map[string]bool)

	for range 100 {
		token, hash, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error = %v", err)
		}

		if tokens[token] {
			t.Errorf("duplicate token generated: %s", token)
		}
		tokens[token] = true

		hashStr := string(hash)
		if hashes[hashStr] {
			t.Errorf("duplicate hash generated")
		}
		hashes[hashStr] = true
	}
}

func TestValidateFormat(t *testing.T) {
	// Generate a valid token for testing
	validToken, _, _ := GenerateToken()

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
		},
		{
			name:    "empty string",
			token:   "",
			wantErr: true,
		},
		{
			name:    "too short",
			token:   "fun_abc",
			wantErr: true,
		},
		{
			name:    "too long",
			token:   validToken + "x",
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			token:   "bad_" + validToken[4:],
			wantErr: true,
		},
		{
			name:    "invalid characters",
			token:   "fun_" + strings.Repeat("!", RandomLength+ChecksumLength),
			wantErr: true,
		},
		{
			name:    "invalid checksum",
			token:   "fun_" + strings.Repeat("a", RandomLength) + "000000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHash(t *testing.T) {
	token := "fun_testtoken123456789012345678901234"

	// Hash should be deterministic
	hash1 := Hash(token)
	hash2 := Hash(token)

	if !bytes.Equal(hash1, hash2) {
		t.Error("Hash() is not deterministic")
	}

	// Hash should be 32 bytes (SHA256)
	if len(hash1) != 32 {
		t.Errorf("Hash() length = %d, want 32", len(hash1))
	}

	// Different tokens should produce different hashes
	hash3 := Hash(token + "x")
	if bytes.Equal(hash1, hash3) {
		t.Error("different tokens produced same hash")
	}
}

func TestGetPrefix(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{
			name:  "full token",
			token: "fun_abcdefghijklmnopqrstuvwxyz1234567890",
			want:  "fun_abcd",
		},
		{
			name:  "exactly prefix length",
			token: "fun_abcd",
			want:  "fun_abcd",
		},
		{
			name:  "shorter than prefix length",
			token: "fun_",
			want:  "fun_",
		},
		{
			name:  "empty",
			token: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPrefix(tt.token); got != tt.want {
				t.Errorf("GetPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAPIToken(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "valid API token",
			s:    "fun_abcdefghijklmnopqrstuvwxyz1234567890",
			want: true,
		},
		{
			name: "just prefix",
			s:    "fun_",
			want: true,
		},
		{
			name: "JWT token",
			s:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.xxx",
			want: false,
		},
		{
			name: "empty string",
			s:    "",
			want: false,
		},
		{
			name: "different prefix",
			s:    "api_abcdefghijklmnopqrstuvwxyz1234567890",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAPIToken(tt.s); got != tt.want {
				t.Errorf("IsAPIToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBase62(t *testing.T) {
	// Test all valid base62 characters
	for _, c := range base62Chars {
		if !isBase62(byte(c)) {
			t.Errorf("isBase62(%c) = false, want true", c)
		}
	}

	// Test some invalid characters
	invalid := []byte{'!', '@', '#', '$', '%', '^', '&', '*', '-', '_', ' '}
	for _, c := range invalid {
		if isBase62(c) {
			t.Errorf("isBase62(%c) = true, want false", c)
		}
	}
}

func TestEncodeBase62(t *testing.T) {
	tests := []struct {
		num    uint64
		minLen int
		want   string
	}{
		{0, 1, "0"},
		{0, 6, "000000"},
		{1, 1, "1"},
		{61, 1, "z"},
		{62, 1, "10"},
		{62, 4, "0010"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := encodeBase62(tt.num, tt.minLen); got != tt.want {
				t.Errorf("encodeBase62(%d, %d) = %v, want %v", tt.num, tt.minLen, got, tt.want)
			}
		})
	}
}
