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

// Role is what an authenticated caller is allowed to do.
type Role string

const (
	RoleInvestor Role = "investor"
	RoleOperator Role = "operator"
)

// Principal is the authenticated caller.
type Principal struct {
	Role       Role
	APIKey     string
	AccountID  string // investors only
	OperatorID string // operators only
}

// IsOperator reports whether the caller is a back-office operator.
func (p *Principal) IsOperator() bool { return p != nil && p.Role == RoleOperator }

// CanSeeAccount reports whether the caller may read data for accountID.
// Operators see every account; investors see only their own.
func (p *Principal) CanSeeAccount(accountID string) bool {
	return p.IsOperator() || (p != nil && p.AccountID == accountID)
}

// Authenticate validates the Bearer token against configured investor and
// operator keys and writes 401 on failure.
func Authenticate(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*Principal, bool) {
	token := BearerToken(r.Header.Get("Authorization"))
	if token != "" {
		if inv := cfg.InvestorByAPIKey(token); inv != nil {
			return &Principal{Role: RoleInvestor, APIKey: token, AccountID: inv.AccountID}, true
		}
		if op := cfg.OperatorByAPIKey(token); op != nil {
			return &Principal{Role: RoleOperator, APIKey: token, OperatorID: op.ID}, true
		}
	}
	WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key", "", nil)
	return nil, false
}

// RequireOperator authenticates the caller and rejects anyone who is not an
// operator with 403.
func RequireOperator(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*Principal, bool) {
	p, ok := Authenticate(w, r, cfg)
	if !ok {
		return nil, false
	}
	if !p.IsOperator() {
		WriteError(w, http.StatusForbidden, "FORBIDDEN", "operator role required", "", nil)
		return nil, false
	}
	return p, true
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(h string) string {
	h = strings.TrimSpace(h)
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
