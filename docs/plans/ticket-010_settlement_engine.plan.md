# TICKET-010: Settlement Engine — Implementation Plan

## Overview

Implement the Settlement Engine: X9 ICL–structured JSON file generation, EOD cutoff at 6:30 PM CT with injectable clock (`?as_of`), next-business-day rollover (skip weekends), and a double-batching guard so no deposit appears in two settlement files.

**Size:** L (1–2 days) | **Depends on:** TICKET-006, TICKET-007, transfer lifecycle (FundsPosted state)

---

## Scope (from PRD)

- **X9-style JSON:** `file_header`, `cash_letters[]` (each with `header`, `bundles[]`), `file_control` (total_amount, item_count, bundle_count).
- **Check detail:** amount (cents), MICR (routing, on_us), image refs (front/back).
- **EOD cutoff:** 6:30 PM CT; optional `?as_of=<RFC3339>` for tests/demo.
- **Rollover:** After cutoff, settlement date = next business day (skip Sat/Sun); Friday 7 PM → Monday.
- **Guard:** Only `FundsPosted` with `settlement_batch_id IS NULL` are included; batch generation and `settlement_batch_id` assignment in one DB transaction.
- **Rejected deposits** are never included (query filters on status = FundsPosted).

---

## Acceptance Criteria (checklist)

- [ ] Generated file has `file_header`, `cash_letters[]`, and `file_control` structure.
- [ ] `file_control.total_amount` equals sum of all check detail amounts.
- [ ] Deposits submitted after 6:30 PM CT get next business day settlement date.
- [ ] Friday 7:00 PM CT deposit rolls to Monday.
- [ ] Rejected deposits are never included in settlement files.
- [ ] Deposits in a batch have `settlement_batch_id` set; second batch does not re-include them (no double-batching).
- [ ] Batch generation and `settlement_batch_id` assignment happen within the same database transaction.

---

## Current State

- **API:** `POST/GET /api/v1/settlement/*` currently return 501 via `api.SettlementHandler(cfg)` (no DB). Need to pass `db` and call settlement engine.
- **Store:** `transfers` has `settlement_batch_id`; no `ListUnbatchedFundsPosted`; no `UpdateTransferSettlementBatch`; no `settlement_batches` table yet.
- **No** `internal/settlement` package yet.

---

## Design

### 1. Settlement batches table

Store each generated batch so we can serve `GET /api/v1/settlement/batches/{id}` and `GET .../batches/{id}/items`.

- **Migration 2:**  
  `CREATE TABLE settlement_batches (id TEXT PRIMARY KEY, settlement_date TEXT NOT NULL, file_json TEXT NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);`
- **Semantics:** When we generate a batch we assign a new batch ID (e.g. `batch-<date>-<short-uuid>`), build the X9 JSON, then in **one transaction**: `INSERT INTO settlement_batches`, `UPDATE transfers SET settlement_batch_id = ? WHERE id IN (...)` for all included transfer IDs. No separate “save file” step.

### 2. Store layer

- **ListUnbatchedFundsPosted(db, settlementDate string)**  
  Returns transfers where `status = 'FundsPosted'` AND `(settlement_batch_id IS NULL OR settlement_batch_id = '')` AND `created_at` is on or before the effective cutoff for that settlement date (if we filter by date at all).  
  For MVP we can take “settlement date” as the business day; include all unbatched FundsPosted that we’re willing to assign to that day. Simplest: no created_at filter in the query — we decide settlement date at generation time (today vs next business day from clock), then list all unbatched FundsPosted. So: **ListUnbatchedFundsPosted(db)** → list of transfers; caller (settlement engine) has already decided “today” or “next business day” and will store that date in `settlement_batches.settlement_date`.
