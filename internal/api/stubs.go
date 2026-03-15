package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/returns"
	"github.com/alediez2048/apex/internal/settlement"
	"github.com/alediez2048/apex/internal/store"
)

// SettlementHandler handles /api/v1/settlement/* routes (TICKET-010).
// POST /api/v1/settlement/batches [?as_of=<RFC3339>] — generate batch.
// GET /api/v1/settlement/batches/{id} — get batch file JSON.
// GET /api/v1/settlement/batches/{id}/items — list transfers in batch.
func SettlementHandler(cfg *config.Config, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/settlement")
		path = strings.TrimPrefix(path, "/")

		segments := strings.Split(path, "/")
		if len(segments) >= 1 && segments[0] == "batches" {
			if len(segments) == 1 {
				if r.Method == http.MethodPost {
					postSettlementBatches(w, r, db, cfg)
					return
				}
				WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", "", nil)
				return
			}
			batchID := segments[1]
			if len(segments) == 2 {
				if r.Method != http.MethodGet {
					WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", batchID, nil)
					return
				}
				getSettlementBatch(w, db, batchID)
				return
			}
			if len(segments) == 3 && segments[2] == "items" {
				if r.Method != http.MethodGet {
					WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", batchID, nil)
					return
				}
				getSettlementBatchItems(w, db, batchID)
				return
			}
		}

		WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
	}
}

func postSettlementBatches(w http.ResponseWriter, r *http.Request, db *sql.DB, _ *config.Config) {
	now := time.Now().UTC()
	if asOf := r.URL.Query().Get("as_of"); asOf != "" {
		t, err := time.Parse(time.RFC3339, asOf)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid as_of (use RFC3339)", "", nil)
			return
		}
		now = t.UTC()
	}
	batchID, settlementDate, fileJSON, err := settlement.GenerateBatch(db, now)
	if err != nil {
		slog.Error("settlement batch generation failed", "error", err)
		WriteError(w, http.StatusInternalServerError, "SYSTEM.INTERNAL", "failed to generate settlement batch", "", nil)
		return
	}
	if batchID == "" {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"message":         "no deposits to batch",
			"settlement_date": settlementDate,
			"batch_id":        nil,
			"file_control":    nil,
		})
		return
	}
	body := map[string]interface{}{
		"batch_id":        batchID,
		"settlement_date": settlementDate,
	}
	if fileJSON != "" {
		var fileObj interface{}
		if json.Unmarshal([]byte(fileJSON), &fileObj) == nil {
			body["file"] = fileObj
		}
	}
	WriteJSON(w, http.StatusCreated, body)
}

func getSettlementBatch(w http.ResponseWriter, db *sql.DB, batchID string) {
	b, err := store.GetBatch(db, batchID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "batch not found", batchID, nil)
			return
		}
		slog.Error("get settlement batch failed", "batch_id", batchID, "error", err)
		WriteError(w, http.StatusInternalServerError, "SYSTEM.INTERNAL", "failed to retrieve batch", batchID, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.FileJSON))
}

func getSettlementBatchItems(w http.ResponseWriter, db *sql.DB, batchID string) {
	list, err := store.ListTransfersBySettlementBatch(db, batchID)
	if err != nil {
		slog.Error("list settlement batch items failed", "batch_id", batchID, "error", err)
		WriteError(w, http.StatusInternalServerError, "SYSTEM.INTERNAL", "failed to list batch items", batchID, nil)
		return
	}
	out := make([]map[string]interface{}, len(list))
	for i, t := range list {
		out[i] = transferToJSON(t)
	}
	WriteJSON(w, http.StatusOK, out)
}

// ReturnsHandler handles /api/v1/returns routes (TICKET-011).
// POST /api/v1/returns — process a check return/reversal.
func ReturnsHandler(cfg *config.Config, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/returns")
		path = strings.TrimPrefix(path, "/")

		if path == "" && r.Method == http.MethodPost {
			postReturn(w, r, cfg, db)
			return
		}

		WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
	}
}

func postReturn(w http.ResponseWriter, r *http.Request, cfg *config.Config, db *sql.DB) {
	var body struct {
		TransferID string `json:"transfer_id"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", "", nil)
		return
	}
	if body.TransferID == "" {
		WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "transfer_id is required", "", nil)
		return
	}

	result, err := returns.ProcessReturn(r.Context(), db, cfg, body.TransferID, body.Reason)
	if err != nil {
		if returns.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "transfer not found", body.TransferID, nil)
			return
		}
		if returns.IsInvalidTransition(err) {
			WriteDomainError(w, err, body.TransferID)
			return
		}
		slog.Error("return processing failed", "transfer_id", body.TransferID, "error", err)
		WriteError(w, http.StatusInternalServerError, "SYSTEM.INTERNAL", "failed to process return", body.TransferID, nil)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}
