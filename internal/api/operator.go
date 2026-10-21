package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/pipeline"
	"github.com/alediez2048/apex/internal/store"
)

// OperatorHandler handles all /api/v1/operator/* routes.
// GET /api/v1/operator/queue → list flagged, POST .../queue/{id}/approve, POST .../queue/{id}/reject.
func OperatorHandler(cfg *config.Config, deps pipeline.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := RequireOperator(w, r, cfg)
		if !ok {
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/operator/")

		if path == "queue" || path == "queue/" {
			if r.Method != http.MethodGet {
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", "", nil)
				return
			}
			listQueue(w, r, deps)
			return
		}

		if !strings.HasPrefix(path, "queue/") {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
			return
		}

		rest := strings.TrimPrefix(path, "queue/")
		segments := strings.Split(rest, "/")
		if len(segments) < 2 {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
			return
		}

		id := segments[0]
		action := segments[1]

		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", id, nil)
			return
		}

		switch action {
		case "approve":
			approveTransfer(w, r, cfg, deps, id, principal)
		case "reject":
			rejectTransfer(w, r, deps, id, principal)
		default:
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", id, nil)
		}
	}
}

func listQueue(w http.ResponseWriter, r *http.Request, deps pipeline.Deps) {
	q := r.URL.Query()
	filters := store.TransferFilters{
		AccountID: q.Get("account"),
		From:      q.Get("from"),
		To:        q.Get("to"),
	}
	statusFilter := q.Get("status")
	if statusFilter != "" {
		filters.Status = statusFilter
	} else {
		filters.Status = string(domain.StateAnalyzing)
	}
	if err := filters.ValidateDateFilters(); err != nil {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), "", nil)
		return
	}

	transfers, err := store.ListTransfers(deps.DB, filters)
	if err != nil {
		slog.Error("list queue failed", "error", err)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "failed to list queue", "", nil)
		return
	}
	WriteJSON(w, http.StatusOK, transfersToJSON(transfers))
}

func approveTransfer(w http.ResponseWriter, r *http.Request, cfg *config.Config, deps pipeline.Deps, id string, p *Principal) {
	var body struct {
		OperatorID       string `json:"operator_id"`
		ContributionType string `json:"contribution_type"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	operatorID, ok := resolveOperatorID(w, r, p, body.OperatorID, id)
	if !ok {
		return
	}

	if err := pipeline.OperatorApprove(r.Context(), deps, cfg, id, operatorID, body.ContributionType); err != nil {
		WriteDomainError(w, err, id)
		return
	}

	t, _ := store.GetTransfer(deps.DB, id)
	status := string(domain.StateFundsPosted)
	if t != nil {
		status = string(t.Status)
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"transfer_id": id,
		"status":      status,
		"message":     "approved by operator:" + operatorID,
	})
}

func rejectTransfer(w http.ResponseWriter, r *http.Request, deps pipeline.Deps, id string, p *Principal) {
	var body struct {
		OperatorID string `json:"operator_id"`
		Reason     string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	operatorID, ok := resolveOperatorID(w, r, p, body.OperatorID, id)
	if !ok {
		return
	}
	reason := body.Reason
	if reason == "" {
		reason = "rejected by operator"
	}

	if err := pipeline.OperatorReject(r.Context(), deps, id, operatorID, reason); err != nil {
		WriteDomainError(w, err, id)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"transfer_id": id,
		"status":      string(domain.StateRejected),
		"message":     "rejected by operator:" + operatorID,
	})
}

// resolveOperatorID returns the operator identity recorded on the decision.
// It always comes from the authenticated token. A client may still send
// X-Operator-ID or operator_id, but only if it matches the token; anything
// else is refused rather than written to the audit trail.
func resolveOperatorID(w http.ResponseWriter, r *http.Request, p *Principal, bodyID, transferID string) (string, bool) {
	for _, claimed := range []string{r.Header.Get("X-Operator-ID"), bodyID} {
		if claimed != "" && claimed != p.OperatorID {
			WriteError(w, http.StatusForbidden, "FORBIDDEN", "operator_id does not match the authenticated operator", transferID, nil)
			return "", false
		}
	}
	return p.OperatorID, true
}