- **CreateBatchWithTransfers(db, batchID, settlementDate string, fileJSON string, transferIDs []string)**  
  In a single transaction:  
  - `INSERT INTO settlement_batches (id, settlement_date, file_json, created_at) VALUES (?, ?, ?, now)`  
  - For each id in transferIDs: `UPDATE transfers SET settlement_batch_id = ?, updated_at = ? WHERE id = ?`  
  - Return error if any update fails (e.g. transfer already batched — should not happen if we lock/list within same tx).  
  To avoid double-batching we must **select and lock** unbatched FundsPosted in the same transaction as the updates. So better: **CreateBatchAndAssignTransfers(db, batchID, settlementDate string, fileJSON string)** (no transferIDs): inside one transaction, `SELECT id FROM transfers WHERE status = 'FundsPosted' AND (settlement_batch_id IS NULL OR settlement_batch_id = '') FOR UPDATE` (SQLite: `BEGIN IMMEDIATE`; then SELECT; build JSON from selected rows? No — we can’t build JSON inside the same tx without having the transfer rows. So: 1) Read unbatched FundsPosted (outside tx or in a read-only step). 2) Build JSON from that list. 3) In one tx: INSERT batch, UPDATE those transfer IDs. Risk: between 1 and 3 another process could batch some of them. So we need to do everything in one transaction: BEGIN IMMEDIATE; SELECT unbatched FundsPosted (we get rows); build JSON from rows (in Go); INSERT batch; UPDATE transfers SET settlement_batch_id WHERE id IN (ids from selection); COMMIT. So the store should expose: **ListUnbatchedFundsPostedForUpdate(db, tx *sql.Tx)** used inside a transaction that the caller starts, so the caller can: BEGIN; list; build JSON; insert batch; update transfers; COMMIT. Alternatively: one store function **GenerateBatch(db, settlementDate, clock)** that does all of it and returns (batchID, fileJSON, nil). That would require the settlement package to call store with a closure or the store to accept a “builder” that receives the list of transfers. Cleaner: **SettlementStore** interface or just functions: **BeginTx**, **ListUnbatchedFundsPostedInTx(tx)**, **InsertSettlementBatch(tx, id, date, json)**, **UpdateTransferSettlementBatch(tx, transferID, batchID)**. Then the settlement engine: opens tx, lists unbatched, builds JSON, inserts batch, updates each transfer, commits. So we need:
  - **ListUnbatchedFundsPostedInTx(tx *sql.Tx)** → []*domain.Transfer
  - **InsertSettlementBatch(tx, id, settlementDate, fileJSON)**
  - **UpdateTransferSettlementBatch(tx, transferID, batchID)**
  And the API/settlement engine runs: tx := db.Begin(); defer tx.Rollback(); list := store.ListUnbatchedFundsPostedInTx(tx); build file from list; batchID := newID(); store.InsertSettlementBatch(tx, batchID, date, fileJSON); for _, t := range list { store.UpdateTransferSettlementBatch(tx, t.ID, batchID) }; tx.Commit().
- **GetBatch(db, batchID)** → (settlementDate, fileJSON, created_at) for GET batch.
- **ListTransfersBySettlementBatch(db, batchID)** → for GET batch items (optional; can also derive from file_json if we store it).

### 3. Cutoff and next-business-day logic

- **Location:** `internal/settlement/cutoff.go` (or `clock.go`).
- **6:30 PM CT:** Parse “today” in CT; set cutoff to today 18:30 CT. Compare clock time (or `as_of`) to that cutoff.
- **Next business day:** Given a time, if it’s after 6:30 PM CT that day, settlement date = next weekday (skip Sat/Sun). Friday after 6:30 PM → Monday. Use a small table or func: add 1 day; if Saturday add 2, if Sunday add 1.
- **Injectable clock:** Handler reads `?as_of=2026-03-09T18:35:00-06:00`; if present parse and use that time for “now” for cutoff and settlement date; otherwise use `time.Now()`. Use CT (America/Chicago) for cutoff.

### 4. X9 JSON structure (mirror blueprint)

- **file_header:** standard_level, destination_routing, origin_routing, file_creation_date (YYYYMMDD), file_creation_time (HHMM).
- **cash_letters:** array of one (or more) cash letter; each has **header** (collection_type, destination_routing, record_type) and **bundles** array.
- **bundle:** header (bundle_id); **checks** array.
- **check:** detail (amount in cents, micr_routing, micr_on_us or equivalent, payor_bank_routing, item_sequence); **image_views** (side, ref) — ref can be placeholder or path like `/api/v1/deposits/{id}/images/front`.
- **file_control:** total_amount (sum of all check amounts), item_count, bundle_count.

