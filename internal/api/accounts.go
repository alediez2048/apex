package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/ledger"
	"github.com/alediez2048/apex/internal/store"
)

// AccountsHandler handles all /api/v1/accounts/* routes.
// GET /api/v1/accounts/{id}/balance → account balance
// GET /api/v1/accounts/{id}/ledger  → ledger entries
func AccountsHandler(cfg *config.Config, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", "", nil)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/accounts/")
		segments := strings.Split(path, "/")
		if len(segments) < 2 {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
			return
		}

		accountID := segments[0]
		resource := segments[1]

		switch resource {
		case "balance":
			getBalance(w, r, db, accountID)
		case "ledger":
			getLedger(w, db, accountID)
		default:
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "not found", "", nil)
		}
	}
}

func getBalance(w http.ResponseWriter, r *http.Request, db *sql.DB, accountID string) {
	bal, err := ledger.Balance(r.Context(), db, accountID)
	if err != nil {
		slog.Error("balance query failed", "error", err, "account_id", accountID)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "balance query failed", "", nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"account_id":   accountID,
		"balance_cents": bal,
	})
}

func getLedger(w http.ResponseWriter, db *sql.DB, accountID string) {
	entries, err := store.ListLedgerEntriesByAccount(db, accountID)
	if err != nil {
		slog.Error("list ledger entries failed", "error", err, "account_id", accountID)
		WriteError(w, http.StatusInternalServerError, domain.CodeSystemInternal, "ledger query failed", "", nil)
		return
	}
	WriteJSON(w, http.StatusOK, entries)
}
