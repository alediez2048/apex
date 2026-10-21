# Apex Mobile Check Deposit System

A mobile check deposit processing system, built for the Apex Fintech Solutions challenger project brief (an independent submission, not an Apex product). Single Go binary with embedded web UI, SQLite persistence, double-entry ledger, deterministic vendor simulation, and X9 ICL-structured settlement output.

## Quick Start

```bash
# Prerequisites: Go 1.22+, jq (for demo script)
git clone https://github.com/alediez2048/apex.git
cd apex

# Build, seed demo data, and start server
make dev

# Or run the full self-contained demo (build → start → demo → stop)
make demo-full
```

The server starts at **http://localhost:8080** with these routes:

| Route | Description |
|-------|-------------|
| `/` | Dashboard — deposit counts and KPIs |
| `/submit` | Submit a new check deposit |
| `/operator` | Operator review queue (risk-sorted) |
| `/transfers/:id` | Transfer detail with event timeline |
| `/api/v1/deposits` | REST API — list/create deposits |
| `/api/v1/operator/queue` | REST API — operator queue |
| `/api/v1/settlement/batches` | REST API — settlement batching |
| `/api/v1/returns` | REST API — return/reversal processing |
| `/health` | Health check |

## Available Commands

```
make           Show help
make dev       Build, seed, and start server
make build     Compile ./cmd/server to ./bin/server
make test      Run all tests (go test ./...)
make demo      Run 6-act demo script against running server
make demo-full Start server, run demo, stop server (self-contained)
make report    Generate test coverage report in /reports
make clean     Remove bin, data, and generated artifacts
```

## Architecture Summary

```
┌─────────────────────────────────────────────────────┐
│                   Go Binary (single)                │
│                                                     │
│  ┌─────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │ REST API │→│ Pipeline  │→│ Vendor Stub        │ │
│  │ (net/http)│ │ Runner   │  │ (7 scenarios)      │ │
│  └────┬─────┘  └────┬─────┘  └────────────────────┘ │
│       │              │                               │
│  ┌────▼─────┐  ┌────▼─────┐  ┌────────────────────┐ │
│  │ Embedded │  │ Funding  │  │ Settlement Engine  │ │
│  │ Web UI   │  │ Engine   │  │ (X9 JSON)          │ │
│  │ (go:embed)│ │ (rules)  │  └────────────────────┘ │
│  └──────────┘  └────┬─────┘                         │
│                ┌────▼─────┐  ┌────────────────────┐ │
│                │ Double-  │  │ Return/Reversal    │ │
│                │ Entry    │  │ ($30 fee)          │ │
│                │ Ledger   │  └────────────────────┘ │
│                └────┬─────┘                         │
│                ┌────▼─────┐                         │
│                │ SQLite   │                         │
│                │ (WAL)    │                         │
│                └──────────┘                         │
└─────────────────────────────────────────────────────┘
```

**Key design decisions:**
- **Go single binary** — zero runtime dependencies, `go:embed` for web assets
- **SQLite with WAL** — single-writer serialization, `BEGIN IMMEDIATE` for financial transactions
- **Double-entry ledger** — every posting creates balanced DEBIT/CREDIT pairs; invariant verified in tests
- **8-state lifecycle** — Requested → Validating → Analyzing → Approved → FundsPosted → Completed → Returned / Rejected
- **Deterministic vendor stub** — magic account prefixes (PASS-, BLUR-, GLARE-, MICR-, DUP-, MISMATCH-) for repeatable scenarios

See [docs/architecture.md](docs/architecture.md) for full system design and [docs/decision_log.md](docs/decision_log.md) for ADRs.

## Configuration

Copy `.env.example` to `.env` (done automatically by `make dev`):

```bash
PORT=8080           # Server port
ENV=development     # Environment (development/production)
DB_PATH=data/apex.db  # SQLite database path
```

Investor accounts and correspondents are configured in `config/investors.yaml` and `config/correspondents.yaml`.

## Test Suite

92 tests across 7 packages covering the PRD's 20 named test scenarios:

```bash
make test      # Run all tests
make report    # Generate coverage report
```

See [reports/](reports/) for test coverage and demo results after running `make demo-full`.

## Vendor Stub Scenarios

| Account Prefix | Outcome | HTTP | Description |
|---------------|---------|------|-------------|
| `PASS-*` | FundsPosted | 201 | Clean pass — auto-approved |
| `BLUR-*` | Rejected | 422 | IQA blur failure |
| `GLARE-*` | Rejected | 422 | IQA glare failure |
| `MICR-*` | Analyzing | 201 | MICR unreadable — flagged for operator review |
| `DUP-*` | Rejected | 422 | Duplicate detected |
| `MISMATCH-*` | Analyzing | 201 | OCR amount mismatch — flagged |

On the standalone vendor stub (`/api/v1/vendor/validate`), you can force a scenario with the `X-Vendor-Scenario` header. The deposit pipeline itself picks the scenario from the account prefix above. Values: `CLEAN_PASS`, `IQA_BLUR`, `IQA_GLARE`, `MICR_FAILURE`, `DUPLICATE`, `AMOUNT_MISMATCH`.

## How it was built

AI coding assistants wrote much of the first draft. The ledger invariants, state machine and settlement rules are pinned by the Go test suite (`go test -race ./...`, run in CI on every push), and the design trade-offs are recorded in [docs/decision_log.md](docs/decision_log.md).

## Disclaimers

- All data is synthetic. No real PII, check images, account numbers, or routing numbers are used.
- The vendor integration is entirely stubbed for demonstration purposes.
- SQLite's single-writer model is appropriate for MVP-scale throughput; production would migrate to PostgreSQL.
- The $5,000 deposit limit and $30 return fee are hardcoded constants for MVP.
- Weekend-only exclusion for business day calculation; Federal Reserve holiday calendar is not implemented.
