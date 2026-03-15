# Architecture — Mobile Check Deposit System

## System Diagram

```
                    ┌──────────────────┐
                    │   Mobile Client  │
                    │   / Web Browser  │
                    └────────┬─────────┘
                             │ HTTPS
                    ┌────────▼─────────┐
                    │    REST API       │
                    │   (net/http mux)  │
                    │                   │
                    │  Auth Middleware   │
                    │  Bearer Token     │
                    └────────┬─────────┘
                             │
          ┌──────────────────┼──────────────────┐
          │                  │                  │
   ┌──────▼──────┐   ┌──────▼──────┐   ┌──────▼──────┐
   │  Deposits   │   │  Operator   │   │ Settlement  │
   │  Handler    │   │  Handler    │   │  Handler    │
   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
          │                  │                  │
   ┌──────▼──────────────────▼──────┐   ┌──────▼──────┐
   │        Pipeline Runner         │   │  Settlement │
   │                                │   │  Engine     │
   │  Step 1: Vendor Validation     │   │  (X9 JSON)  │
   │  Step 2: Funding Rules         │   └──────┬──────┘
   │  Step 3: Ledger Posting        │          │
   └──────┬──────┬──────────────────┘          │
          │      │                             │
   ┌──────▼──┐ ┌─▼───────────┐                │
   │ Vendor  │ │   Funding   │                │
   │  Stub   │ │   Engine    │                │
   │         │ │             │                │
   │ 7 scen- │ │ Auth/Limits │                │
   │ arios   │ │ Duplicates  │                │
   └─────────┘ │ Eligibility │                │
               └─────────────┘                │
                                              │
   ┌──────────────────────────────────────────┘
   │
   ▼
   ┌──────────────────────────────────────────┐
   │            Double-Entry Ledger           │
   │                                          │
   │  Post():     DEBIT omnibus / CREDIT inv  │
   │  Reversal:   DEBIT investor / CREDIT omn │
   │  Fee:        DEBIT investor / CREDIT omn │
   │  Balance():  SUM(credits) - SUM(debits)  │
   └──────────────────┬───────────────────────┘
                      │
   ┌──────────────────▼───────────────────────┐
   │              SQLite (WAL mode)           │
   │                                          │
   │  transfers        — deposit lifecycle    │
   │  ledger_entries   — balanced DEBIT/CREDIT│
   │  deposit_events   — full audit trail     │
   │  settlement_batches — X9 file storage    │
   │  schema_migrations — version tracking    │
   └──────────────────────────────────────────┘
```

## Data Flow

### 1. Deposit Submission

```
Client POST /api/v1/deposits {account_id, amount_cents}
  │
  ├─ Auth: Bearer token → resolve investor, correspondent, omnibus
  ├─ Create transfer (status: Requested)
  │
  ├─ Pipeline Step 1: Vendor Validation
  │   ├─ Account prefix → scenario selection (PASS/BLUR/GLARE/MICR/DUP/MISMATCH)
  │   ├─ Extract MICR data (routing, account, check number)
  │   ├─ Persist vendor fields to transfer row
  │   └─ Reject on IQA failure or vendor duplicate → status: Rejected (terminal)
  │
  ├─ Pipeline Step 2: Funding Rules
  │   ├─ Per-correspondent deposit limit ($5,000 for CORR-APEX)
  │   ├─ Account eligibility check
  │   ├─ 30-day composite duplicate detection (routing|account|check|amount)
  │   ├─ Contribution type assignment (IRA → INDIVIDUAL)
  │   └─ Reject on rule violation → status: Rejected (terminal)
  │
  ├─ Risk Scoring
  │   ├─ Confidence ≥ 0.9 → score 0 (LOW) → auto-approve
  │   └─ Confidence < 0.9 → score 65 (CRITICAL) → flag for review
  │
  ├─ Pipeline Step 3: Ledger Posting (if auto-approved)
  │   ├─ DEBIT omnibus account (amount)
  │   ├─ CREDIT investor account (amount)
  │   └─ Status: Approved → FundsPosted
  │
  └─ Response: 201 {transfer_id, status, action} or 422 {code, message}
```

