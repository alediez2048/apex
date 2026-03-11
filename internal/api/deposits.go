package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/pipeline"
	"github.com/alediez2048/apex/internal/store"
)

// DepositsHandler handles all /api/v1/deposits* routes.
// Routing: GET /api/v1/deposits → list, POST → create,
// GET /api/v1/deposits/{id} → get, GET .../history → history, GET .../images/{side} → image.
func DepositsHandler(cfg *config.Config, db *sql.DB, deps pipeline.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/deposits")
		path = strings.TrimPrefix(path, "/")

		if path == "" {
			switch r.Method {
			case http.MethodGet:
				listDeposits(w, r, db)
			case http.MethodPost:
				createDeposit(w, r, cfg, db, deps, apiKey)
			default:
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", "", nil)
			}
			return
		}

		if path == "validate" {
			return
		}

		segments := strings.Split(path, "/")
		id := segments[0]

		if len(segments) == 1 {
			if r.Method != http.MethodGet {
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", id, nil)
				return
			}
			getDeposit(w, db, id)
			return
		}

		switch segments[1] {
		case "history":
			if r.Method != http.MethodGet {
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", id, nil)
				return
			}
			getHistory(w, db, id)
		case "images":
			if r.Method != http.MethodGet {
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", id, nil)
				return
			}
			if len(segments) < 3 {
				WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "missing image side (front/back)", id, nil)
				return
			}
			getImage(w, r, id, segments[2])
		default:
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", id, nil)
		}
	}
}

func listDeposits(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	q := r.URL.Query()
	filters := store.TransferFilters{
		Status:    q.Get("status"),
		AccountID: q.Get("account"),
		From:      q.Get("from"),
		To:        q.Get("to"),
	}
	if err := filters.ValidateDateFilters(); err != nil {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), "", nil)
		return
	}
	transfers, err := store.ListTransfers(db, filters)
	if err != nil {
		slog.Error("list deposits failed", "error", err)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "failed to list deposits", "", nil)
		return
	}
	WriteJSON(w, http.StatusOK, transfersToJSON(transfers))
}

func createDeposit(w http.ResponseWriter, r *http.Request, cfg *config.Config, db *sql.DB, deps pipeline.Deps, apiKey string) {
	var body struct {
		AccountID   string `json:"account_id"`
		AmountCents int64  `json:"amount_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON", "", nil)
		return
	}
	inv := cfg.InvestorByAccountID(body.AccountID)
	if inv == nil {
		WriteError(w, http.StatusBadRequest, domain.CodeFundingAccountNotFound, "unknown account_id", "", nil)
		return
	}
	txID := newTransferID()
	t := &domain.Transfer{
		ID:                txID,
		InvestorAccountID: body.AccountID,
		CorrespondentID:   inv.CorrespondentID,
		Amount:            domain.Amount(body.AmountCents),
		Status:            domain.StateRequested,
		CheckNumber:       "",
	}
	if err := store.CreateTransfer(db, t); err != nil {
		slog.Error("create transfer failed", "error", err)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "internal error", "", nil)
		return
	}
	key := apiKey
	if key == "" {
		key = inv.APIKey
	}
	res, err := pipeline.Run(r.Context(), deps, txID, key)
	if err != nil {
		slog.Error("pipeline failed", "error", err, "transfer_id", txID)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "deposit processing failed", txID, nil)
		return
	}
	status := http.StatusCreated
	if res.Action == pipeline.HaltRejected {
		status = http.StatusUnprocessableEntity
	}
	WriteJSON(w, status, map[string]interface{}{
		"transfer_id": txID,
		"status":      res.Status,
		"action":      res.Action.String(),
		"reason":      res.Reason,
	})
}

func getDeposit(w http.ResponseWriter, db *sql.DB, id string) {
	t, err := store.GetTransfer(db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "transfer not found", id, nil)
			return
		}
		slog.Error("get transfer failed", "error", err)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "internal error", id, nil)
		return
	}
	WriteJSON(w, http.StatusOK, transferToJSON(t))
}

func getHistory(w http.ResponseWriter, db *sql.DB, id string) {
	_, err := store.GetTransfer(db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "transfer not found", id, nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "internal error", id, nil)
		return
	}
	events, err := store.ListEvents(db, id)
	if err != nil {
		slog.Error("list events failed", "error", err)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "internal error", id, nil)
		return
	}
	WriteJSON(w, http.StatusOK, events)
}

func getImage(w http.ResponseWriter, r *http.Request, transferID, side string) {
	if side != "front" && side != "back" {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "side must be front or back", transferID, nil)
		return
	}

	filename := side + ".png"
	perTransfer := filepath.Join("data", "images", transferID, filename)
	if _, err := os.Stat(perTransfer); err == nil {
		http.ServeFile(w, r, perTransfer)
		return
	}

	stub := filepath.Join("data", "images", "stub_"+filename)
	if _, err := os.Stat(stub); err == nil {
		http.ServeFile(w, r, stub)
		return
	}

	WriteError(w, http.StatusNotFound, "NOT_FOUND", "image not found", transferID, nil)
}

func newTransferID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "tx-" + hex.EncodeToString(b[:])
}

func transferToJSON(t *domain.Transfer) map[string]interface{} {
	m := map[string]interface{}{
		"transfer_id":        t.ID,
		"investor_account_id": t.InvestorAccountID,
		"correspondent_id":   t.CorrespondentID,
		"amount_cents":       int64(t.Amount),
		"status":             string(t.Status),
		"check_number":       t.CheckNumber,
		"created_at":         t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":         t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if t.VendorTransactionID != "" {
		m["vendor_transaction_id"] = t.VendorTransactionID
	}
	if t.RiskScore != nil {
		m["risk_score"] = *t.RiskScore
	}
	if t.ContributionType != "" {
		m["contribution_type"] = t.ContributionType
	}
	if t.SettlementBatchID != "" {
		m["settlement_batch_id"] = t.SettlementBatchID
	}
	return m
}

func transfersToJSON(ts []*domain.Transfer) []map[string]interface{} {
	out := make([]map[string]interface{}, len(ts))
	for i, t := range ts {
		out[i] = transferToJSON(t)
	}
	return out
}
