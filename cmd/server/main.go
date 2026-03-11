package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alediez2048/apex/internal/api"
	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/funding"
	"github.com/alediez2048/apex/internal/pipeline"
	"github.com/alediez2048/apex/internal/store"
	"github.com/alediez2048/apex/internal/vendor"
)

//go:embed web/index.html web/app.js web/style.css
var webFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	slog.Info("config loaded",
		"port", cfg.Port,
		"env", cfg.Env,
		"db_path", cfg.DBPath,
		"correspondents", len(cfg.Correspondents),
		"investors", len(cfg.Investors),
	)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := store.RunMigrations(db); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	if err := vendor.EnsureStubImages("data/images"); err != nil {
		slog.Error("stub images failed", "error", err)
		os.Exit(1)
	}

	vs := vendor.NewStub()
	dup := &funding.SQLDuplicateChecker{DB: db}
	fundingEngine := funding.NewEngine(cfg, dup)
	pipeDeps := pipeline.Deps{DB: db, VendorStub: vs, FundingEngine: fundingEngine}

	mux := http.NewServeMux()

	// Deposits: list/create/get/history/images
	mux.HandleFunc("/api/v1/deposits/", api.DepositsHandler(cfg, db, pipeDeps))
	mux.HandleFunc("/api/v1/deposits", api.DepositsHandler(cfg, db, pipeDeps))

	// Funding validation (standalone, from TICKET-005)
	mux.HandleFunc("/api/v1/deposits/validate", funding.Handler(fundingEngine))

	// Vendor validation (standalone)
	mux.HandleFunc("/api/v1/vendor/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", "", nil)
			return
		}
		var body struct {
			AccountID   string `json:"account_id"`
			AmountCents int64  `json:"amount_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON", "", nil)
			return
		}
		resp := vs.Validate(vendor.Request{AccountID: body.AccountID, AmountCents: body.AmountCents}, r.Header.Get(vendor.HeaderScenario))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Operator queue: list/approve/reject
	mux.HandleFunc("/api/v1/operator/", api.OperatorHandler(cfg, pipeDeps))

	// Accounts: balance/ledger
	mux.HandleFunc("/api/v1/accounts/", api.AccountsHandler(cfg, db))

	// Settlement stubs (TICKET-010)
	mux.HandleFunc("/api/v1/settlement/", api.SettlementHandler(cfg))

	// Returns stubs (TICKET-011)
	mux.HandleFunc("/api/v1/returns/", api.ReturnsHandler(cfg))
	mux.HandleFunc("/api/v1/returns", api.ReturnsHandler(cfg))

	// Stats for dashboard (TICKET-009)
	mux.HandleFunc("/api/v1/stats/dashboard", api.StatsHandler(cfg, db))

	// Health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Embedded web UI (TICKET-009): SPA and static assets.
	// Go's ServeMux matches "/" only for exact path "/"; register each route explicitly.
	webRoot, _ := fs.Sub(webFS, "web")
	wh := webHandler(cfg, webRoot)
	mux.HandleFunc("/", wh)
	mux.HandleFunc("/submit", wh)
	mux.HandleFunc("/operator", wh)
	mux.HandleFunc("/operator/", wh)
	mux.HandleFunc("/transfers/", wh)
	mux.HandleFunc("/app.js", wh)
	mux.HandleFunc("/style.css", wh)

	slog.Info("routes registered",
		"deposits", "/api/v1/deposits",
		"deposits_validate", "/api/v1/deposits/validate",
		"vendor_validate", "/api/v1/vendor/validate",
		"operator_queue", "/api/v1/operator/queue",
		"accounts", "/api/v1/accounts/{id}",
		"settlement", "/api/v1/settlement/*",
		"returns", "/api/v1/returns/*",
	)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		slog.Info("server listening", "addr", srv.Addr, "url", "http://localhost:"+cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")
	if err := srv.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// webHandler serves the embedded SPA and static assets; injects demo API key into index.html.
func webHandler(cfg *config.Config, webRoot fs.FS) http.HandlerFunc {
	demoKey := ""
	if len(cfg.Investors) > 0 {
		demoKey = cfg.Investors[0].APIKey
	}
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		path = strings.TrimSuffix(path, "/") // so /operator/ and /operator both work
		// SPA routes: serve index.html with key injected
		isAppRoute := path == "" || path == "submit" || path == "operator" ||
			strings.HasPrefix(path, "transfers/")
		if isAppRoute {
			index, err := fs.ReadFile(webRoot, "index.html")
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			html := strings.ReplaceAll(string(index), "__DEMO_API_KEY__", demoKey)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(html))
			return
		}
		// Static assets
		if path == "app.js" || path == "style.css" {
			data, err := fs.ReadFile(webRoot, path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if path == "app.js" {
				w.Header().Set("Content-Type", "application/javascript")
			} else {
				w.Header().Set("Content-Type", "text/css")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}
}
