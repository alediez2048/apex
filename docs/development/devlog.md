# Mobile Check Deposit System — Development Log

**Project:** Mobile Check Deposit System for Apex Fintech Services  
**Sprint:** Mar 9-27, 2026 (Phase 1: Foundation & Core Processing) | Mar 16-22, 2026 (Phase 2: Review, Settlement & Returns) | Mar 23-27, 2026 (Phase 3: Demo, QA & Documentation)  
**Developer:** JAD  
**AI Assistant:** Cursor Agent (GPT-5.4)

---

## Timeline

| Phase | Days | Target |
|-------|------|--------|
| Phase 1 — Foundation & Core Processing | Days 1-10 (~63h) | Working Go service, SQLite schema, vendor stub, funding rules, ledger, pipeline, and REST API |
| Phase 2 — Review, Settlement & Returns | Days 11-15 (~30h) | Operator workflow, settlement batching, EOD cutoff, and return/reversal handling |
| Phase 3 — Demo, QA & Documentation | Days 16-19 (~38h) | Seed data, 20+ tests, demo script, reports, docs, and submission package |

---

## Phase 1: Foundation & Core Processing (TICKET-001 -> TICKET-008)

The following tickets are required to establish the financial and technical backbone of the system. This phase covers project setup, ledger correctness, vendor simulation, business rules, pipeline control flow, and the public API surface.

| Ticket | Title | MVP Role | Priority | Est. | Status |
|--------|-------|----------|----------|------|--------|
| TICKET-001 | Project Scaffolding and Configuration System | **Foundation** — establishes repo structure, config loading, and startup flow | P0 | 6h | DONE |
| TICKET-002 | SQLite Schema and Migration System | **Foundation** — core persistence and migration safety | P0 | 6h | DONE |
| TICKET-003 | Domain Types and State Machine | **Foundation** — pure domain model and transition rules | P0 | 3h | DONE |
| TICKET-004 | Vendor Service Stub | **Core** — deterministic scenario engine for all deposit outcomes | P0 | 12h | DONE |
| TICKET-005 | Funding Service and Business Rule Engine | **Core** — auth, account resolution, limits, duplicates, contribution defaults | P0 | 12h | DONE |
| TICKET-006 | Double-Entry Ledger Posting | **Core** — financial correctness and account balances | P0 | 6h | DONE |
| TICKET-007 | Deposit Pipeline Orchestration | **Core** — wires validation, rules, actions, and event logging together | P0 | 6h | DONE |
| TICKET-008 | REST API Layer | **Core** — submission, status, queue, settlement, and return endpoints | P0 | 12h | DONE |

### Phase 1 Dependencies

- TICKET-001 blocks all implementation work.
- TICKET-002 and TICKET-003 block TICKET-005, TICKET-006, and TICKET-007.
- TICKET-004 and TICKET-005 both feed into TICKET-007.
- TICKET-006 depends on TICKET-002 and TICKET-003.
- TICKET-008 depends on TICKET-004 through TICKET-007 being defined enough to expose stable handlers.

---

## Phase 2: Review, Settlement & Returns (TICKET-009 -> TICKET-011)

This phase turns the processing engine into a full end-to-end product by adding operator tooling, settlement output, and post-settlement reversal handling.

| Ticket | Title | Phase Role | Priority | Est. | Status |
|--------|-------|------------|----------|------|--------|
| TICKET-009 | Operator Web UI | **Operations** — manual review queue, risk triage, and operator actions | P0 | 12h | DONE |
| TICKET-010 | Settlement Engine | **Settlement** — X9-style structured JSON, batching, cutoff, and rollover | P0 | 12h | DONE |
| TICKET-011 | Return/Reversal Processing | **Financial Recovery** — bounced check reversals and fee application | P0 | 6h | DONE |

### Phase 2 Dependencies

- TICKET-009 depends on TICKET-008 and usable review/query APIs.
- TICKET-010 depends on TICKET-006, TICKET-007, and transfer lifecycle stability.
- TICKET-011 depends on TICKET-006, TICKET-007, and valid `FundsPosted` / `Completed` states.

---

## Phase 3: Demo, QA & Documentation (TICKET-012 -> TICKET-015)

This phase makes the project submission-ready with seeded scenarios, automated verification, demo storytelling, and documentation aligned to the rubric.