Amount in X9 is often in “cents” or similar; PRD says “control totals at each level” and “file_control.total_amount equals sum of all check detail amounts”, so use integer cents consistently.

### 5. API endpoints

- **POST /api/v1/settlement/batches**  
  - Query: `?as_of=<RFC3339>` optional.  
  - Auth: required (existing RequireAuth).  
  - Logic: resolve “now” from as_of or time.Now(); compute settlement date (today if before 6:30 PM CT, else next business day); start tx; list unbatched FundsPosted in tx; if empty, return 200 with message “no deposits to batch” and no batch created (or 200 with body indicating 0 items); else build X9 JSON, generate batch ID, insert batch, update transfers, commit; return 201 with Location and JSON body containing batch_id, settlement_date, file (full X9 object) and/or file_control totals.
- **GET /api/v1/settlement/batches/{id}**  
  - Return 200 with full file JSON (from settlement_batches.file_json) or 404 if not found.
- **GET /api/v1/settlement/batches/{id}/items**  
  - Return list of transfer summaries (transfer_id, amount_cents, status, etc.) for that batch (from transfers where settlement_batch_id = id).

Settlement handler must receive **db *sql.DB** and route by path/method (POST batches, GET batches/{id}, GET batches/{id}/items). Keep existing 501 for other paths if any.

### 6. Package layout

- **internal/settlement/**
  - **cutoff.go** — Cutoff(time) (cutoff time for that day CT), AfterCutoff(now) bool, SettlementDate(now time.Time) string (YYYY-MM-DD), NextBusinessDay(t time.Time) time.Time.
  - **x9.go** — BuildFile(transfers []*domain.Transfer, settlementDate string, fileCreationTime time.Time) (fileJSON string, err error). Builds the nested file_header, cash_letters, file_control; total_amount = sum of amounts; item_count = len(transfers); bundle_count = 1 (or more if we split).
  - **engine.go** — GenerateBatch(db, now time.Time) (batchID, settlementDate, fileJSON string, err error): begin tx, list unbatched in tx, build file, insert batch, update transfers, commit.
- **internal/store/**
  - **settlement_batches.go** (or in transfers.go): InsertSettlementBatch(tx, id, date, fileJSON), GetBatch(db, id), ListTransfersBySettlementBatch(db, batchID).
  - **transfers.go:** ListUnbatchedFundsPostedInTx(tx), UpdateTransferSettlementBatch(tx, transferID, batchID).
- **internal/api/stubs.go** → replace SettlementHandler(cfg) with **SettlementHandler(cfg, db)** that routes POST/GET and calls settlement engine + store.

### 7. Events

- Optionally insert a `SETTLEMENT.BATCH_CREATED` (or similar) event per transfer when they’re added to a batch (actor = "system", payload = { batch_id, settlement_date }). PRD doesn’t require it for TICKET-010; can add later for audit.

### 8. Testing (TICKET-013 will add integration tests)

- Unit tests for cutoff: before 6:30 PM CT → today; after → next business day; Friday 7 PM → Monday.
- Unit test for X9 builder: one transfer → file_control.total_amount equals transfer amount, item_count=1, bundle_count=1.
- Integration test: create two FundsPosted, generate batch → both get settlement_batch_id; generate again → no new batch or 0 items (no double-batching).

---

## Implementation Order

1. **Migration 2** — add `settlement_batches` table.
2. **Store** — ListUnbatchedFundsPostedInTx, UpdateTransferSettlementBatch, InsertSettlementBatch, GetBatch, ListTransfersBySettlementBatch.
3. **internal/settlement** — cutoff (SettlementDate, NextBusinessDay, AfterCutoff), X9 builder (BuildFile), engine (GenerateBatch using tx).
4. **API** — SettlementHandler(cfg, db): parse path/method; POST batches (with ?as_of), GET batches/{id}, GET batches/{id}/items; wire db in main.go.
5. **Devlog/PRD** — update TICKET-010 status and any doc references.

---

## Out of scope (this ticket)

- Actual binary X9.37 / EBCDIC.
- Holiday calendar (weekends only for “next business day”).
- Settlement Bank acknowledgment (FundsPosted → Completed) — separate flow.