### 2. Operator Review

```
GET /api/v1/operator/queue → flagged deposits (status: Analyzing), sorted by risk score

POST /api/v1/operator/queue/{id}/approve
  ├─ Transition: Analyzing → Approved → FundsPosted
  ├─ Ledger posting (DEBIT omnibus / CREDIT investor)
  ├─ Optional: override contribution_type
  └─ Events: STATE_TRANSITION (actor: operator:{id})

POST /api/v1/operator/queue/{id}/reject
  ├─ Transition: Analyzing → Rejected (terminal)
  └─ Events: STATE_TRANSITION with rejection reason
```

### 3. Settlement Batching

```
POST /api/v1/settlement/batches
  ├─ Query: all FundsPosted transfers without settlement_batch_id
  ├─ EOD cutoff: 6:30 PM CT
  │   ├─ Before cutoff → settlement_date = today
  │   └─ After cutoff → settlement_date = next business day (skip weekends)
  ├─ Build X9 ICL-structured JSON (file_header, cash_letters, file_control)
  ├─ Single transaction: insert batch + assign settlement_batch_id to transfers
  └─ Response: 201 {batch_id, settlement_date, total_amount, item_count}

GET /api/v1/settlement/batches/{id} → X9 file JSON
GET /api/v1/settlement/batches/{id}/items → transfer summaries
```

### 4. Return/Reversal

```
POST /api/v1/returns {transfer_id, reason}
  ├─ Validate: transfer must be FundsPosted or Completed
  ├─ Single transaction (BEGIN IMMEDIATE):
  │   ├─ Reversal pair: DEBIT investor / CREDIT omnibus (original amount)
  │   ├─ Fee pair: DEBIT investor / CREDIT omnibus ($30.00)
  │   ├─ Status: → Returned (terminal)
  │   └─ Events: RETURN_RECEIVED, REVERSAL_POSTED, INVESTOR_NOTIFIED
  └─ Response: 200 {status, fee_cents, reason}
```

## Service Boundaries

| Package | Responsibility | Dependencies |
|---------|---------------|--------------|
| `cmd/server` | HTTP server, route wiring, embedded web UI | All internal packages |
| `internal/api` | Request handling, auth, JSON response formatting | config, store, pipeline, settlement, returns |
| `internal/pipeline` | Deposit orchestration (3-step), operator approve/reject | vendor, funding, ledger, store, domain |
| `internal/vendor` | Deterministic vendor simulation (7 scenarios) | None (pure) |
| `internal/funding` | Auth, limits, eligibility, duplicate detection | config, store |
| `internal/ledger` | Double-entry posting, balance queries | store |
| `internal/settlement` | X9 file generation, EOD cutoff, batch management | store |
| `internal/returns` | Reversal processing, fee application | store, ledger, domain |
| `internal/domain` | State machine, amount type, error codes | None (pure) |
| `internal/store` | SQLite access, migrations, CRUD operations | database/sql |
| `internal/config` | YAML config loading, env vars | None |
| `internal/seed` | Demo data population (idempotent) | pipeline, settlement, returns |

## State Machine

```
Requested → Validating → Analyzing ──→ Approved → FundsPosted → Completed
                              │                        │            │
                              ▼                        ▼            ▼
                           Rejected                 Returned     Returned
                           (terminal)               (terminal)   (terminal)
```

All 8 states are persisted. `Approved` is always recorded (even for auto-approved deposits) to maintain a complete audit trail. Terminal states (`Rejected`, `Returned`) cannot transition further.

## Concurrency Model

SQLite with WAL mode and `MaxOpenConns=1` ensures single-writer serialization. All financial mutations (ledger posting, return processing, settlement batching) use `BEGIN IMMEDIATE` transactions to acquire the write lock at transaction start, preventing TOCTOU races. Read operations are non-blocking under WAL mode.