| Ticket | Title | Phase Role | Priority | Est. | Status |
|--------|-------|------------|----------|------|--------|
| TICKET-012 | Programmatic Data Seeding | **Demo Enablement** — bootstrap realistic deposits, images, and states | P1 | 6h | DONE |
| TICKET-013 | Test Suite | **QA** — 20+ tests covering invariants and end-to-end flows | P0 | 20h | DONE |
| TICKET-014 | Demo Script and Makefile | **Developer Experience** — one-command setup and narrated walkthrough | P0 | 6h | DONE |
| TICKET-015 | Documentation Package | **Submission** — README, architecture, ADRs, risks, and submission doc | P0 | 6h | DONE |

### Phase 3 Dependencies

- TICKET-012 depends on enough core services existing to seed valid end states.
- TICKET-013 depends on core API, business rules, ledger logic, and settlement flow being implemented.
- TICKET-014 depends on seeded data and stable flows from TICKET-012 and TICKET-013.
- TICKET-015 should track work continuously, but finalization depends on architecture and behavior being stable.

---

## KICKOFF-001: Requirements Review & Devlog Setup ✅

### Plain-English Summary
- Reviewed the three provided project documents to extract the functional requirements, architecture direction, constraints, and delivery expectations.
- Confirmed that the workspace currently contains planning documents only and no implementation scaffold yet.
- Attempted to index the workspace with GitNexus, but indexing failed because the folder is not initialized as a git repository.
- Created this `devlog.md` as the canonical work log so future changes, blockers, and ticket progress can be tracked in one place.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-09
- **Time:** ~1h actual vs 1h estimate
- **Ticket:** KICKOFF-001
- **Branch:** N/A (workspace is not a git repo yet)

### Scope
- Requirements review only.
- No implementation files existed before this entry.
- No code was generated beyond this devlog.

### Key Achievements
- Captured the project's intended architecture: Go single binary, SQLite, double-entry ledger, embedded web UI, structured settlement JSON, and deterministic vendor stub.
- Mapped the PRD backlog into three practical delivery phases for execution tracking.
- Established a reusable entry template for future work.

### Technical Implementation
- Parsed the PRD, challenger project brief, and architectural blueprint.
- Cross-checked the blueprint against the PRD to confirm it clarified decisions rather than changing scope.
- Verified that `npx gitnexus analyze` cannot run until the workspace becomes a git repository.

### Issues & Solutions
- Issue: GitNexus indexing was requested implicitly as part of project familiarization.
- Solution: Performed a manual requirements index from the docs and recorded the blocker for future setup.

### Errors / Bugs / Problems
- `npx gitnexus analyze` failed with: `fatal: not a git repository (or any of the parent directories): .git`
- No code errors encountered because no implementation exists yet.

### Testing
- Manual verification only:
  - Confirmed the workspace contains exactly three project documents.
  - Confirmed the architectural blueprint aligns with the PRD.
  - Confirmed no implementation tree exists yet.

### Files Changed
- **Created:** `devlog.md` — project development log and execution tracker

### Acceptance Criteria
- [x] Requirements documents reviewed
- [x] High-level architecture direction captured
- [x] Execution phases and ticket plan outlined
- [x] Development log created for future updates
- [x] GitNexus indexing blocker documented

### Performance
- No runtime metrics yet.
- Documentation review and setup tasks completed quickly with no environment dependencies beyond local file access.

### Next Steps
- TICKET-001: scaffold the Go project and configuration system.
- Initialize git if we want GitNexus indexing and cleaner change tracking.

### Learnings
- The docs are unusually complete; the main risk is execution drift, not ambiguous requirements.
- Financial invariants, operator workflow, and demo readiness are all first-class evaluation criteria, not just implementation details.

---

## TICKET-002: SQLite Schema and Migration System ✅

### Plain-English Summary
- Added `internal/store`: Open DB (SQLite with WAL), forward-only migration runner with `schema_migrations` version table, and migration 001 creating `transfers`, `ledger_entries`, and `deposit_events` with required columns and composite index for duplicate detection.
- Config now includes `DBPath` (env `DB_PATH`, default `data/apex.db`). Server opens DB and runs migrations on startup; restart skips already-applied migrations.
- Single-writer connection pool (MaxOpenConns=1) and WAL mode for SQLite; BEGIN IMMEDIATE is used when we add write transactions in later tickets.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-09
- **Ticket:** TICKET-002
- **Branch:** main

