package dcimauthn

import (
	"io"
	"log/slog"
	"testing"
)

func testServer(frontendURL string) *Server {
	return &Server{
		config: &Config{FrontendURL: frontendURL},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestSubjectUUID_Deterministic(t *testing.T) {
	a := subjectUUID("CgNkZXg")
	b := subjectUUID("CgNkZXg")
	if a != b {
		t.Fatalf("subjectUUID not deterministic: %v != %v", a, b)
	}
	if c := subjectUUID("other"); c == a {
		t.Fatal("subjectUUID collided for different subjects")
	}
}

func TestState_RoundTrip(t *testing.T) {
	state, err := generateState("https://dcim.fundament.localhost:8443/racks")
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	data, err := parseState(state)
	if err != nil {
		t.Fatalf("parseState: %v", err)
	}
	if data.ReturnTo != "https://dcim.fundament.localhost:8443/racks" {
		t.Errorf("return_to = %q, want round-tripped value", data.ReturnTo)
	}
	if data.Nonce == "" {
		t.Error("expected non-empty nonce")
	}
}

func TestGenerateState_UniqueNonce(t *testing.T) {
	s1, _ := generateState("")
	s2, _ := generateState("")
	if s1 == s2 {
		t.Fatal("expected unique state values")
	}
}

func TestIsSafeReturnTo(t *testing.T) {
	const frontend = "https://dcim.fundament.localhost:8443"
	s := testServer(frontend)

	tests := []struct {
		name     string
		returnTo string
		want     bool
	}{
		{"same origin path", frontend + "/racks", true},
		{"same origin root", frontend, true},
		{"cross origin", "https://evil.com/phish", false},
		{"scheme downgrade", "http://dcim.fundament.localhost:8443/racks", false},
		{"different port", "https://dcim.fundament.localhost:9999/racks", false},
		{"protocol relative", "//evil.com", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.isSafeReturnTo(tt.returnTo); got != tt.want {
				t.Errorf("isSafeReturnTo(%q) = %v, want %v", tt.returnTo, got, tt.want)
			}
		})
	}
}

func TestGetRedirectURL_RejectsOpenRedirect(t *testing.T) {
	const frontend = "https://dcim.fundament.localhost:8443"
	s := testServer(frontend)

	state, err := generateState("https://evil.com/phish")
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	if got := s.getRedirectURL(state); got != frontend {
		t.Errorf("getRedirectURL = %q, want fallback %q", got, frontend)
	}

	safe := frontend + "/inventory"
	state, _ = generateState(safe)
	if got := s.getRedirectURL(state); got != safe {
		t.Errorf("getRedirectURL = %q, want %q", got, safe)
	}
}
