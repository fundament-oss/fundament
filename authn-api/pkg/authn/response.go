package authn

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// writeJSON writes a JSON response with the given status code.
func (s *AuthnServer) writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encoding JSON response: %w", err)
	}
	return nil
}

// writeErrorJSON writes an error JSON response, logging any write failure.
func (s *AuthnServer) writeErrorJSON(w http.ResponseWriter, status int, message string) {
	if err := s.writeJSON(w, status, authnhttp.ErrorResponse{Error: message}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