### Scope
- Store layer and migrations only. No domain types or API changes beyond ensuring server starts with DB and migrations.

### Key Achievements
- `schema_migrations` tracks applied version; migration 001 creates all three core tables and `idx_transfers_duplicate`; `ledger_entries.reversal_of` nullable FK present.

### Technical Implementation
- `internal/store/db.go`: Open with sqlite3 driver, WAL, busy_timeout. `internal/store/migrations.go`: create `schema_migrations` if not exists, loop migrations, run SQL and insert version. Migration 1: transfers (id, investor_account_id, correspondent_id, amount, status, vendor_transaction_id, check_number, micr_routing, micr_account, micr_data, risk_score, contribution_type, settlement_batch_id, created_at, updated_at), ledger_entries (id, transfer_id, account_id, entry_type, amount, memo, posted_at, reversal_of), deposit_events (id, transfer_id, event_type, actor, payload, created_at), index on (micr_routing, micr_account, check_number).

### Files Changed
- **Created:** internal/store/db.go, internal/store/migrations.go, config/correspondents.yaml, config/investors.yaml (if missing), cmd/server/main.go (if missing), internal/config/config.go (with DBPath), Makefile (CGO_ENABLED=1 for sqlite3), .env.example (DB_PATH)
- **Updated:** go.mod (sqlite3 dep), devlog.md

### Acceptance Criteria
- [x] Server runs migrations on startup; `schema_migrations` table tracks applied versions.
- [x] Restarting server skips already-applied migrations.
- [x] `transfers` table has all required columns including `risk_score` and `contribution_type`.
- [x] `ledger_entries` has `reversal_of` nullable FK for return handling.
- [x] Composite index on `transfers` (`micr_routing`, `micr_account`, `check_number`) for duplicate detection.

### Next Steps
- TICKET-003 (domain types and state machine).

---

## TICKET-003: Domain Types and State Machine ✅

### Plain-English Summary
- Added `internal/domain`: `Amount` (int64 cents) with `ToDollars()` and `ParseAmount()`; 8-state enum and hardcoded transition map; `Transition(from, to State)` returns `ErrInvalidTransition` (code `STATE.INVALID_TRANSITION`) for invalid moves; structured `DomainError` with `Code()` for API use.
- Unit tests cover Amount formatting/parsing (including fractional-cents rejection) and all valid/invalid state transitions.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-09
- **Ticket:** TICKET-003
- **Branch:** develop

### Scope
- Pure domain types and state machine only. No DB or API wiring. `Transition()` validates only; callers persist STATE_TRANSITION to `deposit_events` in TICKET-007.
- Extended after review: Transfer model, full VENDOR./FUNDING./SETTLEMENT./SYSTEM. code constants, `ValidTransitions` returns a copy, `ReturnFeeCents`, `StateTransitionPayload` for event payloads.

### Key Achievements
- `Transition(Requested, Completed)` returns `STATE.INVALID_TRANSITION`; all PRD-valid transitions succeed in tests; `ParseAmount("150.001")` returns error.

### Technical Implementation
- `amount.go`: `Amount` type, `ToDollars()`, `ParseAmount()` (rejects fractional cents). `state.go`: `State` constants, `validTransitions` map, `Transition()`, `ValidTransitions()`. `errors.go`: `DomainError` (ErrCode, Message), `Code()`, `ErrInvalidTransition`. Tests: `amount_test.go`, `state_test.go`.

### Files Changed
- **Created:** internal/domain/amount.go, internal/domain/state.go, internal/domain/errors.go, internal/domain/amount_test.go, internal/domain/state_test.go
- **Updated:** docs/development/devlog.md

### Acceptance Criteria
- [x] `Amount(15000).ToDollars()` returns `"$150.00"`.
- [x] `ParseAmount("150.00")` returns `Amount(15000)`.
- [x] `ParseAmount("150.001")` returns error (fractional cents).
- [x] `Transition(Requested, Completed)` returns `STATE.INVALID_TRANSITION` error.
- [x] All valid transitions succeed; all invalid transitions fail. Covered by unit tests.

### Next Steps
- TICKET-004 (Vendor Service Stub).

---

## TICKET-004: Vendor Service Stub ✅

