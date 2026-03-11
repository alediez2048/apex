# Mobile Check Deposit System — Product Requirements Document

**High-Reliability MCD Ecosystem: From OCR Capture to Final Bank Settlement**

| | |
|---|---|
| **Organization** | Apex Fintech Services |
| **Version** | 1.1 |
| **Date** | March 2026 |
| **Classification** | Internal / Confidential |
| **Prepared by** | Engineering Architecture Team |

---

## Table of Contents

1. [Executive Summary & Vision](#1-executive-summary--vision)
2. [Target Audience & Personas](#2-target-audience--personas)
3. [Goals & Success Metrics (KPIs)](#3-goals--success-metrics-kpis)
4. [User Stories](#4-user-stories)
5. [Functional Requirements (Prioritized)](#5-functional-requirements-prioritized)
6. [User Flow & Experience](#6-user-flow--experience)
7. [Technical Architecture](#7-technical-architecture)
8. [Phasing & Roadmap](#8-phasing--roadmap)
9. [Implementation Tickets (Phase 1 Backlog)](#9-implementation-tickets-phase-1-backlog)
10. [Assumptions, Constraints & Open Questions](#10-assumptions-constraints--open-questions)
11. [Appendix A: Rubric Traceability Matrix](#appendix-a-rubric-traceability-matrix)

---

## 1. Executive Summary & Vision

### 1.1 North Star

Deliver a zero-error, fully auditable mobile check deposit pipeline that processes investor deposits from image capture through bank settlement with 100% gating correctness and 100% settlement reconciliation, while providing operators with intelligent, risk-prioritized review tools.

### 1.2 Problem Statement

Modern brokerage platforms require mobile check deposit capabilities to remain competitive. Investors expect the convenience of photographing a check and seeing funds credited to their account within hours, not days. However, the complexity of this seemingly simple interaction is substantial: each deposit must pass through image quality validation, MICR/OCR data extraction, duplicate detection, business rule enforcement, compliance gating, operator review, double-entry ledger posting, X9 ICL settlement file generation, and return/reversal handling.

The current state requires building this capability from scratch, integrating with an external vendor for image processing while maintaining internal control over business rules, ledger integrity, and settlement. The system must handle the full deposit lifecycle with zero tolerance for financial errors: no deposit should post to the ledger without passing all validation gates, and every cent that enters the system must be reconcilable against the settlement file.

### 1.3 Interview-Driven Insights

Three rounds of architectural interviews (30 decisions total) refined the original requirements into an execution-ready specification. Key findings that shaped this PRD:

**Financial precision is non-negotiable.** Integer cents (int64) representation eliminates floating-point rounding errors that would break settlement reconciliation. This was identified as a critical invariant: the sum of all ledger debits must exactly equal the sum of all credits.

**The vendor stub is a first-class deliverable.** At 15 rubric points, the stub quality is the third highest-scored category. Magic account number prefixes with header override provide self-documenting, deterministic test scenarios.

**Operator experience differentiates the submission.** A composite risk scoring model (4 weighted signals) transforms the operator queue from a flat list into an intelligent triage tool, directly addressing the "risk indicators and Vendor Service scores" requirement.

**Developer experience is rubric-critical.** One-command setup (`make dev`), narrated demo scripts, and a complete Makefile workflow directly map to 10 rubric points. The system must be immediately usable upon cloning.

---

## 2. Target Audience & Personas

### 2.1 Investor (End User)

**Role:** Brokerage account holder depositing checks via mobile application.

**Pain Points:** Slow mail-based deposits; unclear failure reasons when images are rejected; no visibility into deposit status after submission; unexpected return fees without explanation.

**Needs:** Instant feedback on image quality (retake prompts for blur/glare); real-time status tracking through all states; clear notification when deposits complete or when checks are returned with fee breakdown.

**Interview Insight:** Structured error codes (VENDOR.IQA_BLUR, FUNDING.OVER_LIMIT) enable the mobile UI to display actionable, human-readable messages rather than generic failure notices.

### 2.2 Operations Analyst (Operator)

**Role:** Internal reviewer responsible for approving or rejecting flagged deposits.

**Pain Points:** Flat review queues with no prioritization; insufficient context to make approval decisions; no ability to override system defaults when legitimate edge cases arise.

**Needs:** Risk-prioritized queue with check images, MICR data, and confidence scores side-by-side; one-click approve/reject with mandatory audit logging; ability to override contribution type defaults for retirement accounts.

**Interview Insight:** The composite risk scoring model (MICR confidence, amount delta, limit proximity, first-deposit flag) was identified as a key differentiator. Operators see a 0–100 risk score with individual signal breakdowns, enabling faster and more informed decisions.

### 2.3 Correspondent Firm Administrator

**Role:** External firm using the platform to offer check deposit to their investors.

**Pain Points:** One-size-fits-all deposit limits; no visibility into their investors' deposit activity; inability to customize business rules.

**Needs:** Per-correspondent configurable deposit limits and contribution type defaults; dedicated omnibus account for settlement; transparent fee structures.

**Interview Insight:** The static YAML configuration with 3 correspondents (each with different deposit limits and contribution defaults) demonstrates parameterized business rules, addressing the "looked up via client config" requirement.

### 2.4 Platform Engineer (Reviewer/Developer)

**Role:** Technical evaluator assessing the system for code quality, architecture, and production readiness.

**Pain Points:** Projects that require complex setup; opaque business logic hidden behind frameworks; insufficient test coverage; inability to trace a deposit through the system.

**Needs:** One-command setup; explicit control flow; comprehensive tests; narrated demo scripts; decision log documenting trade-offs.

**Interview Insight:** Go single-binary with embedded web UI was chosen specifically because explicit control flow (no annotation magic) makes code review faster, and `go:embed` preserves the one-command setup requirement.

---

## 3. Goals & Success Metrics (KPIs)

The following metrics define quantitative success criteria, directly traceable to the evaluation rubric and interview findings.

These KPIs should be visible in a live benchmark dashboard within the web UI so reviewers and operators can see current system health, not just read test output after the fact.

| KPI | Target | Measurement Method | Rubric Alignment |
|---|---|---|---|
| Gating Correctness | 0 deposits posted without passing all validation gates | Integration test: `TestLedgerInvariant_DebitsEqualCredits` verifies no unvalidated postings | Core Correctness (25 pts) |
| Settlement Reconciliation | 100% match between ledger entries and settlement file totals | Test: sum of check detail amounts = bundle total = cash letter total = file control total | Core Correctness (25 pts) |
| Vendor Stub Scenario Coverage | 7/7 differentiated response types exercisable and testable | Demo script exercises all scenarios with pass/fail verification | Stub Quality (15 pts) |
| Operator Queue Response | Flagged deposits appear in review queue within 1 second of flagging | Integration test timing assertion | Operator Workflow (10 pts) |
| Return/Reversal Accuracy | 100% of returned checks reversed with exact $30.00 fee deduction | Test: reversal amount = original deposit + fee; ledger balanced post-reversal | Return Handling (10 pts) |
| Test Coverage | Minimum 20 tests (2x the 10-test minimum) | `go test ./... -count=1` with coverage report | Tests & Evaluation (10 pts) |
| Setup Time | Clone-to-running in under 30 seconds via `make dev` | Manual verification; Makefile automates build, seed, and run | Developer Experience (10 pts) |
| Benchmark Dashboard Freshness | KPI tiles update on page load and within 5 seconds of benchmark-affecting actions in demo mode | Manual verification plus UI integration test for dashboard refresh after deposit, settlement, and return actions | Operator Workflow (10 pts) + DevEx (10 pts) |

---

## 4. User Stories

### 4.1 Investor Stories

**US-01:** As an investor, I want to photograph the front and back of a check and submit it for deposit into my brokerage account, so that I can fund my account without mailing a physical check.

**US-02:** As an investor, I want to receive immediate feedback if my check image is too blurry or has glare, so that I can retake the photo and resubmit without waiting for a manual rejection.

**US-03:** As an investor, I want to track my deposit status in real time (Requested, Validating, Analyzing, Approved, FundsPosted, Completed), so that I know exactly where my money is in the process.

**US-04:** As an investor, I want to be notified if my check is returned with a clear explanation and fee breakdown ($30 return fee), so that I understand the deduction from my account.

**US-05:** As an investor, I want to receive a specific error message when my deposit exceeds the $5,000 limit, so that I know the exact threshold and can adjust my deposit amount.

### 4.2 Operator Stories

**US-06:** As an operations analyst, I want to see flagged deposits sorted by risk score (0–100) with individual signal breakdowns, so that I can prioritize the highest-risk items first.

**US-07:** As an operations analyst, I want to view check images (front and back) alongside MICR data and confidence scores in a single screen, so that I can make approval decisions without switching between views.

**US-08:** As an operations analyst, I want to override the default contribution type for retirement accounts (e.g., change from INDIVIDUAL to ROLLOVER), so that legitimate edge cases are handled correctly.

**US-09:** As an operations analyst, I want every approve/reject action I take to be logged with my operator ID, timestamp, and reason, so that there is a complete audit trail for compliance.

**US-10:** As an operations analyst, I want to search and filter the review queue by date range, status, account, and amount, so that I can quickly find specific deposits.

### 4.3 System/Platform Stories

**US-11:** As the settlement engine, I want to batch all FundsPosted deposits into an X9 ICL-structured JSON file at the 6:30 PM CT cutoff, so that the Settlement Bank receives a complete and reconcilable file.

**US-12:** As the settlement engine, I want deposits submitted after the EOD cutoff to roll to the next business day (skipping weekends), so that settlement dates are always valid banking days.

**US-13:** As the funding service, I want to detect duplicate deposits using a composite key (routing + account + check number + amount) within a 30-day rolling window, so that the same check cannot be deposited twice.

**US-14:** As the platform, I want every financial operation (posting, reversal, fee deduction) to execute within a single database transaction using BEGIN IMMEDIATE, so that no partial state can exist.

**US-15:** As a platform engineer or reviewer, I want a dashboard that displays live benchmark status for gating correctness, settlement reconciliation, queue latency, return accuracy, and vendor scenario coverage, so that I can quickly verify whether the system is meeting its core success criteria during a demo or test run.

---

## 5. Functional Requirements (Prioritized)

Requirements are organized by priority. Items sourced from "Interview" were added or significantly refined based on the three rounds of architectural interviews.

### 5.1 P0: Critical (Must Ship)

| ID | Feature | Description | Source |
|---|---|---|---|
| FR-01 | Deposit Submission API | `POST /api/v1/deposits` accepting account ID, amount, check number, and image payloads. Returns transfer ID and initial status. | Original |
| FR-02 | Vendor Service Stub | Configurable stub returning 7 differentiated responses. Selection via magic account prefixes (`PASS-`, `BLUR-`, `GLARE-`, `MICR-`, `DUP-`, `MISMATCH-`) with `X-Vendor-Scenario` header override. | Original + Interview |
| FR-03 | Transfer State Machine | 8-state machine (Requested, Validating, Analyzing, Approved, FundsPosted, Completed, Rejected, Returned) with hardcoded transition map and audit-logged transitions. | Original + Interview |
| FR-04 | Double-Entry Ledger | Separate `transfers` and `ledger_entries` tables. Every posting creates balanced DEBIT + CREDIT entries in a single transaction. All amounts as int64 cents. | Original + Interview |
| FR-05 | Business Rule Engine | Deposit limit enforcement ($5,000 per correspondent config), contribution type defaults for retirement accounts, account eligibility check, session validation via static API keys. | Original + Interview |
| FR-06 | Dual-Layer Duplicate Detection | Vendor stub layer (image-based, configurable) + Funding service layer (composite key: routing + account + check# + amount in 30-day window). | Original + Interview |
| FR-07 | Settlement File Generation | X9 ICL-structured JSON with file header, cash letter, bundle, check detail, and image view records. Control totals at each level for reconciliation. | Original + Interview |
| FR-08 | EOD Cutoff with Injectable Clock | 6:30 PM CT cutoff via on-demand endpoint with optional `?as_of` parameter. Post-cutoff deposits roll to next business day (skip weekends). | Original + Interview |
| FR-09 | Return/Reversal Processing | Synchronous processing in single DB transaction: 4 ledger entries as 2 balanced pairs (reversal pair: DEBIT investor / CREDIT omnibus; fee pair: DEBIT investor $30 / CREDIT omnibus $30), state transition to Returned, notification event. Valid from both FundsPosted and Completed states. | Original + Interview |
| FR-10 | Operator Review Queue | Web UI showing flagged deposits with check images, MICR data, risk scores. Approve/reject with mandatory audit logging. Search and filter capabilities. | Original + Interview |

### 5.2 P1: Important (Should Ship)

| ID | Feature | Description | Source |
|---|---|---|---|
| FR-11 | Composite Risk Scoring | 4-signal model: MICR confidence, amount delta, limit proximity, first-deposit flag. Score 0–100 with LOW/MEDIUM/HIGH/CRITICAL levels driving queue sort order. | Interview |
| FR-12 | Structured Error Codes | Namespaced error taxonomy (`VENDOR.IQA_BLUR`, `FUNDING.OVER_LIMIT`, `STATE.INVALID_TRANSITION`) with machine-readable codes, human messages, and transfer ID reference. | Interview |
| FR-13 | Unified Event Log | `deposit_events` table recording every significant action with event type, actor, timestamp, and JSON payload. Serves both audit trail and per-deposit decision trace. | Interview |
| FR-14 | Contribution Type Override | Default-and-override pattern: retirement accounts get correspondent-configured default; operators can override via dropdown (INDIVIDUAL, EMPLOYER, ROLLOVER). | Interview |
| FR-15 | Pipeline Step Functions | Deposit processing as a sequence of named steps (vendor validation, business rules, ledger posting) with typed actions (Continue, HaltRejected, HaltFlagged, HaltApproved). | Interview |
| FR-16 | Structured Logging | `slog` with JSON output, `deposit_id` correlation on every entry, settlement monitoring for missed/delayed files. | Interview |
| FR-17 | Live Benchmark Dashboard | Web UI dashboard showing benchmark cards for gating correctness, settlement reconciliation, vendor scenario coverage, operator queue latency, return/reversal accuracy, setup/test status, and current deposit counts by state. Values update on page load and after benchmark-affecting actions. | Interview + PRD Revision |

### 5.3 P2: Nice-to-Have (If Time Permits)

| ID | Feature | Description | Source |
|---|---|---|---|
| FR-18 | Ledger Balance View | Account balance page with recent postings, accessible via `GET /api/v1/accounts/{id}/balance`. | Interview |
| FR-19 | Docker Compose Alternative | Containerized deployment option alongside native `make dev` for reviewers who prefer Docker. | Original |
| FR-20 | Settlement Bank Acknowledgment Tracking | POST endpoint to simulate bank acknowledgment; transitions deposits from FundsPosted to Completed. | Original |

---

## 6. User Flow & Experience

### 6.1 Happy Path: End-to-End Deposit

The following describes the primary workflow for a successful deposit, from submission through settlement. This flow was validated across all three interview rounds as the critical path for the Core Correctness rubric category (25 points).

| Step | Actor | Action | System Response | State |
|---|---|---|---|---|
| 1 | Investor | Opens mobile app, photographs front and back of check, enters deposit amount ($150.00) and account ID (PASS-10001) | Creates transfer record, stores images to `data/images/{transfer_id}/` | Requested |
| 2 | System | Pipeline Step 1: Sends deposit to Vendor Stub | Vendor stub reads `PASS-` prefix, returns CLEAN_PASS with extracted MICR data, 0.98 confidence score | Validating |
| 3 | System | Pipeline Step 2: Funding Service applies business rules | Validates session (`tok_alice_001`), resolves CORR-APEX omnibus account, checks $5,000 limit (PASS), checks duplicate (PASS), assigns INDIVIDUAL contribution type | Analyzing |
| 4 | System | Risk assessment computes score | Score: 0 (LOW) — no risk signals triggered. Does not require operator review. Auto-approves. | Approved |
| 5 | System | Pipeline Step 3: Posts to double-entry ledger | Creates DEBIT on OMNI-APEX-001 for 15000 cents, CREDIT on PASS-10001 for 15000 cents. Logs LEDGER_POSTED event. | FundsPosted |
| 6 | System | Settlement engine triggered (pre-6:30 PM CT cutoff) | Batches deposit into X9 JSON file. File control total: 15000 cents. Bundle count: 1. Item count: 1. | FundsPosted |
| 7 | Settlement Bank | Acknowledges settlement file | Transfer transitions to Completed. SETTLEMENT_CONFIRMED event logged. | Completed |

### 6.2 Failure Path: MICR Read Failure with Operator Review

| Step | Actor | Action | System Response | State |
|---|---|---|---|---|
| 1 | Investor | Submits deposit with account MICR-10001 | Transfer created | Requested |
| 2 | Vendor Stub | Returns MICR_READ_FAILURE, confidence: 0.42 | Error code: VENDOR.MICR_FAILURE. Deposit flagged for review. | Validating |
| 3 | System | Risk score computed: 65 (CRITICAL) | Signal: MICR confidence 0.42 (threshold: 0.90), weight: 23 points. Deposit enters operator queue at top priority. | Analyzing |
| 4 | Operator | Reviews check images, MICR data, risk signals. Clicks Approve with note. | OPERATOR_APPROVED event logged with operator ID, timestamp, notes. Ledger posted. | FundsPosted |

### 6.3 Return Path: Bounced Check Reversal

| Step | Actor | Action | System Response | State |
|---|---|---|---|---|
| 1 | Settlement Bank | Sends return notification (NSF reason code) | System receives `POST /api/v1/returns` with transfer_id and reason | FundsPosted |
| 2 | System | Processes return in single DB transaction | Creates 4 balanced ledger entries: 1) DEBIT investor 15000 cents / CREDIT omnibus 15000 cents (reversal pair). 2) DEBIT investor 3000 cents / CREDIT omnibus 3000 cents (fee pair). Logs RETURN_RECEIVED, REVERSAL_POSTED, INVESTOR_NOTIFIED events. | Returned |
| 3 | Investor | Receives notification | Message: check returned (NSF), $150.00 reversed, $30.00 fee applied. Net debit: $180.00. | Returned |

---

## 7. Technical Architecture

### 7.1 Technology Decisions Summary

The following decisions were made across three rounds of architectural interviews (30 total decisions). Each was evaluated against three strategies and selected based on first principles: zero gating errors, 100% settlement reconciliation, and maximum reviewer clarity.

| Domain | Decision | Rationale |
|---|---|---|
| Language | Go single-binary monolith | One-command setup, explicit control flow, native concurrency, no framework magic |
| Data Store | SQLite with double-entry ledger | Enforces debit=credit invariant; immutable audit trail; reconciliation-ready |
| Currency | Integer cents (int64) | Zero rounding errors; deterministic `==` comparison for reconciliation |
| State Machine | Hardcoded transition map | 8 states in 15 lines; pure, testable, no library overhead |
| API Design | Resource-oriented REST with `/api/v1/` prefix | Endpoint names trace deposit lifecycle; clean separation from UI routes |
| Vendor Stub | Magic account prefixes + header override | Self-documenting tests; deterministic; no hidden state |
| Operator UI | Embedded single-page web UI (Go embed) | Visual review of check images; one-binary setup preserved |
| Settlement | Structured JSON mirroring X9 record hierarchy | Demonstrates domain knowledge; human-readable; reconcilable control totals |
| EOD Cutoff | On-demand endpoint + injectable clock | Deterministic testing of time-dependent behavior |
| Configuration | Env vars (infra) + YAML files (domain) | Startup validation; zero-config `make dev`; YAML as documentation |
| Concurrency | BEGIN IMMEDIATE transactions | Atomic read-check-write; no double-posting |
| Audit | Unified `deposit_events` table | Single source for operator audit trail and per-deposit decision trace |
| Errors | Structured domain error codes | Machine-readable, testable, actionable in UI |
| Pipeline | Step function pattern | Each step independently testable; decision trace built into runner |
| Observability | `slog` + `deposit_id` correlation | Per-deposit traces via `jq`; settlement monitoring |
| Migrations | Embedded forward-only with version table | Server self-migrates; no external tools |

### 7.2 Database Schema

#### Core Tables

| Table | Purpose | Key Columns |
|---|---|---|
| `transfers` | Business object tracking deposit lifecycle | id (UUID), investor_account_id, correspondent_id, amount (int64 cents), status (enum), vendor_transaction_id, check_number, micr_data (JSON), risk_score, contribution_type, settlement_batch_id (nullable), created_at, updated_at |
| `ledger_entries` | Immutable double-entry financial records | id (UUID), transfer_id (FK), account_id, entry_type (DEBIT\|CREDIT), amount (int64 cents), memo, posted_at, reversal_of (nullable FK) |
| `deposit_events` | Unified audit trail and decision trace (consolidates both state transition logging and per-deposit event history into a single table) | id, transfer_id (FK), event_type (enum), actor, payload (JSON), created_at |
| `schema_migrations` | Forward-only migration tracking | version (INT PK), applied_at (TIMESTAMP) |

> **Consolidation note:** The `deposit_events` table serves as **both** the state transition audit log and the general-purpose event log. There is no separate `transfer_state_log` table. State transitions are recorded as `deposit_events` with event types like `STATE_TRANSITION` and a payload containing `from_state`, `to_state`, `actor`, and `reason`. This avoids redundant logging while preserving the full audit trail. All references to `transfer_state_log` in the architectural blueprint should be read as `deposit_events` with a `STATE_TRANSITION` event type.

> **Settlement guard:** The `settlement_batch_id` column on `transfers` prevents double-batching. When a deposit is included in a settlement file, its `settlement_batch_id` is set within the same transaction that generates the file. The settlement query filters on `WHERE settlement_batch_id IS NULL AND status = 'FundsPosted'`, ensuring no deposit can appear in two settlement files.

### 7.3 Component Interaction Diagram

```
                          ┌─────────────────────────────────────────────┐
                          │              HTTP Layer (api/)              │
                          │  Auth Middleware → Router → Handlers        │
                          └────┬──────┬──────┬──────┬──────┬───────────┘
                               │      │      │      │      │
                    ┌──────────┘      │      │      │      └──────────┐
                    ▼                 ▼      │      ▼                 ▼
            ┌──────────────┐  ┌────────────┐ │ ┌──────────┐  ┌──────────────┐
            │ Vendor Stub  │  │  Funding   │ │ │ Operator │  │   Returns    │
            │  (vendor/)   │  │  Service   │ │ │(operator/)│  │  (returns/)  │
            │              │  │ (funding/) │ │ │           │  │              │
            │ • Scenario   │  │ • Session  │ │ │ • Queue   │  │ • Validate   │
            │   routing    │  │   auth     │ │ │   queries │  │   state      │
            │ • MICR data  │  │ • Rules    │ │ │ • Approve │  │ • Reversal   │
            │ • Images     │  │ • Acct     │ │ │ • Reject  │  │   entries    │
            └──────┬───────┘  │   resolve  │ │ │ • Audit   │  │ • Fee calc   │
                   │          └──────┬─────┘ │ └─────┬─────┘  └──────┬───────┘
                   │                 │       │       │               │
                   └────────┬────────┘       │       └───────┬───────┘
                            ▼                │               │
                   ┌─────────────────┐       │               │
                   │    Pipeline     │       │               │
                   │  Orchestrator   │       │               │
                   │   (funding/)    │       │               │
                   │                 │       │               │
                   │ Step 1: Vendor  │       │               │
                   │ Step 2: Rules   │       │               │
                   │ Step 3: Ledger  │       │               │
                   └────────┬────────┘       │               │
                            │                │               │
                            ▼                ▼               ▼
                   ┌─────────────────────────────────────────────────┐
                   │              Ledger Service (funding/)          │
                   │  • postToLedger(): balanced DEBIT+CREDIT pairs  │
                   │  • BEGIN IMMEDIATE transaction boundaries       │
                   └────────────────────┬────────────────────────────┘
                                        │
                   ┌────────────────────┐│┌──────────────────────────┐
                   │  Settlement Engine │││    Store Layer (store/)   │
                   │  (settlement/)     │││                          │
                   │                    │▼│  • SQLite connection     │
                   │  • X9 JSON gen     │ │  • Migration runner      │
                   │  • EOD cutoff      ├─┤  • Repository methods    │
                   │  • Batch guard     │ │  • Seed data             │
                   └────────────────────┘ └──────────┬───────────────┘
                                                     │
                                                     ▼
                                              ┌──────────────┐
                                              │   SQLite DB   │
                                              │               │
                                              │ • transfers   │
                                              │ • ledger_entries│
                                              │ • deposit_events│
                                              └──────────────┘
```

**Request flow (happy path):**
1. Client sends `POST /api/v1/deposits` with Bearer token
2. Auth middleware validates token → handler creates transfer record (Requested)
3. Pipeline orchestrator executes Step 1: Vendor Stub → validates images → returns CLEAN_PASS (Validating)
4. Pipeline Step 2: Funding Service → applies rules → auto-approves (Analyzing → Approved)
5. Pipeline Step 3: Ledger Service → posts balanced DEBIT/CREDIT pair (Approved → FundsPosted)
6. Later: `POST /api/v1/settlement/batches` → Settlement Engine queries unbatched FundsPosted deposits → generates X9 JSON → marks deposits with `settlement_batch_id`
7. Settlement acknowledgment → FundsPosted → Completed

### 7.4 State Transition → Side-Effect Mapping

Every state transition triggers specific side effects. This table is the authoritative specification for the pipeline orchestrator (TICKET-007) and related handlers.

| Transition | Trigger | Side Effects | Actor |
|---|---|---|---|
| Requested → Validating | Pipeline Step 1 starts | Send deposit to vendor stub; store vendor response on transfer record | system |
| Validating → Analyzing | Vendor returns non-terminal result (CLEAN_PASS, MICR_FAILURE, AMOUNT_MISMATCH) | Compute risk score; store score on transfer; if HIGH/CRITICAL, flag for operator review | system |
| Validating → Rejected | Vendor returns terminal failure (IQA_BLUR, IQA_GLARE, DUPLICATE) | Log rejection reason; emit DEPOSIT_REJECTED event; no ledger entries created | system |
| Analyzing → Approved | Business rules pass + risk LOW (auto-approve) OR operator clicks Approve | Log approval with actor attribution; emit DEPOSIT_APPROVED event | system or operator:\<id\> |
| Analyzing → Rejected | Business rules fail (OVER_LIMIT, DUPLICATE, INELIGIBLE) OR operator clicks Reject | Log rejection reason; emit DEPOSIT_REJECTED event; no ledger entries created | system or operator:\<id\> |
| Approved → FundsPosted | Pipeline Step 3 (immediate after Approved) | **Post to ledger:** DEBIT omnibus + CREDIT investor (balanced pair, single tx); emit LEDGER_POSTED event | system |
| FundsPosted → Completed | Settlement acknowledgment received | Emit SETTLEMENT_CONFIRMED event | system |
| FundsPosted → Returned | Return notification received | **Reversal:** 4 ledger entries (2 balanced pairs: reversal + fee) in single tx; emit RETURN_RECEIVED, REVERSAL_POSTED, INVESTOR_NOTIFIED events | system |
| Completed → Returned | Late return notification received | Same as FundsPosted → Returned | system |

> **Invariant:** No ledger entries are ever created for transitions to Rejected. Ledger entries are only created at `Approved → FundsPosted` (posting) and `FundsPosted/Completed → Returned` (reversal). This guarantees that the ledger only contains financially meaningful records.

### 7.5 Repository Layout

The repository follows a domain-oriented package layout where directory structure directly mirrors the spec's service boundaries:

```
/
├── cmd/server/main.go              — Entry point. Wires all services, starts HTTP server.
├── internal/
│   ├── domain/                     — Transfer model, Amount type (int64), state machine, error codes.
│   ├── vendor/                     — Vendor stub with scenario routing and response types.
│   ├── funding/                    — Business rules, duplicate detection, ledger posting, account resolver.
│   ├── operator/                   — Review queue queries, approve/reject actions, audit logging.
│   ├── settlement/                 — X9 JSON generator, EOD cutoff logic with injectable clock.
│   ├── returns/                    — Return processing, reversal posting, fee calculation.
│   ├── store/                      — SQLite setup, migrations, repository, seed data.
│   └── api/                        — Router, HTTP handlers, auth middleware, error formatting.
├── web/                            — Embedded HTML/CSS/JS for dashboard, submission form, operator queue.
├── config/                         — correspondents.yaml, investors.yaml (domain seed data).
├── scripts/demo.sh                 — Narrated 6-act scenario runner with pass/fail badges.
├── docs/
│   ├── architecture.md
│   └── decision_log.md             — 10 ADRs in standard format.
├── reports/                        — Generated test/demo reports.
├── data/                           — Runtime data (SQLite DB, images).
├── .env.example
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

### 7.6 REST API Endpoints

```
# Core deposit lifecycle
POST   /api/v1/deposits                          # Submit new deposit
GET    /api/v1/deposits/{id}                      # Get deposit with full state
GET    /api/v1/deposits/{id}/images/{side}        # Get check image (front/back)
GET    /api/v1/deposits/{id}/history              # State transition audit log
GET    /api/v1/deposits                           # List with filters (?status=, ?account=, ?from=, ?to=)

# Operator actions
GET    /api/v1/operator/queue                     # Flagged deposits for review
POST   /api/v1/operator/queue/{id}/approve        # Approve with audit entry
POST   /api/v1/operator/queue/{id}/reject         # Reject with reason (in body)

# Ledger and accounts
GET    /api/v1/accounts/{id}/balance              # Current balance
GET    /api/v1/accounts/{id}/ledger               # Ledger entries for account

# Settlement
POST   /api/v1/settlement/batches                 # Trigger batch generation
GET    /api/v1/settlement/batches/{id}            # Get settlement file
GET    /api/v1/settlement/batches/{id}/items      # Items in batch

# Returns
POST   /api/v1/returns                            # Simulate check return
GET    /api/v1/returns/{id}                       # Return details with reversal info
```

### 7.7 Error Code Taxonomy

```
VENDOR.IQA_BLUR            → 422  Image quality: blur detected
VENDOR.IQA_GLARE           → 422  Image quality: glare detected
VENDOR.MICR_FAILURE        → 422  MICR line unreadable (flags for review)
VENDOR.DUPLICATE           → 409  Check previously deposited
VENDOR.AMOUNT_MISMATCH     → 422  OCR amount ≠ entered amount (flags for review)
FUNDING.OVER_LIMIT         → 422  Deposit exceeds correspondent limit
FUNDING.DUPLICATE          → 409  Duplicate detected by business rules
FUNDING.ACCOUNT_NOT_FOUND  → 404  Account identifier not resolvable
FUNDING.INELIGIBLE         → 403  Account not eligible for check deposit
STATE.INVALID_TRANSITION   → 409  Requested state transition not allowed
SETTLEMENT.CUTOFF_PASSED   → 422  Past EOD cutoff, rolled to next business day
SYSTEM.INTERNAL            → 500  Unexpected error (logged with correlation ID)
```

### 7.8 Transfer State Machine

```
                                        ┌──────────────────────────────────┐
                                        │          (auto-approve)          │
                                        ▼                                  │
Requested ──→ Validating ──→ Analyzing ──→ Approved ──→ FundsPosted ──→ Completed
                  │               │                         │               │
                  └──→ Rejected   └──→ Rejected             └──→ Returned ◄─┘
```

| State | Description | Valid Transitions |
|---|---|---|
| Requested | Deposit submitted by investor | Validating |
| Validating | Sent to Vendor Service for IQA/MICR/OCR | Analyzing, Rejected |
| Analyzing | Business rules being applied by Funding Service | Approved, Rejected |
| Approved | Passed all checks; awaiting ledger posting. **Always persisted** — even auto-approved deposits log this transition before proceeding to FundsPosted, ensuring a complete audit trail. | FundsPosted |
| FundsPosted | Provisional credit posted to investor account | Completed, Returned |
| Completed | Settlement confirmed by Settlement Bank | Returned |
| Rejected | Failed validation, business rules, or operator review | *(terminal)* |
| Returned | Check bounced after settlement; reversal posted | *(terminal)* |

> **Implementation note:** The `Approved` state must always be persisted as a discrete, logged transition — even for auto-approved deposits where the pipeline immediately continues to ledger posting. The transition sequence is always `Analyzing → Approved → FundsPosted`, never `Analyzing → FundsPosted` directly. This ensures the `deposit_events` audit trail is complete and that operators can distinguish auto-approved from operator-approved deposits by checking the actor field (`system` vs `operator:<id>`).

---

## 8. Phasing & Roadmap

### Phase 1: MVP (Target: Submission-Ready)

The leanest version that addresses all P0 requirements and achieves passing scores across all rubric categories. This phase covers the complete deposit lifecycle and all 7 vendor stub scenarios.

| Milestone | Deliverables | Rubric Coverage |
|---|---|---|
| M1: Foundation | Go project scaffolding, SQLite schema with migrations, domain types (Transfer, Amount, State Machine), configuration loading with startup validation | System Design (20 pts) |
| M2: Vendor Stub | 7-scenario vendor stub with magic account prefixes + header override. Unit tests for each scenario response. | Stub Quality (15 pts) |
| M3: Funding Service | Session validation, account resolution, business rule engine (limits, duplicates, contribution types), double-entry ledger posting | Core Correctness (25 pts) |
| M4: Pipeline & State Machine | Step function pipeline wiring vendor and funding. State machine enforcement with audit-logged transitions. | Core Correctness (25 pts) |
| M5: Operator Workflow | Embedded web UI with review queue, check image display, approve/reject controls, risk scoring, audit logging, and live benchmark dashboard cards | Operator Workflow (10 pts) |
| M6: Settlement Engine | X9 JSON generator with hierarchical structure, EOD cutoff with injectable clock, business day rollover | Core Correctness (25 pts) |
| M7: Return/Reversal | Return processing endpoint, synchronous reversal with fee deduction, notification event recording | Return Handling (10 pts) |
| M8: Tests & Demo | 20+ tests (integration + unit), narrated demo script (6 acts), Makefile workflow, test report generation | Tests (10 pts) + DevEx (10 pts) |
| M9: Documentation | README, architecture.md, decision_log.md (10 ADRs), .env.example, SUBMISSION.md, risks/limitations | System Design (20 pts) + DevEx (10 pts) |

### Phase 2: V1.1 (Optimization)

Post-MVP enhancements that improve operational quality but are not required for a passing submission.

| Feature | Description | Priority |
|---|---|---|
| Enhanced Dashboard | Expanded analytics page with deposit volume charts, benchmark history, status distribution, and daily trends beyond the MVP live benchmark cards | P2 |
| Ledger Explorer | Paginated ledger entry viewer with account balance calculations and export capability | P2 |
| Settlement Monitoring | Background check for missed settlement files 30 minutes past cutoff with slog warnings | P1 |
| Docker Compose | Multi-stage Dockerfile + docker-compose.yml for containerized deployment alternative | P2 |
| Batch Return Processing | Endpoint accepting multiple return notifications in a single request | P2 |

### Phase 3: Future Vision

Long-term capabilities that would move the system toward production readiness. These are documented in the submission as "With one more week, we would:" items.

| Feature | Description |
|---|---|
| Real Vendor Integration | Replace stub with actual vendor API client; maintain stub for testing via feature flag |
| PostgreSQL Migration | Move from SQLite to PostgreSQL for concurrent write support and production-scale durability |
| Holiday Calendar | Business day calculation incorporating Federal Reserve holiday schedule |
| Binary X9.37 Output | Generate actual EBCDIC-encoded X9.37 files for direct bank submission |
| OAuth/JWT Authentication | Replace static API keys with token-based auth supporting expiry and refresh |
| Real-Time Notifications | WebSocket or SSE push for deposit status updates to mobile client |
| Multi-Currency Support | Extend Amount type to support currency codes and exchange rate lookups |

---

## 9. Implementation Tickets (Phase 1 Backlog)

The following tickets represent the development backlog for Phase 1. Each ticket is scoped for independent implementation and testing. T-shirt sizes reflect relative effort: S (< 0.5 day), M (0.5–1 day), L (1–2 days), XL (2–3 days).

---

### TICKET-001: Project Scaffolding and Configuration System

**Size:** M (0.5–1 day) | **Milestone:** M1 — Foundation

Initialize Go module, establish domain-oriented package structure, implement layered configuration loading (env vars for infrastructure + YAML for domain data), and create startup validation that fails fast on misconfiguration. Seed `correspondents.yaml` with 3 firms and `investors.yaml` with 4–6 test investors mapped to magic account prefixes.

**Acceptance Criteria:**
- [ ] `go build ./cmd/server` compiles without errors.
- [ ] `make dev` creates directories, copies `.env.example` to `.env`, builds, and starts the server.
- [ ] Server refuses to start if `correspondents.yaml` references non-existent omnibus accounts.
- [ ] Config loaded and printed to structured log at startup.

---

### TICKET-002: SQLite Schema and Migration System

**Size:** M (0.5–1 day) | **Milestone:** M1 — Foundation

Implement embedded forward-only migration runner with `schema_migrations` version table. Create initial migration with `transfers`, `ledger_entries`, and `deposit_events` tables. All amount columns as INTEGER (cents). Set up database connection pooling with BEGIN IMMEDIATE transaction support.

**Acceptance Criteria:**
- [ ] Server runs migrations on startup; `schema_migrations` table tracks applied versions.
- [ ] Restarting server skips already-applied migrations.
- [ ] `transfers` table has all required columns including `risk_score` and `contribution_type`.
- [ ] `ledger_entries` has `reversal_of` nullable FK for return handling.
- [ ] Composite index on `transfers` (`micr_routing`, `micr_account`, `check_number`) for duplicate detection.

---

### TICKET-003: Domain Types and State Machine

**Size:** S (< 0.5 day) | **Milestone:** M1 — Foundation

Implement core domain types: `Amount` (int64 cents with `ToDollars`/`ParseAmount`), Transfer model, State enum with 8 values, and hardcoded transition map. State machine `Transition()` function validates transitions and logs to `deposit_events` (as `STATE_TRANSITION` event type). Structured error code taxonomy (`VENDOR.*`, `FUNDING.*`, `STATE.*`).

**Acceptance Criteria:**
- [ ] `Amount(15000).ToDollars()` returns `"$150.00"`.
- [ ] `ParseAmount("150.00")` returns `Amount(15000)`.
- [ ] `ParseAmount("150.001")` returns error (fractional cents).
- [ ] `Transition(Requested, Completed)` returns `STATE.INVALID_TRANSITION` error.
- [ ] All valid transitions succeed; all invalid transitions fail. Covered by unit tests.

---

### TICKET-004: Vendor Service Stub

**Size:** L (1–2 days) | **Milestone:** M2 — Vendor Stub

Build configurable vendor stub with 7 differentiated response scenarios. Primary selection via account number prefix mapping; secondary selection via `X-Vendor-Scenario` header override. Each response includes appropriate MICR data, confidence scores, error codes, and transaction IDs. Synthetic check image generation at startup.

**Acceptance Criteria:**
- [ ] `PASS-*` account returns CLEAN_PASS with MICR data and confidence >= 0.95.
- [ ] `BLUR-*` returns IQA_FAIL_BLUR with error code `VENDOR.IQA_BLUR`.
- [ ] `GLARE-*` returns IQA_FAIL_GLARE.
- [ ] `MICR-*` returns MICR_READ_FAILURE with confidence 0.42.
- [ ] `DUP-*` returns DUPLICATE_DETECTED.
- [ ] `MISMATCH-*` returns AMOUNT_MISMATCH with OCR amount differing from entered amount.
- [ ] `X-Vendor-Scenario` header overrides prefix behavior when present.
- [ ] Synthetic PNG check images generated for each test deposit in `data/images/`.

---

### TICKET-005: Funding Service and Business Rule Engine

**Size:** L (1–2 days) | **Milestone:** M3 — Funding Service

Implement session validation (static API key lookup), account resolution (investor → correspondent → omnibus), and business rule engine: deposit limit check (per-correspondent from YAML config), dual-layer duplicate detection (composite key in 30-day window), contribution type defaulting for retirement accounts, and account eligibility verification.

**Acceptance Criteria:**
- [x] Request with invalid/missing API key returns 401 with structured error.
- [x] Request from ineligible investor (Dave Wilson) returns 403.
- [x] Deposit of $6,000 against CORR-APEX ($5,000 limit) returns `FUNDING.OVER_LIMIT`.
- [x] Same check submitted twice within 30 days returns `FUNDING.DUPLICATE`.
- [x] IRA account gets INDIVIDUAL contribution type from CORR-APEX config.
- [x] Omnibus account correctly resolved per correspondent (`OMNI-APEX-001` for CORR-APEX).

---

### TICKET-006: Double-Entry Ledger Posting

**Size:** M (0.5–1 day) | **Milestone:** M3 — Funding Service

Implement ledger posting service that creates balanced DEBIT + CREDIT entry pairs within a single BEGIN IMMEDIATE transaction. Transfer attributes: To AccountId (investor), From AccountId (omnibus), Type: MOVEMENT, Memo: FREE, SubType: DEPOSIT, Transfer Type: CHECK, Currency: USD.

**Acceptance Criteria:**
- [x] Every posting creates exactly one DEBIT and one CREDIT of equal amounts.
- [x] Both entries created in same DB transaction (BEGIN IMMEDIATE).
- [x] Balance query (SUM of credits minus debits) returns correct account balance.
- [x] `TestLedgerInvariant`: sum of all DEBITs == sum of all CREDITs across all accounts.

---

### TICKET-007: Deposit Pipeline Orchestration

**Size:** M (0.5–1 day) | **Milestone:** M4 — Pipeline

Wire the vendor stub and funding service into a step function pipeline. Each step receives the deposit and returns a typed action (Continue, HaltRejected, HaltFlagged, HaltApproved). Pipeline runner executes steps in sequence, manages state transitions per the Side-Effect Mapping (Section 7.4), and logs each step to `deposit_events`.

**Acceptance Criteria:**
- [x] Clean deposit flows: Requested → Validating → Analyzing → Approved → FundsPosted (Approved state always persisted, even for auto-approved deposits).
- [x] BLUR deposit flows: Requested → Validating → Rejected (pipeline halts at step 1).
- [x] MICR failure flows: Requested → Validating → Analyzing (flagged, needs review).
- [x] Each pipeline step logged as `deposit_event` with step index and action.
- [x] Auto-approved deposits have actor=`system` on the Analyzing→Approved transition; operator-approved deposits have actor=`operator:<id>`.

---

### TICKET-008: REST API Layer

**Size:** L (1–2 days) | **Milestone:** M4 — Pipeline

Implement resource-oriented REST API with `/api/v1/` prefix. All endpoints per Section 7.6. Auth middleware validates Bearer tokens. Structured error responses per Section 7.7.

**Acceptance Criteria:**
- [x] All endpoints return structured JSON with appropriate HTTP status codes.
- [x] Error responses include `code`, `message`, `transfer_id`, and `details` fields.
- [x] Auth middleware rejects requests without valid Bearer token.
- [x] Image endpoint serves check images via `http.FileServer`.
- [x] Operator queue supports `?status=`, `?account=`, `?from=`, `?to=` query filters.

---

### TICKET-009: Operator Web UI

**Size:** L (1–2 days) | **Milestone:** M5 — Operator Workflow

Build embedded single-page web UI using vanilla HTML/CSS/JS compiled into the Go binary via `go:embed`. Pages: dashboard (`/`), deposit submission form (`/submit`), operator review queue (`/operator`), transfer detail (`/transfers/:id`). The dashboard displays live benchmark cards for core KPIs, and the operator queue displays risk scores, check images, MICR data, and approve/reject buttons with contribution type override dropdown.

**Acceptance Criteria:**
- [ ] Opening `localhost:8080/` shows benchmark cards for gating correctness, settlement reconciliation, vendor scenario coverage, queue latency, return accuracy, and deposit counts by state.
- [ ] Benchmark cards refresh after deposit submission, operator approval/rejection, settlement batch generation, and return processing.
- [ ] Benchmark values are derived from live system state and/or latest test/demo run artifacts, not hardcoded text.
- [ ] Opening `localhost:8080/operator` shows flagged deposits sorted by risk score (highest first).
- [ ] Each queue item displays: risk score badge, amount, account, check images, MICR data.
- [ ] Approve button posts to `/api/v1/operator/queue/{id}/approve` and updates UI.
- [ ] Reject button prompts for reason before posting.
- [ ] Contribution type dropdown shows INDIVIDUAL, EMPLOYER, ROLLOVER options.
- [ ] Transfer detail page (`/transfers/:id`) displays the full per-deposit decision trace: deposit inputs, vendor response, business rules applied, operator actions (if any), and settlement status — sourced from `deposit_events`.

---

### TICKET-010: Settlement Engine

**Size:** L (1–2 days) | **Milestone:** M6 — Settlement

Implement X9 ICL-structured JSON settlement file generator. Hierarchical structure: file header, cash letter header, bundle header, check detail records (with MICR data and amounts), image view records, and control totals at each level. EOD cutoff at 6:30 PM CT with injectable clock (`?as_of` parameter). Next-business-day rollover skipping weekends.

**Acceptance Criteria:**
- [ ] Generated file has `file_header`, `cash_letters[]`, and `file_control` structure.
- [ ] `file_control.total_amount` equals sum of all check detail amounts.
- [ ] Deposits submitted after 6:30 PM CT get next business day settlement date.
- [ ] Friday 7:00 PM CT deposit rolls to Monday.
- [ ] Rejected deposits are never included in settlement files.
- [ ] Deposits included in a batch have `settlement_batch_id` set; generating a second batch does not re-include them (no double-batching).
- [ ] Batch generation and `settlement_batch_id` assignment happen within the same database transaction.

---

### TICKET-011: Return/Reversal Processing

**Size:** M (0.5–1 day) | **Milestone:** M7 — Returns

Implement return handling endpoint (`POST /api/v1/returns`) that processes the full reversal in a single database transaction: validates transfer state (must be FundsPosted or Completed), creates reversal ledger entries, transitions to Returned state, and logs all events.

Reversal creates exactly **4 ledger entries as 2 balanced pairs:**
1. **Reversal pair:** DEBIT investor (original amount) / CREDIT omnibus (original amount)
2. **Fee pair:** DEBIT investor (3000 cents = $30.00) / CREDIT omnibus (3000 cents)

Both pairs are inserted within a single `BEGIN IMMEDIATE` transaction via two calls to `postToLedger()`.

**Acceptance Criteria:**
- [ ] Return on FundsPosted deposit succeeds; transfer moves to Returned.
- [ ] Return on Completed deposit succeeds; transfer moves to Returned (late returns).
- [ ] Reversal creates exactly 4 ledger entries: 2 balanced pairs (reversal + fee), each with matching DEBIT and CREDIT amounts.
- [ ] Fee amount is exactly 3000 cents ($30.00).
- [ ] Return on Requested deposit returns `STATE.INVALID_TRANSITION` error.
- [ ] Post-reversal, ledger debits still equal credits (invariant preserved).
- [ ] `INVESTOR_NOTIFIED` event payload includes: original amount, fee amount, net debit, reason code (e.g., NSF), and human-readable message.

---

### TICKET-012: Programmatic Data Seeding

**Size:** M (0.5–1 day) | **Milestone:** M8 — Tests & Demo

Implement programmatic seed function that creates a complete demo-ready universe on first startup: correspondents, investors with magic-prefix accounts, sample deposits in every state (Completed, Analyzing/flagged, Returned), ledger entries for completed deposits, `deposit_events` for audit trails, and synthetic check images on disk.

**Acceptance Criteria:**
- [ ] Server starts with populated data on fresh database.
- [ ] Dashboard shows deposits in at least 4 different states.
- [ ] Operator queue has at least 2 flagged items with check images.
- [ ] Seed is idempotent (safe to restart without duplicating data).

---

### TICKET-013: Test Suite

**Size:** XL (2–3 days) | **Milestone:** M8 — Tests & Demo

Write 20+ tests: integration tests with real in-memory SQLite exercising the full HTTP API, plus unit tests for pure logic. Tests cover: happy path E2E, all 7 vendor scenarios, business rule enforcement, operator approve/reject, return/reversal with fee, settlement file contents and double-batching guard, auth failures, ledger balance invariant, state machine transitions, amount parsing, and cutoff logic.

**Test Manifest:**

| # | Test Name | Type | What It Proves |
|---|---|---|---|
| 1 | `TestIntegration_HappyPath` | Integration | End-to-end deposit → settlement works |
| 2 | `TestIntegration_VendorBlur` | Integration | IQA blur → Rejected with correct error code |
| 3 | `TestIntegration_VendorGlare` | Integration | IQA glare → Rejected |
| 4 | `TestIntegration_VendorMICRFailure` | Integration | MICR failure → flagged for operator review |
| 5 | `TestIntegration_VendorDuplicate` | Integration | Vendor duplicate → Rejected |
| 6 | `TestIntegration_VendorAmountMismatch` | Integration | Amount mismatch → flagged for review |
| 7 | `TestIntegration_OverDepositLimit` | Integration | $6,000 deposit → Rejected by funding rules |
| 8 | `TestIntegration_FundingDuplicateDetection` | Integration | Same check submitted twice → second rejected |
| 9 | `TestIntegration_OperatorApprove` | Integration | Operator approves flagged → FundsPosted |
| 10 | `TestIntegration_OperatorReject` | Integration | Operator rejects flagged → Rejected |
| 11 | `TestIntegration_ReturnAndReversal` | Integration | Return → reversal + $30 fee + Returned state |
| 12 | `TestIntegration_SettlementFileContents` | Integration | Settlement file has correct X9 structure and totals |
| 13 | `TestIntegration_UnauthenticatedRequest` | Integration | No token → 401 |
| 14 | `TestIntegration_IneligibleAccount` | Integration | Suspended investor → 403 |
| 15 | `TestStateMachine_InvalidTransitions` | Unit | Every invalid state pair is rejected |
| 16 | `TestAmountParsing_EdgeCases` | Unit | Fractional cents, negatives, overflow rejected |
| 17 | `TestCutoff_BusinessDayLogic` | Unit | Friday evening → Monday, weekend → Monday |
| 18 | `TestLedgerInvariant_DebitsEqualCredits` | Integration | Sum of all debits == sum of all credits |
| 19 | `TestSettlement_NoDoubleBatching` | Integration | Second batch generation returns zero items; no deposit appears in two files |
| 20 | `TestReturnOnCompletedDeposit` | Integration | Late return on Completed deposit succeeds with correct reversal |

> **Shift-left testing note:** Tests 15–17 (state machine, amount parsing, cutoff logic) are pure unit tests with no service dependencies. These should be written during Phase 1 alongside TICKET-003 and TICKET-006 to catch foundational bugs early, rather than waiting for Phase 3. The remaining integration tests require the full service stack and belong in Phase 3.

**Acceptance Criteria:**
- [ ] `go test ./... -count=1` passes with 20+ tests.
- [ ] `TestLedgerInvariant_DebitsEqualCredits` passes after running all scenarios.
- [ ] Each of the 7 vendor stub scenarios has a named integration test.
- [ ] `TestSettlement_NoDoubleBatching` verifies idempotent batch generation.
- [ ] Coverage report generated at `reports/coverage.out`.

---

### TICKET-014: Demo Script and Makefile

**Size:** M (0.5–1 day) | **Milestone:** M8 — Tests & Demo

Write narrated 6-act demo script (`scripts/demo.sh`) with colored pass/fail output and summary. Acts: Happy Path, Vendor Rejections, Business Rules, Operator Review, Settlement, Return/Reversal. Create Makefile with targets: `help` (default), `dev`, `test`, `demo`, `demo-full`, `report`, `docker`, `clean`. `make help` prints all available commands.

**Acceptance Criteria:**
- [ ] `make help` (default target) lists all available commands.
- [ ] `make dev` builds, seeds, and starts server with printed URLs.
- [ ] `make demo` runs all scenarios and outputs formatted results.
- [ ] `make demo-full` starts server in background, waits for readiness, runs demo, stops server (fully self-contained single-command demo).
- [ ] `make report` generates test + demo reports in `/reports`.
- [ ] Demo output saved to `reports/demo_results.txt`.
- [ ] `demo.sh` checks for `jq` presence at startup and prints install instructions if missing.

---

### TICKET-015: Documentation Package

**Size:** M (0.5–1 day) | **Milestone:** M9 — Documentation

Write `README.md` (setup, architecture summary, how to demo, disclaimers), `docs/architecture.md` (system diagram, data flow, service boundaries), `docs/decision_log.md` (10 ADRs in standard format: Context, Decision, Alternatives, Consequences), `.env.example`, `SUBMISSION.md`, and risks/limitations note.

**Acceptance Criteria:**
- [ ] README includes copy-paste commands to run the system.
- [ ] Decision log has 10 ADRs covering language, data store, vendor stub, settlement, state machine, currency, risk scoring, API, concurrency, and pipeline.
- [ ] Architecture doc includes system diagram and data flow.
- [ ] `SUBMISSION.md` includes all required fields: Project name, Summary (3-5 sentences), How to run (copy-paste commands), Test/eval results (with link to `/reports`), "With one more week, we would:", Risks and limitations, "How should ACME evaluate production readiness?"
- [ ] Short write-up (<=1 page) covering architecture choices, vendor stub design, state machine rationale, and risks/limitations is included in `SUBMISSION.md` or as a dedicated section in the README.
- [ ] `reports/` contains scenario coverage summary mapping which test exercised which system path.

---

## 10. Assumptions, Constraints & Open Questions

### 10.1 Assumptions

**A-1:** All data is synthetic. No real PII, check images, account numbers, or routing numbers will be used in any environment.

**A-2:** The vendor integration is entirely stubbed. The stub's response selection mechanism (magic prefixes + headers) is sufficient for demonstrating all validation scenarios.

**A-3:** SQLite's single-writer concurrency model is acceptable for MVP-scale throughput. Production migration to PostgreSQL is a Phase 3 consideration.

**A-4:** The $5,000 deposit limit and $30 return fee are hardcoded for MVP. Per-correspondent configurability of fee amounts is a Phase 2 enhancement.

**A-5:** Weekend-only exclusion for business day calculation is sufficient. Federal Reserve holiday calendar integration is Phase 3.

**A-6:** The system will be evaluated on a single machine. Cross-machine deployment, TLS, and production hardening are out of scope.

### 10.2 Constraints

**C-1:** Language: Go or Java (Go selected with documented justification).

**C-2:** Setup: Must be achievable via a single command (`make dev` or `docker compose up`).

**C-3:** Testing: Minimum 10 tests required (target: 20+).

**C-4:** Data: Secrets must use environment variables with `.env.example` provided.

**C-5:** Vendor stub must be configurable without code changes.

**C-6:** No AI/ML frameworks required or expected.

### 10.3 Open Questions

| ID | Question | Impact | Resolution Path |
|---|---|---|---|
| OQ-1 | Should the vendor stub simulate network latency (e.g., 200ms delay) to test timeout handling? | Medium — affects realism of demo but adds timing complexity to tests | Defer to Phase 2; MVP stub responds immediately |
| OQ-2 | Should settlement file generation run automatically on a schedule, or only on-demand? | Low — injectable clock covers demo needs | Resolved: on-demand with injectable clock (Interview Q7) |
| OQ-3 | What level of MICR data fidelity is expected in the stub responses? | Medium — affects settlement file realism | Use realistic but synthetic routing/account numbers (e.g., 021000021 is a known test routing number) |
| OQ-4 | Should operator authentication be separate from investor authentication? | Low — affects audit trail attribution | Use separate API key prefixes (`tok_alice_` for investors, `tok_operator_` for operators) |
| OQ-5 | How should the system handle concurrent return notifications for the same deposit? | Low at MVP scale — BEGIN IMMEDIATE serializes writes | Resolved: database-level serialization via BEGIN IMMEDIATE (Interview Q20) |

---

## Appendix A: Rubric Traceability Matrix

This matrix maps every evaluation rubric category to the specific PRD sections, functional requirements, implementation tickets, and tests that address it.

| Rubric Category | Points | PRD Sections | Key Tickets | Validation Tests |
|---|---|---|---|---|
| System Design & Architecture | 20 | Sec 7 (Architecture, Component Diagram 7.3, Side-Effect Map 7.4), Sec 8 (Roadmap) | TICKET-001, 002, 003, 015 | State machine transitions, config validation |
| Core Correctness | 25 | Sec 5.1 (FR-01 to FR-09), Sec 6 (User Flows), Sec 7.4 (Side-Effect Map) | TICKET-005, 006, 007, 010 | Happy path E2E, ledger invariant, settlement reconciliation, no double-batching |
| Vendor Stub Quality | 15 | Sec 5.1 (FR-02), Sec 4.1 (US-02) | TICKET-004 | 7 scenario tests, header override test |
| Operator Workflow | 10 | Sec 5.1 (FR-10), Sec 5.2 (FR-11, 13, 14) | TICKET-009 | Approve/reject tests, audit log verification |
| Return/Reversal | 10 | Sec 5.1 (FR-09), Sec 6.3 (Return Flow) | TICKET-011 | Reversal + fee test (4 entries / 2 balanced pairs), ledger balance post-reversal, return on Completed |
| Tests & Evaluation | 10 | Sec 3 (KPIs), Sec 9 (Tickets) | TICKET-013, 014 | 20+ tests, demo script, coverage report |
| Developer Experience | 10 | Sec 8 (Phasing), Sec 9 (Tickets) | TICKET-014, 015 | `make dev`, `make demo`, `make demo-full`, `make report` all succeed |

---

*Mobile Check Deposit System PRD v1.1 — Apex Fintech Services — March 2026*

---

### Changelog

**v1.1 (2026-03-09) — Post-Review Updates**

| Change | Rationale |
|---|---|
| Fixed reversal entry count in Section 6.3 | PRD showed 3 entries (2 DEBITs + 1 CREDIT); corrected to 4 entries as 2 balanced DEBIT/CREDIT pairs matching Blueprint Q17 implementation |
| Clarified `Approved` state is always persisted (Section 7.8) | Pipeline was skipping `Approved` for auto-approved deposits, creating a phantom state with incomplete audit trail |
| Added `Completed → Returned` transition (Section 7.8) | Late returns after settlement are a real-world scenario; implementation already supported it but diagram omitted it |
| Added `settlement_batch_id` column and double-batching guard (Section 7.2) | No mechanism previously prevented a deposit from appearing in two settlement files |
| Consolidated `transfer_state_log` into `deposit_events` (Section 7.2) | Blueprint referenced both tables; clarified that `deposit_events` with `STATE_TRANSITION` event type serves both purposes |
| Added Component Interaction Diagram (Section 7.3) | Reviewers needed a visual showing the full HTTP request path through all services |
| Added State Transition → Side-Effect Mapping (Section 7.4) | No authoritative mapping existed for which side effects occur at each transition; critical for pipeline implementation |
| Updated TICKET-007 to require `Approved` state persistence | Acceptance criteria previously showed `Analyzing → FundsPosted` (skipping `Approved`) |
| Added double-batching acceptance criteria to TICKET-010 | Settlement guard needs explicit test coverage |
| Added `TestSettlement_NoDoubleBatching` and `TestReturnOnCompletedDeposit` to test manifest | Test count increased from 18 to 20 |
| Added `make demo-full` target and `jq` dependency check to TICKET-014 | Improves developer experience for reviewers |
| Added shift-left testing note to TICKET-013 | Unit tests for state machine, amount parsing, and cutoff logic should be written in Phase 1 |
