package api

import (
	"net/http"
	"strings"

	"github.com/alediez2048/apex/internal/config"
)

// SettlementHandler handles /api/v1/settlement/* routes — all stubs for TICKET-010.
func SettlementHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}
		WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"settlement endpoints are not yet implemented (TICKET-010)", "", nil)
	}
}

// ReturnsHandler handles /api/v1/returns/* routes — all stubs for TICKET-011.
func ReturnsHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := RequireAuth(w, r, cfg)
		if !ok {
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/returns")
		path = strings.TrimPrefix(path, "/")

		if path == "" && r.Method == http.MethodPost {
			WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				"returns endpoints are not yet implemented (TICKET-011)", "", nil)
			return
		}

		WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"returns endpoints are not yet implemented (TICKET-011)", "", nil)
	}
}