### Plain-English Summary
- `internal/vendor`: deterministic stub with 7 scenarios via account prefix (PASS-, BLUR-, GLARE-, MICR-, DUP-, MISMATCH-) and `X-Vendor-Scenario` header override. Returns outcome, domain error codes, confidence, MICR fields, txn id; mismatch returns OCR amount cents differing from entered.
- `EnsureStubImages` writes `stub_front.png` / `stub_back.png` under `data/images/` at startup if missing.
- `POST /api/v1/vendor/validate` JSON `{account_id, amount_cents}` for manual/demo; optional header `X-Vendor-Scenario` with values CLEAN_PASS, IQA_BLUR, IQA_GLARE, MICR_FAILURE, DUPLICATE, AMOUNT_MISMATCH.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-09
- **Ticket:** TICKET-004
- **Branch:** main

### Acceptance Criteria
- [x] PASS-* CLEAN_PASS, confidence >= 0.95, MICR data
- [x] BLUR-* IQA_FAIL_BLUR + VENDOR.IQA_BLUR
- [x] GLARE-* IQA_FAIL_GLARE
- [x] MICR-* MICR_READ_FAILURE, confidence 0.42
- [x] DUP-* DUPLICATE_DETECTED
- [x] MISMATCH-* AMOUNT_MISMATCH, OCR amount differs
- [x] X-Vendor-Scenario overrides prefix
- [x] Synthetic PNGs in data/images/

### Next Steps
- TICKET-005 (Funding Service and business rules) — done below.

---

## TICKET-005: Funding Service and Business Rule Engine ✅

