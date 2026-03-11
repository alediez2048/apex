package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
)

// ErrorBody is the structured error response per PRD §7.7.
type ErrorBody struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	TransferID string      `json:"transfer_id,omitempty"`
	Details    interface{} `json:"details,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a structured error response.
func WriteError(w http.ResponseWriter, status int, code, message, transferID string, details interface{}) {
	WriteJSON(w, status, ErrorBody{
		Code:       code,
		Message:    message,
		TransferID: transferID,
		Details:    details,
	})
}

// WriteDomainError maps a domain error (or generic error) to structured HTTP response.
func WriteDomainError(w http.ResponseWriter, err error, transferID string) {
	de, ok := err.(*domain.DomainError)
	if !ok {
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, err.Error(), transferID, nil)
		return
	}
	status := http.StatusUnprocessableEntity
	switch de.Code() {
	case domain.CodeFundingAccountNotFound:
		status = http.StatusUnauthorized
	case domain.CodeFundingIneligible:
		status = http.StatusForbidden
	case domain.CodeInvalidTransition:
		status = http.StatusConflict
	}
	WriteError(w, status, de.Code(), de.Error(), transferID, nil)
}

// RequireAuth extracts and validates a Bearer token against configured investor API keys.
// Returns the API key and true if valid; writes a 401 error and returns false if not.
func RequireAuth(w http.ResponseWriter, r *http.Request, cfg *config.Config) (string, bool) {
	token := BearerToken(r.Header.Get("Authorization"))
	if token == "" {
		WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key", "", nil)
		return "", false
	}
	for _, inv := range cfg.Investors {
		if inv.APIKey == token {
			return token, true
		}
	}
	WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key", "", nil)
	return "", false
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(h string) string {
	h = strings.TrimSpace(h)
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
