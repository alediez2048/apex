package funding

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alediez2048/apex/internal/domain"
)

// DepositValidateRequest is the minimal body for funding-only validation (TICKET-005).
type DepositValidateRequest struct {
	AmountCents int64  `json:"amount_cents"`
	MICRRouting string `json:"micr_routing,omitempty"`
	MICRAccount string `json:"micr_account,omitempty"`
	CheckNumber string `json:"check_number,omitempty"`
}

// errorBody matches the structured error shape from api.ErrorBody (kept local to avoid import cycle).
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Handler returns an http.Handler that runs the funding engine (no persistence).
func Handler(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		apiKey := bearerToken(r.Header.Get("Authorization"))
		var req DepositValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
			return
		}
		ctx, err := engine.ValidateDeposit(apiKey, req.AmountCents, req.MICRRouting, req.MICRAccount, req.CheckNumber, "")
		if err != nil {
			writeDomainErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":                 true,
			"account_id":         ctx.Investor.AccountID,
			"correspondent_id":   ctx.Correspondent.ID,
			"omnibus_account_id": ctx.OmnibusID,
			"contribution_type":  ctx.ContributionType,
		})
	}
}

func bearerToken(h string) string {
	h = strings.TrimSpace(h)
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func writeDomainErr(w http.ResponseWriter, err error) {
	de, ok := err.(*domain.DomainError)
	if !ok {
		writeErr(w, http.StatusInternalServerError, domain.CodeSystemInternal, err.Error())
		return
	}
	status := http.StatusUnprocessableEntity
	switch de.Code() {
	case domain.CodeFundingAccountNotFound:
		status = http.StatusUnauthorized
	case domain.CodeFundingIneligible:
		status = http.StatusForbidden
	}
	writeErr(w, status, de.Code(), de.Error())
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Code: code, Message: msg})
}
