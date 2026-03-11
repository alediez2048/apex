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
		_, ok := RequireAuth(w, r, cfg)
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
			approveTransfer(w, r, cfg, deps, id)
		case "reject":
			rejectTransfer(w, r, deps, id)
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

func approveTransfer(w http.ResponseWriter, r *http.Request, cfg *config.Config, deps pipeline.Deps, id string) {
	operatorID := r.Header.Get("X-Operator-ID")
	var body struct {
		OperatorID       string `json:"operator_id"`
		ContributionType string `json:"contribution_type"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if operatorID == "" {
		operatorID = body.OperatorID
	}
	if operatorID == "" {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "operator_id is required (header X-Operator-ID or body)", id, nil)
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

func rejectTransfer(w http.ResponseWriter, r *http.Request, deps pipeline.Deps, id string) {
	var body struct {
		OperatorID string `json:"operator_id"`
		Reason     string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	operatorID := r.Header.Get("X-Operator-ID")
	if operatorID == "" {
		operatorID = body.OperatorID
	}
	if operatorID == "" {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "operator_id is required (header X-Operator-ID or body)", id, nil)
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
