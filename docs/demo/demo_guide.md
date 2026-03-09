# Mobile Check Deposit System — Demo Guide

**Purpose:** This document defines the final demo structure, requirements, presenter script, and system walkthrough for the MCD project evaluation.

---

## Table of Contents

1. [Demo Overview](#1-demo-overview)
2. [Pre-Demo Requirements](#2-pre-demo-requirements)
3. [System Architecture Walkthrough](#3-system-architecture-walkthrough)
4. [Key Files & Components](#4-key-files--components)
5. [Demo Script (6 Acts)](#5-demo-script-6-acts)
6. [Rubric Alignment Checklist](#6-rubric-alignment-checklist)
7. [Presenter Talking Points](#7-presenter-talking-points)
8. [Troubleshooting](#8-troubleshooting)

---

## 1. Demo Overview

The demo tells a story in 6 acts: a check deposit that works perfectly, deposits that fail in every way the vendor can catch, business rules that block bad deposits, an operator who reviews edge cases, a bounced check that reverses correctly, and a final proof that every cent in the system balances.

**Duration:** ~10-15 minutes (narrated) or ~2 minutes (automated `make demo`)

**Delivery modes:**

| Mode | Command | Use Case |
|---|---|---|
| Fully automated (self-contained) | `make demo-full` | Starts server, runs all scenarios, stops server. Single command, zero setup. |
| Automated (server already running) | `make demo` | Runs all 6 acts against a running server at `localhost:8080`. |
| Manual walkthrough | Open `localhost:8080` in browser | Interactive demo using the web UI + curl commands from this guide. |
| Test suite as demo | `make test` | Runs 20+ automated tests with coverage report. |

**What the reviewer sees at the end:**
- Terminal output with colored pass/fail badges for every scenario
- A running web UI at `localhost:8080` with seeded data across all deposit states
- Reports in `/reports/` (test output, coverage, demo results)
- A live benchmark dashboard showing all KPIs at a glance

---

## 2. Pre-Demo Requirements

### 2.1 System Prerequisites

| Requirement | Version | Check Command |
|---|---|---|
| Go | 1.22+ | `go version` |
| Make | Any | `make --version` |
| jq | 1.6+ (for demo script) | `jq --version` |
| curl | Any (for demo script) | `curl --version` |
| Modern browser | Chrome/Firefox/Safari | For web UI walkthrough |

> **Note:** `jq` is only required for the automated demo script (`make demo`). The web UI demo works without it. The demo script checks for `jq` at startup and prints install instructions if missing.

### 2.2 One-Command Setup

```bash
git clone <repo-url>
cd mcd-system
make dev
```

This single command:
1. Creates `data/` and `reports/` directories
2. Copies `.env.example` to `.env` (if not present)
3. Builds the Go binary (`cmd/server/main.go`)
4. Runs SQLite migrations (creates tables, indexes)
5. Seeds the database with demo data (correspondents, investors, sample deposits)
6. Generates synthetic check images in `data/images/`
7. Starts the server and prints URLs:
   - `http://localhost:8080/` — Dashboard with benchmark cards
   - `http://localhost:8080/submit` — Deposit submission form
   - `http://localhost:8080/operator` — Operator review queue

### 2.3 Pre-Seeded Demo State

On first startup, the system seeds itself with a realistic universe:

| Entity | Count | Details |
|---|---|---|
| Correspondents | 3 | CORR-APEX ($5,000 limit), CORR-VANGUARD ($10,000 limit), CORR-FIDELITY ($3,000 limit) |
| Investors | 4-6 | Mapped to magic account prefixes (PASS-, BLUR-, MICR-, DUP-, MISMATCH-, GLARE-) |
| Pre-seeded deposits | 6+ | At least one deposit in each of: Completed, FundsPosted, Analyzing (flagged), Rejected, Returned |
| Ledger entries | Balanced | All seeded deposits have correct double-entry postings |
| Check images | Synthetic PNGs | Front/back images for every seeded deposit |

The reviewer opens `localhost:8080` and immediately sees a populated system, not a blank screen.

---

## 3. System Architecture Walkthrough

### 3.1 The 30-Second Pitch

> "This is a single Go binary that handles the entire mobile check deposit lifecycle — from the moment an investor photographs a check to the moment it settles with the bank. Every dollar that enters the system is tracked through a double-entry ledger where debits always equal credits. The vendor integration is stubbed with deterministic, self-documenting test scenarios. One command to run, 20+ tests to prove it works."

### 3.2 Architecture at a Glance

```
┌─────────────────────────────────────────────────────────────┐
│                     Go Single Binary                        │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │  Vendor   │  │ Funding  │  │ Operator │  │Settlement │  │
│  │   Stub    │  │ Service  │  │  Review  │  │  Engine   │  │
│  │          │  │          │  │          │  │           │  │
│  │ 7 scen-  │  │ Rules,   │  │ Queue,   │  │ X9 JSON,  │  │
│  │ arios,   │  │ limits,  │  │ approve/ │  │ EOD cut-  │  │
│  │ magic    │  │ ledger,  │  │ reject,  │  │ off, batch│  │
│  │ prefixes │  │ dupes    │  │ images   │  │ guard     │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬─────┘  │
│       │              │             │               │        │
│  ┌────┴──────────────┴─────────────┴───────────────┴────┐   │
│  │              Pipeline Orchestrator                    │   │
│  │     Step 1: Vendor → Step 2: Rules → Step 3: Ledger  │   │
│  └──────────────────────┬───────────────────────────────┘   │
│                         │                                   │
│  ┌──────────────────────┴───────────────────────────────┐   │
│  │               SQLite (data/mcd.db)                    │   │
│  │  transfers │ ledger_entries │ deposit_events           │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │          Embedded Web UI (Go embed)                   │   │
│  │  Dashboard │ Deposit Form │ Operator Queue │ Detail   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 3.3 How a Deposit Flows Through the System

```
Investor submits check
        │
        ▼
   [Requested] ── POST /api/v1/deposits
        │
        ▼
   [Validating] ── Vendor Stub checks image quality + MICR
        │                          │
        │ (clean pass)             │ (blur/glare/duplicate)
        ▼                          ▼
   [Analyzing] ── Rules engine    [Rejected] ── terminal
        │         checks limits,
        │         dupes, eligibility
        │                │
        │ (pass)         │ (over limit / duplicate)
        ▼                ▼
   [Approved] ──        [Rejected] ── terminal
        │
        ▼
   [FundsPosted] ── DEBIT omnibus / CREDIT investor (balanced pair)
        │                          │
        │ (settlement OK)          │ (check bounces)
        ▼                          ▼
   [Completed] ── terminal        [Returned] ── 4 reversal entries
                                               + $30 fee
```

### 3.4 The Financial Invariant

The single most important property of the system:

> **Sum of all DEBIT ledger entries == Sum of all CREDIT ledger entries. Always.**

This is enforced by:
- Every posting creates exactly 1 DEBIT + 1 CREDIT of equal amounts in a single `BEGIN IMMEDIATE` transaction
- Reversals create 2 new balanced pairs (reversal + fee), never mutate existing entries
- `TestLedgerInvariant_DebitsEqualCredits` proves this after all scenarios run
- The benchmark dashboard shows this invariant live

---

## 4. Key Files & Components

### 4.1 Entry Point & Wiring

| File | Purpose | Demo Relevance |
|---|---|---|
| `cmd/server/main.go` | Application entry point. Wires all services, runs migrations, seeds data, starts HTTP server. | Show how the entire system boots in one file. |
| `Makefile` | Build automation: `dev`, `test`, `demo`, `demo-full`, `report`, `clean` | Starting point for every demo interaction. |
| `.env.example` | Environment variables (port, DB path, log level) | Show zero-config setup. |

### 4.2 Domain Core

| File | Purpose | Demo Relevance |
|---|---|---|
| `internal/domain/transfer.go` | Transfer model, state enum (8 states), transition map | Show the state machine in ~15 lines of code. |
| `internal/domain/amount.go` | `Amount` type (int64 cents), `ToDollars()`, `ParseAmount()` | Show how floating-point errors are eliminated. |
| `internal/domain/errors.go` | Structured error codes (`VENDOR.IQA_BLUR`, `FUNDING.OVER_LIMIT`, etc.) | Show machine-readable errors that drive UI messages. |

### 4.3 Service Layer

| File | Purpose | Demo Relevance |
|---|---|---|
| `internal/vendor/stub.go` | 7-scenario vendor stub with magic prefix routing + header override | **15 rubric points.** Show how `BLUR-10001` deterministically triggers a blur failure. |
| `internal/funding/rules.go` | Business rules: $5,000 limit, duplicate detection (composite key, 30-day window), contribution type defaults | Show each rule with a failing test case. |
| `internal/funding/ledger.go` | Double-entry posting: `postToLedger()` creates balanced DEBIT/CREDIT pairs | **The financial heart.** Show the single function that guarantees correctness. |
| `internal/funding/pipeline.go` | Step function orchestrator: vendor → rules → ledger | Show how deposits flow through a 3-step pipeline. |
| `internal/operator/review.go` | Queue queries, approve/reject actions, audit logging | Show risk-sorted queue with operator attribution. |
| `internal/settlement/engine.go` | X9 JSON generator, EOD cutoff, batch guard (`settlement_batch_id`) | Show hierarchical settlement file with control totals. |
| `internal/returns/processor.go` | Return handling: 4 reversal entries in single transaction | Show atomic reversal with $30 fee. |

### 4.4 Data Layer

| File | Purpose | Demo Relevance |
|---|---|---|
| `internal/store/migrations.go` | Embedded forward-only migrations | Show self-migrating server (no external tools). |
| `internal/store/seed.go` | Programmatic seed data from domain logic | Show how the demo universe is created. |
| `config/correspondents.yaml` | 3 firms with different limits and contribution defaults | Show per-correspondent business rules. |
| `config/investors.yaml` | Test investors mapped to magic account prefixes | Show self-documenting test accounts. |

### 4.5 Web UI

| File | Purpose | Demo Relevance |
|---|---|---|
| `web/index.html` | Dashboard with live benchmark cards | **First thing the reviewer sees.** Show all KPIs at a glance. |
| `web/submit.html` | Deposit submission form | Demo submitting a deposit via browser. |
| `web/operator.html` | Review queue with images, risk scores, approve/reject | **10 rubric points.** Show the full operator workflow visually. |

### 4.6 Testing & Demo

| File | Purpose | Demo Relevance |
|---|---|---|
| `*_test.go` (various) | 20+ tests: integration + unit | `make test` runs everything with coverage. |
| `scripts/demo.sh` | Narrated 6-act scenario runner | `make demo` produces the pass/fail summary. |
| `reports/` | Generated test output, coverage, demo results | Submission artifacts. |

### 4.7 Documentation

| File | Purpose | Demo Relevance |
|---|---|---|
| `docs/architecture.md` | System diagram, data flow, service boundaries | Supporting material for architecture walkthrough. |
| `docs/decision_log.md` | 10 ADRs: language, data store, vendor stub, settlement, etc. | Show trade-off rationale (20 rubric points). |
| `SUBMISSION.md` | Summary, setup commands, eval results, risks/limitations | Required submission format. |

---

## 5. Demo Script (6 Acts)

The automated demo (`make demo` / `scripts/demo.sh`) runs all 6 acts sequentially. Below is the presenter-friendly version with talking points for each act.

---

### ACT 1: The Happy Path

**Story:** Alice deposits a $150.00 check. Everything works perfectly.

**What happens:**
1. `POST /api/v1/deposits` with account `PASS-10001`, amount 15000 (cents), check number 1001
2. Vendor stub sees `PASS-` prefix → returns `CLEAN_PASS` with MICR data, 0.98 confidence
3. Funding service validates: session OK, under $5,000 limit, no duplicate, assigns INDIVIDUAL contribution type
4. Risk score: 0 (LOW) → auto-approved (`Analyzing → Approved → FundsPosted`)
5. Ledger posted: DEBIT `OMNI-APEX-001` 15000 / CREDIT `PASS-10001` 15000
6. Settlement triggered: X9 JSON file generated with file control total = 15000
7. Final state: Completed

**Verification checks:**
- [ ] Transfer status progresses through all states
- [ ] Ledger shows balanced DEBIT/CREDIT pair
- [ ] Settlement file contains the deposit with correct amount
- [ ] Dashboard benchmark cards all green

**Talking point:** *"This is the golden path. Every deposit follows this exact pipeline — vendor validation, business rules, ledger posting, settlement. The difference is where it stops."*

---

### ACT 2: Vendor Rejection Scenarios

**Story:** Deposits fail at the vendor validation layer for different reasons.

**Scenarios:**

| Scenario | Account | Expected Response | Expected State |
|---|---|---|---|
| Blurry image | `BLUR-10001` | `VENDOR.IQA_BLUR` (422) | Rejected |
| Glare detected | `GLARE-10001` | `VENDOR.IQA_GLARE` (422) | Rejected |
| Duplicate check | `DUP-10001` | `VENDOR.DUPLICATE` (409) | Rejected |

**Verification checks:**
- [ ] Each scenario returns the correct structured error code
- [ ] Transfer moves to Rejected (terminal state)
- [ ] No ledger entries created for rejected deposits
- [ ] Error messages are human-readable and actionable

**Talking point:** *"The vendor stub is worth 15 points on the rubric. Each account prefix deterministically triggers a different failure. The tests are self-documenting — when you see `BLUR-10001`, you know exactly what will happen."*

---

### ACT 3: Business Rule Enforcement

**Story:** Deposits pass vendor validation but fail business rules.

**Scenarios:**

| Scenario | Setup | Expected Response | Expected State |
|---|---|---|---|
| Over deposit limit | $6,000 against CORR-APEX ($5,000 limit) | `FUNDING.OVER_LIMIT` (422) | Rejected |
| Duplicate deposit | Same check submitted twice within 30 days | `FUNDING.DUPLICATE` (409) | Rejected |
| Ineligible account | Suspended investor submits deposit | `FUNDING.INELIGIBLE` (403) | Rejected |

**Verification checks:**
- [ ] Over-limit rejection includes the correspondent's actual limit in the error message
- [ ] Duplicate detection uses the 4-field composite key (routing + account + check# + amount)
- [ ] No ledger entries created
- [ ] Rules are per-correspondent (different limits for different firms)

**Talking point:** *"This is dual-layer protection. The vendor catches image problems. The funding service catches business rule violations. A deposit has to pass both to get anywhere near the ledger."*

---

### ACT 4: Operator Review Workflow

**Story:** Deposits that need human judgment land in the operator queue.

**Scenarios:**

| Scenario | Account | Why Flagged | Operator Action |
|---|---|---|---|
| MICR read failure | `MICR-10001` | Confidence 0.42 (threshold 0.90) → risk score 65 (CRITICAL) | Approve with note |
| Amount mismatch | `MISMATCH-10001` | OCR amount differs from entered amount | Reject with reason |

**Web UI walkthrough:**
1. Open `localhost:8080/operator`
2. Show flagged deposits sorted by risk score (highest first)
3. Click into MICR failure deposit — show check images (front/back), MICR data, confidence scores, risk signal breakdown
4. Click Approve → enter note → submit
5. Show deposit moves to FundsPosted, ledger entries created
6. Click into amount mismatch deposit → show entered vs OCR amount discrepancy
7. Click Reject → enter reason → submit
8. Show deposit moves to Rejected, audit trail logged

**Verification checks:**
- [ ] Queue sorted by risk score descending
- [ ] Check images render in the browser
- [ ] Approve creates ledger entries + updates state
- [ ] Reject updates state without creating ledger entries
- [ ] Audit trail shows operator ID, timestamp, action, and notes
- [ ] Contribution type dropdown is functional

**Talking point:** *"The operator queue isn't just a list — it's a risk-prioritized triage tool. The 4-signal composite score (MICR confidence, amount delta, limit proximity, first-deposit flag) puts the riskiest deposits at the top. Every approve/reject is logged with who did it, when, and why."*

---

### ACT 5: Return/Reversal Path

**Story:** A previously settled check bounces. The system reverses the posting and deducts a $30 fee.

**Setup:** Use a deposit that reached FundsPosted or Completed in Act 1.

**What happens:**
1. `POST /api/v1/returns` with transfer_id and reason code (NSF)
2. System validates state (must be FundsPosted or Completed)
3. Single database transaction creates 4 ledger entries:
   - Reversal pair: DEBIT investor 15000 / CREDIT omnibus 15000
   - Fee pair: DEBIT investor 3000 / CREDIT omnibus 3000
4. Transfer transitions to Returned
5. Events logged: `RETURN_RECEIVED`, `REVERSAL_POSTED`, `INVESTOR_NOTIFIED`

**Verification checks:**
- [ ] Transfer moves to Returned state
- [ ] Exactly 4 new ledger entries created (2 balanced pairs)
- [ ] Fee is exactly $30.00 (3000 cents)
- [ ] Investor net debit: $180.00 (original $150.00 + $30.00 fee)
- [ ] Ledger still balances after reversal (debits == credits globally)
- [ ] Return on a Requested deposit returns `STATE.INVALID_TRANSITION` error

**Talking point:** *"This is where financial correctness gets tested hardest. The reversal creates new ledger entries — it never mutates existing ones. Two balanced pairs: one for the reversal, one for the fee. After this, the global ledger invariant still holds."*

---

### ACT 6: Invariant Verification

**Story:** After all scenarios, prove the system is mathematically correct.

**What happens:**
1. Query all ledger entries: `SELECT entry_type, SUM(amount) FROM ledger_entries GROUP BY entry_type`
2. Verify: total DEBITs == total CREDITs
3. Query settlement file: verify no rejected deposits are included
4. Generate a second settlement batch: verify zero new items (double-batching guard)
5. Query deposit counts by state: verify expected distribution

**Verification checks:**
- [ ] Global ledger invariant: sum(DEBIT) == sum(CREDIT)
- [ ] No rejected deposits in any settlement file
- [ ] Second settlement batch produces zero items (no double-batching)
- [ ] Deposit state counts match expected totals from Acts 1-5
- [ ] Benchmark dashboard reflects all of the above

**Talking point:** *"This is the closer. Every act before this showed that individual features work. This act proves the system is financially sound as a whole. The double-entry invariant holds across every scenario — happy path, rejections, operator actions, and reversals. The settlement file only contains what it should. And the double-batching guard ensures no deposit gets settled twice."*

---

### Demo Summary Output

The automated script produces a final summary:

```
═══════════════════════════════════════════════════
  RESULTS: 24 passed, 0 failed out of 24 checks
═══════════════════════════════════════════════════
  Report saved to: reports/demo_results.txt
```

---

## 6. Rubric Alignment Checklist

Use this checklist to verify the demo covers every rubric point before presenting.

| Category | Points | Demo Coverage | Act(s) |
|---|---|---|---|
| **System Design & Architecture** | 20 | Component diagram, data flow walkthrough, decision log (10 ADRs), state machine, project structure | Pre-demo walkthrough + docs |
| **Core Correctness** | 25 | Happy path E2E, business rules enforced, state transitions verified, ledger postings balanced | Acts 1, 3, 6 |
| **Vendor Stub Quality** | 15 | All 7 scenarios exercised (PASS, BLUR, GLARE, MICR, DUP, MISMATCH, CLEAN), deterministic via prefixes, header override | Acts 1, 2, 4 |
| **Operator Workflow & Observability** | 10 | Review queue with images/risk scores, approve/reject with audit logging, per-deposit decision trace, benchmark dashboard | Act 4 + web UI |
| **Return/Reversal Handling** | 10 | Bounced check reversed with $30 fee, 4 balanced ledger entries, state transition to Returned | Act 5 |
| **Tests & Evaluation Rigor** | 10 | 20+ tests, all paths exercised, coverage report in `/reports`, demo script as additional validation | `make test` + Act 6 |
| **Developer Experience** | 10 | `make dev` (one command), `make demo-full` (self-contained), README, demo script, decision log | Setup + `make help` |

**Total: 100 points addressable**

---

## 7. Presenter Talking Points

### Opening (30 seconds)

> "This is a mobile check deposit system built as a single Go binary. It handles the full lifecycle — image capture through bank settlement — with a double-entry ledger that guarantees every cent reconciles. Let me show you."

### Architecture (1-2 minutes)

> "The system is organized around 5 domain packages that map directly to the spec's service boundaries: vendor stub, funding service, operator review, settlement engine, and returns. They're all compiled into one binary — `make dev` and you're running."

**Show:** `cmd/server/main.go` (wiring), `internal/domain/transfer.go` (state machine), `Makefile` targets.

### Key Design Decisions (1 minute)

> "Three decisions shaped everything: integer cents for zero floating-point errors, double-entry ledger for provable financial correctness, and magic account prefixes for self-documenting tests. The decision log has 10 ADRs if you want the full trade-off analysis."

**Show:** `internal/domain/amount.go` (Amount type), `internal/funding/ledger.go` (postToLedger), `config/correspondents.yaml` (per-firm rules).

### Live Demo (5-8 minutes)

Run through Acts 1-6 using either the automated script or the web UI walkthrough.

### Closing (30 seconds)

> "20 tests pass. The ledger invariant holds. Every deposit is traceable from submission to settlement. The settlement file reconciles to the cent. And the whole thing runs with one command."

---

## 8. Troubleshooting

| Problem | Cause | Fix |
|---|---|---|
| `make dev` fails with "go: not found" | Go not installed or not in PATH | Install Go 1.22+ and add to PATH |
| `make demo` fails with "jq: command not found" | jq not installed | `brew install jq` (macOS) or `apt-get install jq` (Linux) |
| Port 8080 already in use | Another process on the port | `lsof -i :8080` then kill the process, or set `PORT=8081` in `.env` |
| "database is locked" error | Concurrent write attempt | Normal under high load with SQLite; retry. Should not occur during demo. |
| Demo script shows FAIL on status check | Processing not complete before check | Increase `sleep` duration in `demo.sh` or check server logs |
| Check images not rendering in operator UI | Images not generated during seed | Re-run `make dev` to regenerate seed data and images |
| Settlement file has 0 items | No deposits in FundsPosted state | Run Act 1 first to create a settled deposit |
| `make demo-full` hangs | Server didn't start in time | Check server logs in background; increase readiness wait timeout |

---

*Demo Guide v1.0 — Mobile Check Deposit System — Apex Fintech Services — March 2026*