### Plain-English Summary
- `internal/funding`: **Engine** resolves investor by static API key (`Bearer` token), maps to correspondent + omnibus from YAML-backed config, enforces per-correspondent deposit limit, eligibility, and 30-day composite duplicate check (MICR routing + account + check # + amount).
- **DuplicateChecker**: in-memory for unit tests; **SQLDuplicateChecker** queries `transfers` so duplicates persist across restarts once TICKET-007 inserts rows.
- **HTTP**: `POST /api/v1/deposits/validate` — JSON `amount_cents`, optional `micr_routing`, `micr_account`, `check_number`; **401** + `FUNDING.ACCOUNT_NOT_FOUND` for missing/invalid key; **403** + `FUNDING.INELIGIBLE` for ineligible investor (e.g. Dave Wilson / `tok_dave_004`); **422** + `FUNDING.OVER_LIMIT` / `FUNDING.DUPLICATE` for rule failures; success JSON includes `omnibus_account_id` and `contribution_type` (IRA → correspondent default, e.g. INDIVIDUAL for CORR-APEX).

### Metadata
- **Status:** Complete
- **Date:** 2026-03-09
- **Ticket:** TICKET-005
- **Branch:** main

### Acceptance Criteria
- [x] Invalid/missing API key → 401 structured error (`FUNDING.ACCOUNT_NOT_FOUND`)
- [x] Ineligible investor (Dave Wilson) → 403 (`FUNDING.INELIGIBLE`)
- [x] $6,000 vs CORR-APEX $5,000 limit → `FUNDING.OVER_LIMIT`
- [x] Same check twice within 30 days → `FUNDING.DUPLICATE` (memory checker in tests; SQL checker when rows exist)
- [x] IRA → INDIVIDUAL from CORR-APEX config
- [x] Omnibus `OMNI-APEX-001` for CORR-APEX

### Next Steps
- TICKET-006 (Double-Entry Ledger Posting) — done below.

---

## TICKET-006: Double-Entry Ledger Posting ✅

### Plain-English Summary
- `internal/ledger`: **Post()** creates exactly one DEBIT and one CREDIT of equal amount in a single `BEGIN IMMEDIATE` transaction (explicit SQLite write lock). Uses `crypto/rand` for unique entry IDs (`le-` prefix + 32-char hex).
- **Balance()** returns per-account net balance via `SUM(CASE WHEN CREDIT THEN amount ELSE -amount END)`, returning 0 for accounts with no entries.
- Semantics match PRD §7.4: DEBIT on omnibus (from), CREDIT on investor (to). Memo defaults to `"FREE"`. TICKET-007 will wire this into the pipeline at the Approved → FundsPosted transition.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-10
- **Ticket:** TICKET-006
- **Branch:** main

### Acceptance Criteria
- [x] Every posting creates exactly one DEBIT and one CREDIT of equal amounts
- [x] Both entries created in same DB transaction (BEGIN IMMEDIATE)
- [x] Balance query (SUM of credits minus debits) returns correct account balance
- [x] `TestLedgerInvariant`: sum of all DEBITs == sum of all CREDITs across all accounts

### Next Steps
- TICKET-007 (Deposit Pipeline Orchestration) — done below.

---

## TICKET-007: Deposit Pipeline Orchestration ✅

### Plain-English Summary
- **Pipeline runner** (`internal/pipeline/runner.go`): step-function orchestrator that takes a Requested transfer through vendor validation (step 1), funding rules (step 2), and ledger posting (step 3). Each step persists state transitions and logs to `deposit_events`.
- **Three flows implemented:** Clean (PASS-* → FundsPosted), BLUR/GLARE/DUP (→ Rejected at step 1), MICR (→ Analyzing, flagged for operator review when confidence < 0.9).
- **Store layer** (`internal/store/transfers.go`, `events.go`): CreateTransfer, UpdateTransferStatus, UpdateTransferVendor, GetTransfer, InsertEvent, ListEvents.
- **Risk scoring** (`internal/pipeline/risk.go`): single-signal MVP — confidence >= 0.9 → score 0 (LOW, auto-approve), < 0.9 → score 65 (CRITICAL, flagged).
- **HTTP** (`POST /api/v1/deposits`): accepts `{account_id, amount_cents}` with Bearer token, creates transfer, runs pipeline, returns `{transfer_id, status, action, reason}`. 201 on success/flagged, 422 on rejection.
- **Approved always persisted**: even auto-approved deposits log Analyzing → Approved (actor=system) before Approved → FundsPosted.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-10
- **Ticket:** TICKET-007
- **Branch:** main

### Acceptance Criteria
- [x] Clean deposit flows: Requested → Validating → Analyzing → Approved → FundsPosted
- [x] BLUR deposit flows: Requested → Validating → Rejected (pipeline halts at step 1)
- [x] MICR failure flows: Requested → Validating → Analyzing (flagged, needs review)
- [x] Each pipeline step logged as `deposit_event` with step index and action
- [x] Auto-approved deposits have actor=`system` on the Analyzing→Approved transition

### Next Steps
- TICKET-008 (REST API Layer) for full endpoint coverage.

---

## TICKET-008: REST API Layer ✅

### Plain-English Summary
- Implemented the full REST API surface per PRD §7.6: auth middleware, structured error responses, all deposit endpoints (list, get, create, history, images), operator queue with approve/reject, account balance/ledger, and stub endpoints for settlement and returns.
- Created shared `internal/api` package with `WriteJSON`, `WriteError`, `WriteDomainError`, and `RequireAuth` helpers. All error responses follow the `{code, message, transfer_id, details}` schema from PRD §7.7.
- Auth middleware validates Bearer tokens against configured investor API keys; rejects invalid/missing tokens with 401 and structured JSON.
- Operator approve flow (`OperatorApprove`) transitions Analyzing → Approved → FundsPosted with actor=`operator:{id}` and performs ledger posting. Reject flow transitions to Rejected with reason.
- Image endpoint serves per-transfer images from `data/images/{id}/front.png` (or back), falling back to stub images.
- Settlement and returns endpoints return 501 Not Implemented with structured error bodies (TICKET-010 / TICKET-011).
- Added `ListTransfers` (with status/account/from/to filters) and `ListLedgerEntriesByAccount` store functions.
- Refactored `cmd/server/main.go` to use centralized handlers — removed inline `depositHandler`, `writeJSON`, `bearerToken`, `newTransferID` from main.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-10
- **Ticket:** TICKET-008
- **Branch:** main

### Acceptance Criteria
- [x] All endpoints return structured JSON with appropriate HTTP status codes
- [x] Error responses include `code`, `message`, `transfer_id`, and `details` fields
- [x] Auth middleware rejects requests without valid Bearer token (401)
- [x] Image endpoint serves check images via file serving (stub fallback)
- [x] Operator queue supports `?status=`, `?account=`, `?from=`, `?to=` query filters

### Files Changed
- **Created:** `internal/api/response.go` — shared WriteJSON, WriteError, WriteDomainError, RequireAuth, BearerToken
- **Created:** `internal/api/deposits.go` — DepositsHandler (list/create/get/history/images)
- **Created:** `internal/api/operator.go` — OperatorHandler (queue list/approve/reject)
- **Created:** `internal/api/accounts.go` — AccountsHandler (balance/ledger)
- **Created:** `internal/api/stubs.go` — SettlementHandler, ReturnsHandler (501 stubs)
- **Created:** `internal/api/api_test.go` — 18 tests covering auth, errors, all endpoints
- **Created:** `internal/pipeline/operator.go` — OperatorApprove, OperatorReject
- **Created:** `internal/store/ledger_entries.go` — ListLedgerEntriesByAccount
- **Modified:** `internal/store/transfers.go` — added TransferFilters, ListTransfers
- **Modified:** `cmd/server/main.go` — rewired to use api package handlers
- **Updated:** `docs/requirements/PRD.md` — checked off TICKET-008 acceptance criteria
- **Updated:** `docs/development/devlog.md` — this entry

### Next Steps
- TICKET-009 (Operator Web UI) for embedded single-page operator interface.

---

## TICKET-009: Operator Web UI ✅

### Plain-English Summary
- Embedded single-page web UI (vanilla HTML/CSS/JS) via `go:embed` in the Go binary. Routes: dashboard (`/`), deposit submission (`/submit`), operator queue (`/operator`), transfer detail (`/transfers/:id`).
- **Dashboard:** `GET /api/v1/stats/dashboard` returns deposit counts by state and KPI placeholders; UI renders benchmark cards and refreshes on button or after submit/approve/reject.
- **Submit:** Form POSTs to `POST /api/v1/deposits` with Bearer token (demo key injected by server from first investor).
- **Operator queue:** Fetches `GET /api/v1/operator/queue`; items sorted by risk score (highest first); each shows risk badge, amount, account, MICR data, check images (fetched with Bearer → blob URL). Approve (with optional contribution type INDIVIDUAL/EMPLOYER/ROLLOVER) and Reject (prompt for reason) post to existing APIs and refresh list.
- **Transfer detail:** Fetches deposit + history; renders decision trace from `deposit_events`.
- **Approve API:** Extended to accept optional `contribution_type` in body; `OperatorApprove` calls `UpdateTransferContribution` when provided.
- UI uses demo API key injected into index.html (first investor key); operator ID in sessionStorage and sent as `X-Operator-ID` on approve/reject.

### Metadata
- **Status:** Complete
- **Date:** 2026-03-10
- **Ticket:** TICKET-009
- **Branch:** main

### Acceptance Criteria
- [x] Dashboard shows benchmark cards and deposit counts by state
- [x] Benchmark cards refresh after submit/approve/reject (Refresh button + refresh on success)
- [x] Values from live system (stats API)
- [x] Operator queue: flagged deposits sorted by risk score (highest first)
- [x] Queue item: risk badge, amount, account, check images, MICR data
- [x] Approve/Reject post to API and update UI; contribution type dropdown (INDIVIDUAL, EMPLOYER, ROLLOVER)
- [x] Transfer detail: full decision trace from deposit_events

### Files Changed
- **Created:** `internal/api/stats.go` — GET /api/v1/stats/dashboard
- **Created:** `cmd/server/web/index.html`, `app.js`, `style.css` — SPA and styles
- **Modified:** `internal/api/operator.go` — approve body includes contribution_type
- **Modified:** `internal/pipeline/operator.go` — OperatorApprove(..., contributionType); UpdateTransferContribution when set
- **Modified:** `internal/api/deposits.go` — transferToJSON includes micr_routing, micr_account
- **Modified:** `cmd/server/main.go` — go:embed web, webHandler (SPA + static), stats route
- **Updated:** `docs/requirements/PRD.md`, `docs/development/devlog.md`

### Next Steps
- TICKET-010 (Settlement Engine).

---

## TICKET-010: Settlement Engine ✅

### Plain-English Summary
- Implemented X9 ICL–structured JSON settlement file generation. **POST /api/v1/settlement/batches** (optional `?as_of=<RFC3339>`) lists unbatched FundsPosted in a single transaction, builds file_header / cash_letters / file_control, inserts a batch row, assigns `settlement_batch_id` to each transfer, and commits (no double-batching).
- **GET /api/v1/settlement/batches/{id}** returns the stored file JSON; **GET .../batches/{id}/items** returns transfer summaries for that batch.
- EOD cutoff: 6:30 PM CT; injectable clock via `as_of`; settlement date = same day if before cutoff, else next business day (skip weekends). Friday 7 PM CT → Monday.
- Rejected deposits are never included (query filters on status = FundsPosted only).

### Metadata
- **Status:** Complete
- **Date:** 2026-03-10
- **Ticket:** TICKET-010
- **Branch:** main

### Acceptance Criteria
- [x] Generated file has file_header, cash_letters[], and file_control structure
- [x] file_control.total_amount equals sum of all check detail amounts
- [x] Deposits after 6:30 PM CT get next business day settlement date
- [x] Friday 7:00 PM CT deposit rolls to Monday
- [x] Rejected deposits never included
- [x] Deposits in a batch have settlement_batch_id set; second batch does not re-include them
- [x] Batch generation and settlement_batch_id assignment in same DB transaction

### Files Changed
- **Created:** `internal/settlement/cutoff.go` — 6:30 PM CT cutoff, SettlementDate, NextBusinessDay
- **Created:** `internal/settlement/x9.go` — X9File struct, BuildFile, ToJSON
- **Created:** `internal/settlement/engine.go` — GenerateBatch(db, now) with single tx
- **Created:** `internal/settlement/cutoff_test.go` — cutoff and Friday→Monday tests
- **Created:** `internal/store/settlement_batches.go` — InsertSettlementBatch, GetBatch, ListTransfersBySettlementBatch
- **Modified:** `internal/store/migrations.go` — migration 2: settlement_batches table
- **Modified:** `internal/store/transfers.go` — ListUnbatchedFundsPostedInTx, UpdateTransferSettlementBatch
- **Modified:** `internal/api/stubs.go` — SettlementHandler(cfg, db): POST batches, GET batch, GET batch items
- **Modified:** `cmd/server/main.go` — pass db to SettlementHandler
- **Modified:** `internal/api/api_test.go` — settlement tests (401, 200 no deposits, 201 + no double-batch)
- **Updated:** `docs/plans/ticket-010_settlement_engine.plan.md`, `docs/development/devlog.md`

### Next Steps
- TICKET-011 (Return/Reversal Processing).

---

## TICKET-011: Return/Reversal Processing ✅

### Plain-English Summary
- Implemented `POST /api/v1/returns` to process check returns in a single database transaction: validate transfer state (FundsPosted or Completed only), create 4 ledger entries (2 balanced pairs: reversal + $30 fee), transition to Returned, and log 3 events.
- **Reversal pair:** DEBIT investor (original amount) / CREDIT omnibus (original amount).
- **Fee pair:** DEBIT investor 3000 cents / CREDIT omnibus 3000 cents.
- **Events:** RETURN_RECEIVED (reason), REVERSAL_POSTED (amounts), INVESTOR_NOTIFIED (original amount, fee, net debit, reason code, human-readable message).
- Returns on invalid states (e.g. Requested) return 409 `STATE.INVALID_TRANSITION`. Returns on non-existent transfers return 404.
- Internal errors are logged and return generic 500 messages (no error leak).

### Metadata
- **Status:** Complete
- **Date:** 2026-03-11
- **Ticket:** TICKET-011
- **Branch:** main

### Acceptance Criteria
- [x] Return on FundsPosted deposit succeeds; transfer moves to Returned
- [x] Return on Completed deposit succeeds; transfer moves to Returned (late returns)
- [x] Reversal creates exactly 4 ledger entries: 2 balanced pairs (reversal + fee)
- [x] Fee amount is exactly 3000 cents ($30.00)
- [x] Return on Requested deposit returns STATE.INVALID_TRANSITION error (409)
- [x] Post-reversal, ledger debits equal credits (invariant preserved)
- [x] INVESTOR_NOTIFIED event payload includes: original amount, fee amount, net debit, reason code, human-readable message

### Files Changed
- **Created:** `internal/returns/process.go` — ProcessReturn (single tx: validate, reverse, fee, status, events)
- **Modified:** `internal/ledger/post.go` — added PostPairTx(tx, ...) for in-transaction ledger pairs
- **Modified:** `internal/store/transfers.go` — added UpdateTransferStatusTx(tx, ...)
- **Modified:** `internal/store/events.go` — added InsertEventTx(tx, ...)
- **Modified:** `internal/api/stubs.go` — ReturnsHandler(cfg, db): POST parses body, calls ProcessReturn, maps errors
- **Modified:** `cmd/server/main.go` — ReturnsHandler(cfg, db) wiring
- **Modified:** `internal/api/api_test.go` — 5 return tests (FundsPosted, Completed, invalid state, not found, ledger invariant)
- **Updated:** `docs/development/devlog.md`

### Next Steps
- TICKET-012 (Programmatic Data Seeding).

---

## Entry Format Template

Each ticket entry follows this standardized structure:

```md
## TICKET-XXX: [Title] [Status Emoji]

### Plain-English Summary
- What was done
- What it means
- Success looked like
- How it works in simple terms

### Metadata
- **Status:** Complete / In Progress / TODO
- **Date:** [date]
- **Time:** [actual] vs [estimate]
- **Ticket:** TICKET-XXX
- **Branch:** [branch-name]

### Scope
- What was planned/built

### Key Achievements
- Notable accomplishments and highlights

### Technical Implementation
- Architecture decisions, code patterns, infrastructure

### Issues & Solutions
- Problems encountered and fixes applied

### Errors / Bugs / Problems
- All errors, bugs, unexpected behaviors, and blockers encountered
- Include what happened, what was tried, and what fixed it (or did not)

### Testing
- Automated and manual test results

### Files Changed
- **Created:** file — description
- **Modified:** file — description
- **Updated:** `devlog.md` — this entry

### Acceptance Criteria
- [x] / [ ] Checklist from PRD

### Performance
- Metrics, benchmarks, observations

### Next Steps
- What comes next

### Learnings
- Key takeaways and insights
```

---

## Ticket Summary

| ID | Title | Phase | Priority | Est. | Status |
|----|-------|-------|----------|------|--------|
| TICKET-001 | Project Scaffolding and Configuration System | Phase 1 | P0 | 6h | DONE |
| TICKET-002 | SQLite Schema and Migration System | Phase 1 | P0 | 6h | DONE |
| TICKET-003 | Domain Types and State Machine | Phase 1 | P0 | 3h | DONE |
| TICKET-004 | Vendor Service Stub | Phase 1 | P0 | 12h | DONE |
| TICKET-005 | Funding Service and Business Rule Engine | Phase 1 | P0 | 12h | DONE |
| TICKET-006 | Double-Entry Ledger Posting | Phase 1 | P0 | 6h | DONE |
| TICKET-007 | Deposit Pipeline Orchestration | Phase 1 | P0 | 6h | DONE |
| TICKET-008 | REST API Layer | Phase 1 | P0 | 12h | DONE |
| TICKET-009 | Operator Web UI | Phase 2 | P0 | 12h | DONE |
| TICKET-010 | Settlement Engine | Phase 2 | P0 | 12h | DONE |
| TICKET-011 | Return/Reversal Processing | Phase 2 | P0 | 6h | DONE |
| TICKET-012 | Programmatic Data Seeding | Phase 3 | P1 | 6h | DONE |
| TICKET-013 | Test Suite | Phase 3 | P0 | 20h | DONE |
| TICKET-014 | Demo Script and Makefile | Phase 3 | P0 | 6h | DONE |
| TICKET-015 | Documentation Package | Phase 3 | P0 | 6h | DONE |

**Total: 15 tickets · ~131 hours · 19 delivery days**

---

## Risks & Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Financial posting bug breaks debit/credit invariants | Medium | High | Build ledger posting and reversal logic around single-transaction inserts; keep invariant tests in TICKET-013 as a release gate |
| Vendor stub behavior drifts from demo/test expectations | Medium | High | Use deterministic scenario routing via magic prefixes plus header override; document scenarios in README and tests |
| Time-dependent settlement logic becomes flaky in tests | Medium | Medium | Use injectable clock and explicit `as_of` controls; avoid wall-clock dependence in automated tests |
| Operator workflow takes too long to polish near deadline | Medium | Medium | Keep UI embedded and minimal; prioritize queue clarity, image rendering, and approve/reject flow over visual flourish |
| Workspace cannot be indexed or cleanly tracked without git | High | Medium | Initialize git early in TICKET-001 so GitNexus and normal change tracking work reliably |
| Demo and seeded data diverge from actual system behavior | Medium | High | Generate scenario data programmatically from the same domain logic used by the app, not from static one-off fixtures |

