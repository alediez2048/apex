# Mobile Check Deposit — Complete Architectural Decision Blueprint

## Purpose

This document consolidates **30 critical architectural decisions** across three evaluation rounds required to build a high-reliability Mobile Check Deposit (MCD) system. Each decision is analyzed through three strategic lenses — from lean/monolithic to event-driven/distributed — and scored on **Financial Accuracy**, **System Complexity**, and **Operational Overhead**. The winning recommendation follows first principles: **zero gating errors and 100% settlement reconciliation.**

The decisions are organized in three layers:

- **Round 1 (Questions 1–10) — Foundations:** Language, data model, state machine, vendor stub, UI, settlement format, cutoff handling, omnibus lookup, duplicate detection, and test strategy.
- **Round 2 (Questions 11–20) — Implementation:** API design, error propagation, image handling, session validation, monetary precision, audit logging, return notifications, code structure, configuration management, and transaction safety.
- **Round 3 (Questions 21–30) — Operations & Polish:** Data seeding, demo narrative, test architecture, pipeline orchestration, observability, Makefile design, schema migrations, contribution types, risk scoring, and decision log structure.

---

# ROUND 1 — FOUNDATIONS

---

## Question 1: Golang or Java — and Why?

### Why This Matters

The language choice cascades into deployment strategy, dependency management, concurrency model, and how fast you can ship a working demo. The rubric rewards justified choices — not the choice itself.

### Strategy A: Go — Single-Binary Monolith

Build all services (Vendor Stub, Funding Service, Settlement Engine, Operator API) as packages within a single Go module, compiled into one binary.

**Pros:**
- Single binary deployment — `make build && ./mcd-server` — is the fastest path to one-command setup
- Go's `net/http` stdlib is production-grade with zero dependencies; no framework lock-in
- Goroutines give you lightweight concurrency for simulating async vendor callbacks without thread pool tuning
- Cross-compilation is trivial — reviewers on any OS can run the binary
- Compile times under 5 seconds keep the feedback loop tight

**Cons:**
- No mature ORM equivalent to Hibernate; you'll write more SQL by hand (or use `sqlc`/`GORM`)
- Go's error handling verbosity adds boilerplate, especially in multi-step validation chains
- Template-based HTML rendering is less ergonomic than Thymeleaf/JSP for the operator UI

### Strategy B: Java + Spring Boot

Use Spring Boot with Spring Data JPA for the Funding Service, a separate controller layer for the Vendor Stub, and Thymeleaf for the operator UI.

**Pros:**
- Spring Boot's auto-configuration and starter ecosystem mean the skeleton is up in minutes
- JPA/Hibernate gives you entity mapping, migration support, and transactional annotations out of the box
- Mature validation framework (`@Valid`, custom validators) maps cleanly to business rule enforcement
- Rich testing ecosystem (JUnit 5, Mockito, Spring Test) makes hitting 10+ tests straightforward

**Cons:**
- JVM startup time (2-5 seconds) and memory footprint (~200MB+) are noticeable in a demo context
- Spring's annotation magic can obscure control flow — a reviewer reading the code may struggle to trace a deposit through the system
- `docker compose` with a JVM service requires a multi-stage Dockerfile to keep image size reasonable
- Dependency tree is deep; a `mvn dependency:tree` on a Spring Boot app is 100+ transitive dependencies

### Strategy C: Hybrid — Go Services + Java Funding Engine

Run the Vendor Stub and Settlement Engine in Go (where raw performance and binary simplicity matter) and the Funding Service in Java/Spring (where business rule complexity benefits from JPA and validation frameworks).

**Pros:**
- Each component uses the language best suited to its concerns
- Demonstrates polyglot architecture thinking

**Cons:**
- Two build systems, two test harnesses, two deployment artifacts
- Dramatically increases reviewer cognitive load for a project scored on clarity
- One-command setup now requires orchestrating two runtimes — Docker Compose becomes mandatory, not optional
- Over-engineered for a system that will handle synthetic data at zero scale

### Winning Recommendation: **Strategy A — Go Single-Binary Monolith**

**Rationale:** The rubric allocates 10 points to developer experience (one-command setup, clear README) and 20 points to system design clarity. Go's single-binary model means `make dev` literally compiles and runs everything. The explicit control flow (no annotation magic) makes it easy for a reviewer to trace a deposit from submission to settlement. The concurrency model handles the async vendor stub callback pattern without framework overhead. For the data layer, `sqlc` or raw `database/sql` with SQLite gives you full SQL control — critical for double-entry ledger correctness where you need explicit transaction boundaries, not ORM magic.

**Justification statement for submission:** *"Go was chosen for single-binary deployment simplicity, explicit control flow that aids code review, and native concurrency for simulating async vendor interactions — all without framework overhead that would obscure the financial logic this project is evaluated on."*

---

## Question 2: Data Store and Ledger Modeling

### Why This Matters

The ledger is the financial heart of the system. A modeling error here means deposits post incorrectly, reversals don't balance, or settlement files contain wrong amounts. The rubric's "Core Correctness" category (25 pts) lives or dies on this decision.

### Strategy A: Single SQLite Database — Flat Transfer Table

One `transfers` table holds the deposit record with columns for amount, status, from/to accounts, and timestamps. Ledger "entries" are implied by the transfer record itself.

**Pros:**
- Simplest schema; fast to implement
- Easy to query transfer status

**Cons:**
- No double-entry guarantee — you can't independently verify that debits equal credits
- Reversals require updating the original row or inserting a second row with negative amounts, leading to ambiguous semantics
- Cannot answer "what is the current balance of account X?" without aggregating transfers and reversals with custom logic
- Fails the "settlement reconciliation" invariant because there's no independent ledger to reconcile against

### Strategy B: SQLite with Separate Transfers + Ledger Entries Tables (Double-Entry)

Two core tables: `transfers` (the business object tracking state) and `ledger_entries` (immutable financial records). Every financial action produces a balanced pair of ledger entries (debit + credit). Transfers own the lifecycle; ledger entries own the money.

**Schema outline:**

```
transfers
├── id (UUID)
├── investor_account_id
├── correspondent_id
├── amount
├── currency (USD)
├── status (state machine enum)
├── vendor_transaction_id
├── check_number
├── micr_data (JSON)
├── created_at / updated_at

ledger_entries
├── id (UUID)
├── transfer_id (FK → transfers)
├── account_id
├── entry_type (DEBIT | CREDIT)
├── amount
├── memo
├── posted_at
├── reversal_of (nullable FK → ledger_entries, for return handling)

transfer_state_log
├── id
├── transfer_id (FK)
├── from_state
├── to_state
├── actor (system | operator:<id>)
├── reason
├── timestamp
```

**Pros:**
- Double-entry invariant is enforceable: every posting inserts exactly one DEBIT and one CREDIT of equal amounts in a single transaction
- Balance queries are trivial: `SELECT SUM(CASE WHEN entry_type='CREDIT' THEN amount ELSE -amount END) FROM ledger_entries WHERE account_id = ?`
- Reversals create new ledger entries (not mutations), preserving full audit history
- The `transfer_state_log` gives you the audit trail the operator workflow rubric requires
- Settlement reconciliation becomes a join: compare `ledger_entries` where `transfer.status = 'Completed'` against settlement file contents

**Cons:**
- More tables, more insert logic, more things to get right
- SQLite's single-writer model means concurrent postings serialize (acceptable for MVP scale)
- Requires disciplined transaction boundaries — every posting must be wrapped in `BEGIN/COMMIT`

### Strategy C: Event-Sourced Ledger with Append-Only Event Log

All state changes are events (`DepositSubmitted`, `ValidationPassed`, `FundsPosted`, `CheckReturned`). Current state is derived by replaying events. The ledger is a projection built from the event stream.

**Pros:**
- Perfect audit trail by construction — every event is immutable
- Can rebuild any point-in-time state
- Natural fit for the state machine (events *are* transitions)

**Cons:**
- Massive over-engineering for an MVP with synthetic data
- Requires building a projection layer to answer simple queries ("what's the balance?")
- SQLite is not a natural event store; you'd fight the tooling
- Reviewers unfamiliar with event sourcing spend time understanding the pattern instead of evaluating your financial logic

### Winning Recommendation: **Strategy B — Double-Entry with Transfers + Ledger Entries**

**Rationale:** The spec explicitly requires "ledger postings" with specific attributes (To AccountId, From AccountId, Type: MOVEMENT, Memo: FREE). A double-entry model maps directly to these requirements. The immutable `ledger_entries` table makes reversals clean (new entries, not mutations), settlement reconciliation provable (join ledger entries against settlement file), and balance queries trivial. The `transfer_state_log` table satisfies the operator audit trail requirement without additional infrastructure. Event sourcing is intellectually appealing but adds complexity that the rubric doesn't reward.

**Key invariant to enforce in code:**
```
func postToLedger(tx *sql.Tx, transferID, debitAcct, creditAcct string, amount int64, memo string) error {
    // Both inserts in same DB transaction — either both succeed or neither does
    // Amount stored as cents (int64) to avoid floating-point errors
    _, err := tx.Exec("INSERT INTO ledger_entries (transfer_id, account_id, entry_type, amount, memo) VALUES (?, ?, 'DEBIT', ?, ?)", transferID, debitAcct, amount, memo)
    _, err = tx.Exec("INSERT INTO ledger_entries (transfer_id, account_id, entry_type, amount, memo) VALUES (?, ?, 'CREDIT', ?, ?)", transferID, creditAcct, amount, memo)
    return err
}
```

---

## Question 3: Vendor Service Stub — Response Selection Mechanism

### Why This Matters

The Vendor Stub is worth 15 rubric points. It must be "configurable," "deterministic," and support 7+ response types. The selection mechanism determines how easy it is to demo, test, and review.

### Strategy A: Magic Account Numbers

Reserve account number prefixes that trigger specific responses. For example:

| Account Number Prefix | Vendor Response |
|---|---|
| `PASS-*` | Clean Pass |
| `BLUR-*` | IQA Fail — Blur |
| `GLARE-*` | IQA Fail — Glare |
| `MICR-*` | MICR Read Failure |
| `DUP-*` | Duplicate Detected |
| `MISMATCH-*` | Amount Mismatch |
| `DEFAULT` | IQA Pass (proceed to MICR/OCR) |

**Pros:**
- Zero configuration files — behavior is encoded in the request itself
- Tests are self-documenting: `submitDeposit("BLUR-12345", 500)` makes the expected outcome obvious
- Demo scripts read like a narrative
- No hidden state between test runs

**Cons:**
- Account numbers in a real system are numeric; alpha prefixes break that convention
- Adding new scenarios requires code changes to the prefix map
- Can't test the same account number with different outcomes (e.g., first submission passes, second is flagged as duplicate)

### Strategy B: Request Header Override

Pass an `X-Vendor-Scenario` header with the desired response type. The stub reads the header and returns the corresponding response.

**Pros:**
- Clean separation — account numbers stay realistic, behavior is controlled externally
- Easy to add new scenarios (just add a new header value and response)
- Tests can use the same account number with different outcomes

**Cons:**
- Headers are invisible in a UI demo — a reviewer watching the operator screen doesn't see why one deposit failed
- Requires the Funding Service to pass through the header, coupling test infrastructure to production code paths
- Demo scripts need curl flags that clutter the walkthrough

### Strategy C: Configuration File with Scenario Sequences

A YAML/JSON config file defines a queue of responses. Each successive call to the stub returns the next response in the sequence.

**Pros:**
- Can choreograph complex demo flows (pass, pass, fail, duplicate, pass)
- No code changes for new scenarios

**Cons:**
- Stateful stub — response depends on call order, not request content
- Tests become order-dependent and brittle
- Parallel test execution is impossible
- A reviewer can't look at a single test and know what the stub will return

### Winning Recommendation: **Strategy A — Magic Account Numbers, with Strategy B as Override**

**Rationale:** Magic account numbers make tests self-documenting and demos self-explanatory. When a reviewer sees `submitDeposit("BLUR-10001", ...)` in a test, they immediately know the expected behavior. For edge cases where you need the same account to produce different outcomes (e.g., duplicate detection on second submission), layer in the `X-Vendor-Scenario` header as an optional override — if present, it takes precedence over the account prefix. This gives you the best of both strategies without the brittleness of stateful sequencing.

**Implementation pattern:**
```
func (s *VendorStub) DetermineResponse(req DepositRequest) VendorResponse {
    // Priority 1: Explicit header override
    if scenario := req.Header("X-Vendor-Scenario"); scenario != "" {
        return s.responseForScenario(scenario)
    }
    // Priority 2: Account prefix mapping
    return s.responseForAccountPrefix(req.AccountNumber)
}
```

---

## Question 4: Operator UI — Web UI or CLI?

### Why This Matters

The Operator Review Workflow is 10 rubric points. The spec requires viewing check images, MICR data, risk scores, and approve/reject controls. The choice here determines whether you can actually *show* that functionality or just describe it.

### Strategy A: Pure CLI with Formatted Tables

A terminal-based interface using formatted ASCII tables, with commands like `review list`, `review approve <id>`, `review reject <id> --reason "..."`.

**Pros:**
- Fastest to build — no HTML, no CSS, no JavaScript
- All interactions are scriptable and reproducible
- Fits naturally into demo scripts

**Cons:**
- Cannot display check images in a terminal (only file paths)
- Review queue as ASCII table is hard to scan with more than a few entries
- Approve/reject workflow feels disconnected — the reviewer can't see what they're approving
- Scores poorly on "operator efficiency" rubric criteria

### Strategy B: Single-Page Web UI (Embedded in Go Binary)

A single HTML file served by the Go HTTP server, using vanilla JavaScript to call the REST API. Embedded in the binary via Go's `embed` package.

**Pros:**
- Check images render natively in `<img>` tags
- Side-by-side layout: deposit details on the left, check images on the right, action buttons below
- Still one-command setup — the HTML is compiled into the binary
- A reviewer opens `localhost:8080/operator` and sees everything
- Can use simple CSS grid for the review queue table with status badges
- No build step, no npm, no React — just HTML + fetch()

**Cons:**
- More code to write than a CLI
- No component framework means repetitive DOM manipulation
- Limited interactivity without a JS framework

### Strategy C: React SPA with Separate Dev Server

A full React frontend with component library, served separately from the Go API.

**Pros:**
- Best UI/UX — reusable components, client-side routing, state management
- Could use a design system for professional appearance

**Cons:**
- Requires `npm install`, a Node.js runtime, and a build step — violates single-binary simplicity
- `docker compose` now needs two services (API + frontend)
- Massively over-scoped for an MVP where the UI is not the primary evaluation target
- Build failures in the frontend pipeline become a demo risk

### Winning Recommendation: **Strategy B — Single-Page Web UI Embedded in Go Binary**

**Rationale:** The operator workflow requires *visual* review — check images, risk scores, MICR data side-by-side. A CLI cannot deliver this. A React SPA is overkill and adds deployment complexity. A single embedded HTML page with vanilla JS gives you the visual presentation the rubric requires while preserving the one-command setup. The Go `embed` package compiles static assets into the binary, so `make dev` still starts everything.

**Key pages to build:**

1. **Dashboard** (`/`) — summary counts by transfer status
2. **Submit Deposit** (`/submit`) — form simulating mobile capture
3. **Operator Queue** (`/operator`) — filterable table of flagged deposits with image preview and approve/reject buttons
4. **Transfer Detail** (`/transfers/:id`) — full lifecycle view with state history and ledger entries
5. **Ledger View** (`/ledger`) — account balances and recent postings

---

## Question 5: State Machine Representation and Enforcement

### Why This Matters

The transfer state machine is the backbone of correctness. An invalid transition (e.g., `Requested` → `Completed` skipping validation) means a deposit posts without checks — the most severe failure mode in the system. The spec defines 8 states with specific valid transitions.

### Strategy A: Hardcoded Transition Map

Define valid transitions as a `map[State][]State` in code. Every state change calls a `transition()` function that checks the map before updating.

```go
var validTransitions = map[State][]State{
    Requested:   {Validating},
    Validating:  {Analyzing, Rejected},
    Analyzing:   {Approved, Rejected},
    Approved:    {FundsPosted},
    FundsPosted: {Completed, Returned},
    // Rejected and Returned are terminal — no outgoing transitions
}
```

**Pros:**
- Entire state machine is visible in one place — ~15 lines of code
- Transition validation is a simple set membership check
- Easy to test: write one test per valid transition and one per invalid transition
- No external dependencies

**Cons:**
- Transition side effects (sending notifications, creating ledger entries) must be handled elsewhere
- No built-in visualization or documentation generation

### Strategy B: State Machine Library

Use a Go state machine library (e.g., `looplab/fsm`) that provides transition definitions, callbacks, and event handling.

**Pros:**
- Callbacks on enter/exit/transition let you co-locate side effects with transitions
- Some libraries generate DOT diagrams for documentation
- Guards (conditional transitions) are first-class

**Cons:**
- External dependency for a simple problem
- Library API adds indirection — reviewers must learn the library to understand the flow
- Most Go FSM libraries are lightly maintained

### Strategy C: Database-Driven Transition Table

Store valid transitions in a `state_transitions` database table. The application queries the table to determine if a transition is legal.

**Pros:**
- Transitions can be modified without code changes
- Natural audit trail integration

**Cons:**
- A database query on every state change adds latency and failure modes
- Transition rules are hidden in data, not visible in code
- Over-engineered for 8 states with fixed transitions

### Winning Recommendation: **Strategy A — Hardcoded Transition Map**

**Rationale:** With only 8 states and well-defined transitions, a hardcoded map is the clearest, most testable, and most reviewable approach. The entire state machine fits on one screen. Side effects (ledger postings, notifications) are triggered by the service layer after a successful transition — keeping the state machine pure and the side effects explicit. Every transition writes to the `transfer_state_log` table for the audit trail.

**Enforcement pattern:**
```go
func (sm *StateMachine) Transition(transfer *Transfer, to State, actor string, reason string) error {
    allowed := validTransitions[transfer.Status]
    if !contains(allowed, to) {
        return fmt.Errorf("invalid transition: %s → %s", transfer.Status, to)
    }
    old := transfer.Status
    transfer.Status = to
    // Log the transition for audit
    sm.logTransition(transfer.ID, old, to, actor, reason)
    return nil
}
```

---

## Question 6: X9 ICL Settlement File Format

### Why This Matters

The spec says "X9 ICL format or structured JSON equivalent." This is a judgment call about domain fidelity vs. implementation speed. The rubric scores settlement integrity (within Core Correctness, 25 pts) — the file must contain correct data, not necessarily be in binary X9.37 format.

### Strategy A: Simplified JSON Settlement File

A JSON file containing an array of settlement records with MICR data, amounts, and image references. No attempt to mirror X9 record hierarchy.

```json
{
  "settlement_date": "2026-03-09",
  "batch_id": "BATCH-001",
  "total_amount": 450000,
  "item_count": 3,
  "items": [
    {
      "transfer_id": "...",
      "micr_routing": "021000021",
      "micr_account": "123456789",
      "micr_check_number": "1001",
      "amount": 150000,
      "front_image_ref": "/images/check-1001-front.png",
      "back_image_ref": "/images/check-1001-back.png"
    }
  ]
}
```

**Pros:**
- Fast to implement — it's just JSON serialization
- Easy to validate in tests — deserialize and assert
- Readable by reviewers without X9 domain knowledge

**Cons:**
- Doesn't demonstrate understanding of real X9.37 structure
- Loses the record-hierarchy concept (file header → cash letter → bundle → check detail → image view) that defines how banks actually process these files

### Strategy B: Structured JSON Mirroring X9 Record Hierarchy

A JSON file that preserves the X9.37 record structure — file header, cash letter header, bundle header, check detail records, image view records — but uses JSON instead of binary encoding.

```json
{
  "file_header": {
    "standard_level": "03",
    "destination_routing": "021000021",
    "origin_routing": "061000052",
    "file_creation_date": "20260309",
    "file_creation_time": "1830"
  },
  "cash_letters": [
    {
      "header": {
        "collection_type": "01",
        "destination_routing": "021000021",
        "record_type": "10"
      },
      "bundles": [
        {
          "header": { "bundle_id": "001" },
          "checks": [
            {
              "detail": {
                "amount": 150000,
                "micr_routing": "021000021",
                "micr_on_us": "123456789/1001",
                "payor_bank_routing": "021000021",
                "item_sequence": "000000001"
              },
              "image_views": [
                { "side": "front", "ref": "/images/check-1001-front.png" },
                { "side": "back", "ref": "/images/check-1001-back.png" }
              ]
            }
          ]
        }
      ]
    }
  ],
  "file_control": {
    "total_amount": 150000,
    "item_count": 1,
    "bundle_count": 1
  }
}
```

**Pros:**
- Demonstrates real understanding of X9.37 structure and banking settlement workflows
- File control totals enable self-consistency validation (sum of check amounts must equal file total)
- Maps 1:1 to how a real implementation would generate binary X9 — showing the reviewer you could do it
- Reconciliation logic can verify bundle totals → cash letter totals → file totals

**Cons:**
- More code to build the nested structure
- Requires research into X9.37 record types (though the structure is well-documented)

### Strategy C: Actual Binary X9.37 File

Generate a real binary X9.37 file using EBCDIC encoding and fixed-width records per the ANSI X9.37 specification.

**Pros:**
- Maximum domain fidelity

**Cons:**
- Requires an X9 library or custom binary encoder
- Binary files are not human-readable — reviewers can't inspect the output without tooling
- EBCDIC encoding is an unnecessary complexity for an MVP
- Time investment is disproportionate to rubric reward

### Winning Recommendation: **Strategy B — Structured JSON Mirroring X9 Hierarchy**

**Rationale:** This is the sweet spot. The JSON is human-readable (reviewers can inspect it), but the structure demonstrates that you understand how X9.37 files actually work. The nested hierarchy (file → cash letter → bundle → check detail → image view) with control totals at each level gives you built-in reconciliation: a test can verify that the sum of check detail amounts equals the bundle total, which equals the cash letter total, which equals the file control total. This directly supports the "100% settlement reconciliation" success criterion.

---

## Question 7: EOD Cutoff and Next-Business-Day Rollover

### Why This Matters

The spec requires a 6:30 PM CT cutoff with rollover. Getting this wrong means deposits settle on the wrong day — a financial accuracy failure.

### Strategy A: On-Demand Endpoint with Cutoff Check

A `POST /settlement/generate` endpoint that, when called, checks the current time against 6:30 PM CT. If before cutoff, it batches all `FundsPosted` deposits for today. If after, it assigns them a next-business-day settlement date. No automated scheduler.

**Pros:**
- Simplest to implement — no cron, no background workers
- Demo scripts call the endpoint explicitly, making the flow visible
- Cutoff logic is testable by injecting a clock interface

**Cons:**
- Requires manual trigger — in production, someone must remember to call it
- No automatic "it's 6:30, generate the file" behavior

### Strategy B: Background Scheduler with Automatic Trigger

A goroutine running a ticker that checks every minute. At 6:30 PM CT, it automatically generates the settlement file for the day's approved deposits.

**Pros:**
- More realistic — mimics production behavior
- Demonstrates understanding of batch processing patterns

**Cons:**
- Time-dependent behavior is hard to demo (reviewer has to wait or change system clock)
- Adds concurrency complexity (mutex on settlement generation, handling race with late deposits)
- Testing requires time mocking or very long test timeouts

### Strategy C: Hybrid — On-Demand Endpoint + Injectable Clock

The settlement endpoint accepts an optional `?as_of=2026-03-09T18:30:00-06:00` parameter. Without it, uses real time. With it, uses the provided timestamp for cutoff evaluation. A scheduler *could* call this endpoint, but the demo uses explicit calls with controlled timestamps.

**Pros:**
- Fully testable — inject any timestamp to test cutoff behavior, weekend rollover, etc.
- Demo scripts show both pre-cutoff and post-cutoff behavior in the same run
- Production-ready pattern (injectable clock) without production-scale complexity
- Can test "next business day" logic by injecting Friday 7 PM → should roll to Monday

**Cons:**
- Slightly more code than Strategy A
- The `as_of` parameter must be clearly documented as test-only

### Winning Recommendation: **Strategy C — On-Demand Endpoint with Injectable Clock**

**Rationale:** Time-dependent behavior is notoriously hard to demo and test. An injectable clock lets your test suite exercise the cutoff boundary (6:29 PM → today, 6:31 PM → next business day) and weekend rollover (Friday 7 PM → Monday) deterministically. The demo script can show both scenarios in sequence without waiting for real time to pass. For "next business day" logic, a simple approach: skip Saturdays and Sundays (holidays are out of scope for MVP, but note it in the risks/limitations doc).

**Implementation pattern:**
```go
type Clock interface {
    Now() time.Time
}

type RealClock struct{}
func (RealClock) Now() time.Time { return time.Now() }

type FixedClock struct{ T time.Time }
func (c FixedClock) Now() time.Time { return c.T }

func (s *SettlementService) GenerateFile(clock Clock) (*SettlementFile, error) {
    cutoff := getCutoffTime(clock.Now()) // 6:30 PM CT on current date
    settlementDate := clock.Now()
    if clock.Now().After(cutoff) {
        settlementDate = nextBusinessDay(clock.Now())
    }
    // Batch all FundsPosted deposits with created_at <= cutoff
    ...
}
```

---

## Question 8: Omnibus Account Lookup and Correspondent Configuration

### Why This Matters

The spec requires that ledger postings use a "From AccountId" that is "the omnibus account for the investor's correspondent, looked up via client config." This is a data modeling question with implications for how realistic and extensible your demo is.

### Strategy A: Single Hardcoded Omnibus Account

One omnibus account ID (`OMNIBUS-001`) used for all deposits. No correspondent differentiation.

**Pros:**
- Simplest possible implementation
- Fewer moving parts in demo

**Cons:**
- Doesn't demonstrate understanding of the correspondent model
- The spec explicitly says "looked up via client config" — a single constant isn't a lookup
- Can't show multi-correspondent scenarios

### Strategy B: Static Configuration File with 3 Correspondents

A `config.json` or `correspondents.yaml` file seeded with 3 sample correspondents, each with their own omnibus account, deposit limits, and feature flags.

```yaml
correspondents:
  - id: "CORR-APEX"
    name: "Apex Securities"
    omnibus_account: "OMNI-APEX-001"
    deposit_limit: 500000  # $5,000.00 in cents
    retirement_contribution_default: "INDIVIDUAL"
  - id: "CORR-BETA"
    name: "Beta Investments"
    omnibus_account: "OMNI-BETA-001"
    deposit_limit: 1000000  # $10,000.00
    retirement_contribution_default: "EMPLOYER"
  - id: "CORR-GAMMA"
    name: "Gamma Financial"
    omnibus_account: "OMNI-GAMMA-001"
    deposit_limit: 250000  # $2,500.00
    retirement_contribution_default: "INDIVIDUAL"
```

**Pros:**
- Demonstrates the correspondent → omnibus account mapping the spec requires
- Per-correspondent deposit limits show the business rule engine is configurable, not hardcoded
- Three correspondents is enough to be realistic without being excessive
- Config file is loaded at startup — no database migration needed
- Reviewers can see the config and understand the data model immediately

**Cons:**
- Requires mapping investors to correspondents (an additional `investors` table or config)
- More seed data to create and maintain

### Strategy C: Database-Driven Correspondent Registry

Full `correspondents` and `accounts` tables in SQLite with foreign key relationships, migration scripts, and CRUD endpoints.

**Pros:**
- Most production-realistic
- Could add a correspondent management UI

**Cons:**
- CRUD for correspondents is not in scope — it's infrastructure, not feature
- Migration scripts add setup complexity
- No rubric points for correspondent management

### Winning Recommendation: **Strategy B — Static Configuration with 3 Correspondents**

**Rationale:** The spec says "looked up via client config" — a config file is the literal implementation of this requirement. Three correspondents with different deposit limits show that business rules are parameterized, not hardcoded. Keep the investor-to-correspondent mapping in a simple seed data file: 5-6 test investors spread across the 3 correspondents, with account numbers using the magic prefixes from Question 3. This creates a self-consistent test universe.

---

## Question 9: Dual-Layer Duplicate Detection Strategy

### Why This Matters

The spec explicitly calls for duplicate detection at two levels — the Vendor Service (image-level) and the Funding Service (business-rule-level). Getting this wrong means either false rejections (blocking legitimate re-deposits) or false passes (allowing the same check to be deposited twice). Both are gating errors.

### Strategy A: Single-Field Match (Check Number Only)

The Funding Service checks if a deposit with the same check number already exists in a non-terminal state.

**Pros:**
- Simple to implement — single column lookup

**Cons:**
- High false positive rate — different payors can have the same check number
- No protection against same check, different account scenarios
- Doesn't meet the "beyond Vendor Service check" requirement since it's trivially simple

### Strategy B: Composite Key Match with Time Window

Duplicate is defined as matching on **routing number + account number + check number + amount** within a **rolling 30-day window**. This is the Funding Service layer. The Vendor stub layer independently returns "duplicate detected" for its own configured scenarios.

```go
type DuplicateCheck struct {
    RoutingNumber string
    AccountNumber string
    CheckNumber   string
    Amount        int64
    WindowDays    int // default 30
}

func (f *FundingService) IsDuplicate(check DuplicateCheck) (bool, *Transfer, error) {
    cutoff := time.Now().AddDate(0, 0, -check.WindowDays)
    existing, err := f.repo.FindTransfer(
        check.RoutingNumber,
        check.AccountNumber,
        check.CheckNumber,
        check.Amount,
        cutoff,
        // Exclude terminal-rejected transfers (those were legitimately rejected)
        []State{Requested, Validating, Analyzing, Approved, FundsPosted, Completed},
    )
    return existing != nil, existing, err
}
```

**Pros:**
- Composite key dramatically reduces false positives
- Time window prevents stale matches from blocking legitimate deposits months later
- Excluding terminal-rejected deposits allows re-submission after legitimate rejections
- Clear separation from Vendor stub's duplicate detection (different mechanism, different layer)

**Cons:**
- Four-field composite match requires a multi-column index for performance (trivial in SQLite)
- Edge case: same check deposited for different amounts (partial deposit) — the amount match would miss this, but this is out of scope for MVP

### Strategy C: Fuzzy Matching with Similarity Scoring

Use amount tolerance (±$0.50), fuzzy check number matching, and confidence scoring to catch near-duplicates.

**Pros:**
- Catches edge cases like OCR misreads of check numbers

**Cons:**
- Introduces ambiguity — what confidence threshold triggers rejection vs. flagging?
- Significantly more complex to implement and test
- False positive rate becomes unpredictable
- Over-engineered for an MVP with synthetic data

### Winning Recommendation: **Strategy B — Composite Key Match with Time Window**

**Rationale:** The four-field composite key (routing + account + check number + amount) is the industry-standard approach for check duplicate detection. The 30-day rolling window is a common parameter in production systems. This approach is clearly different from the Vendor stub's image-level duplicate detection, satisfying the "beyond Vendor Service check" requirement. The implementation is testable: submit the same check twice, verify the second is rejected. Submit a check with the same number but different routing number, verify it passes.

---

## Question 10: Test and Demo Strategy

### Why This Matters

The rubric allocates 10 points to tests/evaluation rigor and 10 to developer experience (which includes demo scripts). The spec requires "minimum 10 tests" and "deterministic demo scripts that exercise all paths." How you structure these determines whether a reviewer can verify your system works in under 5 minutes.

### Strategy A: Separate Test Suite + Manual Demo

Standard `go test` suite with 10+ unit/integration tests. A separate `DEMO.md` file with curl commands the reviewer runs manually.

**Pros:**
- Clean separation between automated tests and manual exploration
- Tests run in CI with `make test`

**Cons:**
- Manual demo is error-prone — reviewers may skip steps or run commands out of order
- Duplication between test scenarios and demo scenarios
- No guarantee that the demo steps actually work (they're not automated)

### Strategy B: Unified Test/Demo Suite with Scenario Runner

A `demo.sh` script that calls the REST API in sequence, exercising all 7 vendor stub scenarios plus the return/reversal path. The script outputs a formatted report showing each step, the API response, and pass/fail status. The same scenarios are also covered by `go test` for CI.

**Recommended scenario script structure:**

```bash
#!/bin/bash
set -e

BASE_URL="http://localhost:8080/api"
PASS=0; FAIL=0

run_scenario() {
    local name=$1; local account=$2; local amount=$3; local expected_status=$4
    echo "═══ Scenario: $name ═══"
    # Submit deposit
    RESPONSE=$(curl -s -X POST "$BASE_URL/deposits" -d "{...}")
    TRANSFER_ID=$(echo $RESPONSE | jq -r '.transfer_id')
    # Wait for processing
    sleep 1
    # Check status
    STATUS=$(curl -s "$BASE_URL/transfers/$TRANSFER_ID" | jq -r '.status')
    if [ "$STATUS" = "$expected_status" ]; then
        echo "✓ PASS — Status: $STATUS (expected: $expected_status)"
        ((PASS++))
    else
        echo "✗ FAIL — Status: $STATUS (expected: $expected_status)"
        ((FAIL++))
    fi
}

# === VENDOR STUB SCENARIOS ===
run_scenario "Clean Pass"        "PASS-10001"     15000 "Analyzing"
run_scenario "IQA Fail - Blur"   "BLUR-10001"     15000 "Rejected"
run_scenario "IQA Fail - Glare"  "GLARE-10001"    15000 "Rejected"
run_scenario "MICR Failure"      "MICR-10001"     15000 "Analyzing"  # flagged for review
run_scenario "Duplicate Check"   "DUP-10001"      15000 "Rejected"
run_scenario "Amount Mismatch"   "MISMATCH-10001" 15000 "Analyzing"  # flagged for review

# === BUSINESS RULE SCENARIOS ===
run_scenario "Over Limit"        "PASS-10002"     600000 "Rejected"  # >$5000

# === OPERATOR WORKFLOW ===
echo "═══ Operator: Approve MICR Failure ═══"
curl -s -X POST "$BASE_URL/operator/approve/$MICR_TRANSFER_ID"

# === SETTLEMENT ===
echo "═══ Generate Settlement File ═══"
curl -s -X POST "$BASE_URL/settlement/generate?as_of=2026-03-09T18:00:00-06:00"

# === RETURN/REVERSAL ===
echo "═══ Simulate Check Return ═══"
curl -s -X POST "$BASE_URL/returns" -d '{"transfer_id": "...", "reason": "NSF"}'

echo "═══ RESULTS: $PASS passed, $FAIL failed ═══"
```

**Pros:**
- One command (`make demo`) shows the reviewer everything
- Output is a formatted report that can be pasted into the submission
- Scripts are deterministic — same results every run
- Serves as living documentation of all system paths

**Cons:**
- Shell scripts are fragile (though `set -e` and `jq` parsing help)
- Separate from Go test suite (some duplication)

### Strategy C: Integration Test Suite as Demo (Go Test with Verbose Output)

Write the demo scenarios as Go integration tests with verbose logging. Run with `go test -v -run TestDemo` to produce a human-readable walkthrough.

**Pros:**
- Single test framework for everything
- Compile-time guarantees (no shell parsing errors)
- Can use test assertions for precise validation

**Cons:**
- Go test output is less visually appealing than a formatted script
- Mixing demo narrative with test assertions can be messy
- Reviewers may not think to run `go test -v` to see the demo

### Winning Recommendation: **Strategy B — Unified Scenario Script + Parallel Go Test Suite**

**Rationale:** The demo script is the reviewer's first impression. A clean `make demo` that walks through all scenarios with formatted output is worth more than a test suite they have to interpret. But you also need the Go test suite for rigor — it catches regressions the shell script won't. Build both, ensure they cover the same scenarios, and structure the Makefile so the reviewer's experience is:

```
make dev        # starts the server
make test       # runs 10+ Go tests
make demo       # runs the scenario script with formatted output
make report     # generates test report artifact in /reports
```

---

---

# ROUND 2 — IMPLEMENTATION

---

## Question 11: REST API Resource Design and Endpoint Structure

### Why This Matters

The API is the contract between every component — mobile client simulation, vendor stub, funding service, operator UI, and settlement engine. A poorly designed API means the demo script is confusing, the tests are brittle, and the reviewer can't trace a deposit through the system by reading endpoint names alone.

### Strategy A: Action-Oriented Flat Endpoints

Endpoints named after actions rather than resources. Each operation gets its own path.

```
POST   /submit-deposit
POST   /validate-deposit
POST   /approve-deposit/{id}
POST   /reject-deposit/{id}
POST   /generate-settlement
POST   /simulate-return/{id}
GET    /get-transfer/{id}
GET    /get-review-queue
GET    /get-ledger
```

**Pros:**
- Every endpoint name tells you exactly what it does — no ambiguity
- Simple to document and demo

**Cons:**
- Not RESTful — verbs in URLs violate REST conventions, signaling weak API design to a reviewer
- No consistent pattern for CRUD operations — each endpoint is a one-off
- Hard to extend — adding "cancel deposit" means inventing another action name
- No clear resource hierarchy (deposits own images, transfers own ledger entries) — relationships are invisible

### Strategy B: Resource-Oriented REST with Sub-Resources

Resources map to domain objects. Actions are expressed through HTTP methods and nested resources. State transitions use a dedicated sub-resource.

```
# Core deposit lifecycle
POST   /api/v1/deposits                          # Submit new deposit
GET    /api/v1/deposits/{id}                      # Get deposit with full state
GET    /api/v1/deposits/{id}/images/{side}        # Get check image (front/back)
GET    /api/v1/deposits/{id}/history              # State transition audit log
GET    /api/v1/deposits                           # List with filters (?status=, ?account=, ?from=, &to=)

# Operator actions (sub-resource pattern)
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

**Pros:**
- RESTful conventions signal strong API design to evaluators
- Resource hierarchy makes relationships explicit (deposit → images, deposit → history)
- `/api/v1/` prefix enables versioning and separates API from UI routes
- Consistent patterns — `GET` reads, `POST` creates/triggers — predictable for the demo script
- Filters on list endpoints (`?status=Rejected&from=2026-03-01`) support the operator search requirement
- Sub-resources for operator actions keep the domain clear: the operator acts on queue items, not raw deposits

**Cons:**
- More endpoints to implement than a flat design
- Some actions (approve/reject) don't map perfectly to REST verbs — `POST .../approve` is a pragmatic compromise
- Requires thoughtful URL design up front

### Strategy C: GraphQL API

Single `/graphql` endpoint with typed queries and mutations for all operations.

**Pros:**
- Clients fetch exactly the data they need — efficient for the operator UI
- Strong typing and introspection — self-documenting
- Single endpoint simplifies CORS and routing

**Cons:**
- Massive overkill for an MVP with a single consumer (the embedded UI)
- Go's GraphQL libraries (`gqlgen`, `graphql-go`) add significant dependency weight
- Demo scripts with curl become ugly — raw GraphQL queries in shell are unreadable
- Reviewers unfamiliar with GraphQL spend time on the framework, not the financial logic
- No rubric points for API sophistication

### Winning Recommendation: **Strategy B — Resource-Oriented REST with Sub-Resources**

**Rationale:** The API is the reviewer's map of your system. When they see `POST /api/v1/deposits` → `GET /api/v1/deposits/{id}/history` → `POST /api/v1/operator/queue/{id}/approve` → `POST /api/v1/settlement/batches`, they can trace the full lifecycle from the endpoint names alone. The `/api/v1/` prefix cleanly separates API routes from the embedded UI routes (`/`, `/operator`, `/submit`). The sub-resource pattern for operator actions keeps the domain model clean — operators don't modify deposits directly, they act on queue items. Demo scripts become self-documenting: each curl command reads like a sentence.

---

## Question 12: Error Handling and Propagation Strategy

### Why This Matters

In a financial pipeline, an unhandled error doesn't just crash a request — it can leave a deposit in a phantom state (money posted but transfer stuck in "Analyzing"), create orphaned ledger entries, or generate a settlement file with missing items. Every error must either resolve cleanly or halt with a traceable reason.

### Strategy A: HTTP Status Codes Only

Return appropriate HTTP status codes (400, 404, 409, 422, 500) with a generic error message body. Let the caller interpret the status code.

**Pros:**
- Standard HTTP semantics — any client understands status codes
- Minimal implementation effort

**Cons:**
- A 400 from the funding service could mean "deposit over limit," "duplicate check," or "account not found" — the caller can't distinguish without parsing a message string
- No error codes for programmatic handling — the demo script can't branch on error type
- The operator UI can't display actionable messages ("retake photo — too blurry" vs. "check previously deposited")
- No correlation between the error and the deposit's state — did the state machine transition to Rejected, or is it stuck?

### Strategy B: Structured Error Responses with Domain Error Codes

Every error response includes an HTTP status code, a machine-readable error code, a human-readable message, and a reference to the affected resource. The error code is a namespaced string that uniquely identifies the failure mode.

```go
type APIError struct {
    Code       string `json:"code"`        // "VENDOR.IQA_BLUR", "FUNDING.OVER_LIMIT"
    Message    string `json:"message"`     // "Image too blurry — please retake"
    TransferID string `json:"transfer_id"` // affected deposit, if applicable
    Details    any    `json:"details"`     // additional context (optional)
}
```

Error code taxonomy:

```
VENDOR.IQA_BLUR            → 422  Image quality: blur detected
VENDOR.IQA_GLARE           → 422  Image quality: glare detected
VENDOR.MICR_FAILURE        → 422  MICR line unreadable (flags for review, not rejection)
VENDOR.DUPLICATE           → 409  Check previously deposited
VENDOR.AMOUNT_MISMATCH     → 422  OCR amount ≠ entered amount (flags for review)
FUNDING.OVER_LIMIT         → 422  Deposit exceeds $5,000 limit
FUNDING.DUPLICATE          → 409  Duplicate detected by business rules
FUNDING.ACCOUNT_NOT_FOUND  → 404  Account identifier not resolvable
FUNDING.INELIGIBLE         → 403  Account not eligible for check deposit
STATE.INVALID_TRANSITION   → 409  Requested state transition not allowed
SETTLEMENT.CUTOFF_PASSED   → 422  Past EOD cutoff, rolled to next business day
SYSTEM.INTERNAL            → 500  Unexpected error (logged with correlation ID)
```

**Pros:**
- Machine-readable codes let the demo script and tests assert on specific failure modes: `assert response.code == "VENDOR.IQA_BLUR"`
- Namespaced codes make it obvious which layer produced the error (vendor vs. funding vs. state machine)
- The operator UI can map codes to actionable messages and icons
- The transfer ID in every error lets you cross-reference the deposit's state
- Extensible — new error types just add a code, no structural change

**Cons:**
- Requires defining the full taxonomy up front
- Every handler must construct a structured error instead of returning a bare string
- Slightly more code than bare status codes

### Strategy C: Error Monad / Result Type Pattern

Use Go's type system to create a `Result[T]` type that carries either a success value or a structured error. All service methods return `Result[T]` instead of `(T, error)`.

**Pros:**
- Forces callers to handle errors — can't accidentally ignore a failure
- Chain-able operations with early return on error

**Cons:**
- Go doesn't have generics mature enough for a clean Result type (pre-1.21 generics are limited)
- Fights the language's idiom (`val, err := ...` is canonical Go)
- Reviewers will question why you're fighting the language conventions
- No benefit over Strategy B for API responses — the Result type is internal, the API still returns JSON

### Winning Recommendation: **Strategy B — Structured Error Responses with Domain Error Codes**

**Rationale:** The error code taxonomy is a direct map of the spec's requirements. Every failure mode listed in the spec (IQA blur, glare, MICR failure, duplicate, amount mismatch, over-limit) gets a unique code. This means tests can assert on exact error codes rather than parsing message strings, the operator UI can display contextual actions ("retake photo" vs. "escalate to supervisor"), and the per-deposit decision trace (observability requirement) can log the exact error code at each step. The `VENDOR.*` / `FUNDING.*` / `STATE.*` namespacing also documents which layer in the pipeline produced the error — directly supporting the "clear separation of concerns" code quality requirement.

---

## Question 13: Check Image Storage and Serving

### Why This Matters

The spec requires check images (front and back) to be viewable in the operator review queue, referenced in settlement files, and submitted during deposit capture. How you store and serve these images affects the operator UI, the settlement engine, and the demo experience.

### Strategy A: Base64 Encoded in Database

Store images as Base64-encoded text in a `check_images` table column alongside the transfer record.

**Pros:**
- No file system dependency — everything is in SQLite
- Image is always co-located with the transfer record
- Simple backup (copy one `.db` file)

**Cons:**
- Base64 inflates image size by ~33% — a 500KB image becomes 667KB in the database
- SQLite performance degrades with large BLOBs (queries touching the images table become slow)
- Cannot serve images directly via HTTP without decoding — adds CPU overhead per request
- The operator UI must decode Base64 in JavaScript or the server must transcode on every view
- Settlement file image references would contain megabytes of Base64, making the file unreadable

### Strategy B: File System Storage with Database References

Store images as files in a structured directory (`/data/images/{transfer_id}/front.png`, `/data/images/{transfer_id}/back.png`). The database stores the file path. The Go server serves images via a static file handler.

**Pros:**
- Images served directly by `http.FileServer` — zero processing overhead
- Operator UI uses standard `<img src="/api/v1/deposits/{id}/images/front">` tags
- Settlement file references point to file paths — clean and lightweight
- File system handles large files efficiently
- Synthetic test images (e.g., a generated PNG with "FRONT" / "BACK" text) are easy to seed

**Cons:**
- Two storage systems to manage (SQLite + file system)
- File cleanup on deposit deletion requires coordination
- If the images directory is deleted, database references become dangling

### Strategy C: Object Store (MinIO / S3-Compatible)

Run a local MinIO instance via Docker for S3-compatible object storage. Images uploaded to a bucket, served via presigned URLs.

**Pros:**
- Production-realistic pattern
- Presigned URLs offload serving from the application

**Cons:**
- Adds a Docker dependency just for image storage
- MinIO must be in `docker-compose.yml` — increases setup complexity
- Presigned URL generation adds code for a problem that doesn't exist at MVP scale
- A reviewer must wait for MinIO to start before the system works

### Winning Recommendation: **Strategy B — File System Storage with Database References**

**Rationale:** Check images are inherently file-like — they're uploaded as multipart form data, displayed as `<img>` tags, and referenced (not inlined) in settlement files. The file system is the natural storage layer. Go's `http.FileServer` serves them with zero custom code. For the demo, generate synthetic check images at startup — a simple PNG with the check number, amount, and "FRONT"/"BACK" label rendered in text. This gives the operator UI real images to display without requiring the reviewer to provide their own.

**Synthetic image generation pattern (for seed data):**
```go
func generateCheckImage(checkNumber, amount, side string) []byte {
    img := image.NewRGBA(image.Rect(0, 0, 600, 300))
    // Light gray background
    draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{245, 245, 245, 255}}, image.Point{}, draw.Src)
    // Render text: "CHECK #1001 | $150.00 | FRONT"
    addLabel(img, fmt.Sprintf("CHECK #%s | $%s | %s", checkNumber, amount, side))
    var buf bytes.Buffer
    png.Encode(&buf, img)
    return buf.Bytes()
}
```

---

## Question 14: Session Validation and Account Eligibility Checks

### Why This Matters

The spec requires the Funding Service to "validate investor session and account eligibility." This is the authentication/authorization gate — deposits from invalid sessions or ineligible accounts must be rejected before any financial logic runs. Getting this wrong means unauthorized deposits can enter the pipeline.

### Strategy A: No Authentication — Trust All Requests

Skip session validation entirely. Every request with a valid account number is accepted. Focus engineering effort on the financial logic.

**Pros:**
- Simplest possible implementation
- No auth code to write or debug
- Demo scripts don't need tokens

**Cons:**
- The spec explicitly requires "validate investor session" — skipping it is a rubric miss
- No way to demonstrate that the system rejects unauthorized requests
- Removes a testable invariant ("unauthenticated deposits are rejected")

### Strategy B: API Key per Investor with Static Token Map

Each test investor has a pre-assigned API key. Requests include the key in an `Authorization: Bearer <token>` header. The server validates against a static map loaded from the seed data config.

```yaml
investors:
  - id: "INV-001"
    name: "Alice Thompson"
    correspondent: "CORR-APEX"
    accounts: ["PASS-10001", "BLUR-10001"]
    api_key: "tok_alice_test_001"
    eligible: true
  - id: "INV-002"
    name: "Bob Martinez"
    correspondent: "CORR-BETA"
    accounts: ["MICR-10001", "DUP-10001"]
    api_key: "tok_bob_test_002"
    eligible: true
  - id: "INV-003"
    name: "Carol Chen"
    correspondent: "CORR-GAMMA"
    accounts: ["MISMATCH-10001"]
    api_key: "tok_carol_test_003"
    eligible: false  # Account suspended — deposits should be rejected
```

**Pros:**
- Satisfies the "validate investor session" requirement with minimal complexity
- Tests can include a "rejected for invalid session" scenario (missing/wrong token)
- Tests can include an "account ineligible" scenario (Carol's suspended account)
- Token-to-investor mapping establishes identity, enabling the audit trail to log *who* submitted each deposit
- API keys in the config file are visible to reviewers — no hidden auth logic
- Demo scripts include the header, making auth visible: `curl -H "Authorization: Bearer tok_alice_test_001"`

**Cons:**
- Not production-realistic (real systems use OAuth/JWT)
- Static tokens mean no expiry or refresh — acceptable for MVP
- One more header in every curl command

### Strategy C: JWT with HMAC Signing

Issue JWTs at a `/auth/token` endpoint. Each request validates the JWT signature, extracts the investor ID from claims, and checks account eligibility.

**Pros:**
- Production-realistic auth pattern
- JWT claims can carry investor metadata (correspondent, account list)
- Demonstrates understanding of token-based auth

**Cons:**
- Requires a token issuance endpoint, HMAC key management, and token validation middleware
- Demo scripts must first call `/auth/token` before any deposit submission — adds friction
- JWT libraries add dependencies
- Token expiry introduces timing issues in long-running demos
- The auth system isn't what's being evaluated — over-investment here steals time from financial logic

### Winning Recommendation: **Strategy B — Static API Key per Investor**

**Rationale:** The spec says "validate investor session and account eligibility" — it doesn't say "build OAuth." A static token map checks both boxes: the token validates identity (session), and the investor record's `eligible` flag validates account status. This gives you two additional test scenarios at minimal cost: (1) request with no/invalid token → 401, (2) request from an ineligible investor → 403. Both strengthen the "gating correctness" metric. The static token approach also threads through the entire demo — every curl command shows which investor is acting, making the demo narrative clear.

---

## Question 15: Monetary Amount Representation and Currency Safety

### Why This Matters

A floating-point rounding error in a financial system is not a bug — it's a settlement reconciliation failure. If the ledger says $150.00 but the settlement file says $149.99, the "100% settlement reconciliation" invariant is broken. This decision affects every layer: API input, database storage, ledger posting, fee calculation, and settlement file output.

### Strategy A: Float64 (Dollars and Cents as Decimal)

Store and compute amounts as `float64` values (e.g., `150.00`).

**Pros:**
- Human-readable in JSON responses and database queries
- No conversion needed for display

**Cons:**
- `0.1 + 0.2 = 0.30000000000000004` in IEEE 754 — this is a known class of bug in financial systems
- The $30 return fee subtraction: `$150.00 - $30.00 = $120.00` works, but `$149.99 - $30.00` might produce `$119.98999...`
- Settlement file totals that aggregate many deposits will accumulate rounding errors
- Comparison operators (`==`) are unreliable on floats — reconciliation checks become probabilistic
- Any reviewer with financial systems experience will flag this as a critical design flaw

### Strategy B: Integer Cents (Amount × 100)

Store all amounts as `int64` representing cents. `$150.00` is stored as `15000`. All arithmetic is integer arithmetic. Convert to dollars only at the API boundary (display/output).

```go
type Amount int64  // cents

func (a Amount) ToDollars() string {
    sign := ""
    v := int64(a)
    if v < 0 { sign = "-"; v = -v }
    return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func ParseAmount(dollars string) (Amount, error) {
    // Parse "150.00" → 15000
    // Reject more than 2 decimal places
    // Reject negative amounts for deposits
}

const ReturnFee Amount = 3000 // $30.00
```

**Pros:**
- Zero rounding errors — integer arithmetic is exact
- `15000 - 3000 = 12000` is always exactly `$120.00`
- Settlement file totals: `sum([]int64)` is exact regardless of item count
- Reconciliation checks use `==` on integers — deterministic
- Industry standard for payment systems (Stripe, Square, most banking cores use integer cents)
- Database storage is a simple `INTEGER` column — no precision configuration needed

**Cons:**
- Every API boundary needs conversion (cents ↔ dollars display string)
- Input validation must reject malformed amounts ("150.001" — fractional cents)
- Developers must remember the convention — a comment or type alias helps

### Strategy C: Decimal Library (shopspring/decimal)

Use a third-party decimal library that provides arbitrary-precision decimal arithmetic.

**Pros:**
- No rounding errors, no conversion needed — `decimal.NewFromFloat(150.00)` works
- Arithmetic methods handle precision automatically
- Can represent sub-cent precision if needed

**Cons:**
- External dependency for a problem that integer cents solves without a library
- Decimal types don't serialize to JSON natively — need custom marshaler
- SQLite has no native decimal type — stored as TEXT, which complicates aggregation queries
- Performance overhead (heap allocation per operation) — irrelevant at MVP scale but signals over-engineering
- A reviewer may question why a 10-item system needs arbitrary-precision arithmetic

### Winning Recommendation: **Strategy B — Integer Cents**

**Rationale:** Integer cents is the industry standard for payment systems, and for good reason: it eliminates floating-point errors by construction, makes reconciliation checks deterministic (`==` on integers), and requires no external dependencies. The `Amount` type alias documents the convention, the `ToDollars()` method handles display, and the `ParseAmount()` function validates input at the API boundary. The $30 return fee becomes `const ReturnFee Amount = 3000` — clear, exact, and impossible to round incorrectly. Every aggregation in the settlement file is a simple integer sum. This directly supports the "100% settlement reconciliation" requirement.

**Fee calculation example:**
```go
func calculateReversal(originalAmount Amount) (debitAmount Amount, feeAmount Amount) {
    feeAmount = ReturnFee           // 3000 ($30.00)
    debitAmount = originalAmount    // Full original amount debited
    // Net effect on investor: -originalAmount (reversal) - fee
    // Implemented as two ledger entries:
    //   1. DEBIT investor originalAmount (reversal)
    //   2. DEBIT investor feeAmount (fee)
    return debitAmount, feeAmount
}
```

---

## Question 16: Audit Logging and Per-Deposit Decision Trace Architecture

### Why This Matters

The spec requires two overlapping but distinct things: (1) an **audit log** of operator actions (who approved/rejected what, when) and (2) a **per-deposit decision trace** showing the full path through the system (inputs → vendor response → business rules → operator actions → settlement). The operator workflow rubric (10 pts) explicitly evaluates "audit trail complete; decision traces available."

### Strategy A: Unstructured Application Logs

Use Go's `log` or `slog` package to write decision information to stdout. Grep the logs to reconstruct a deposit's history.

**Pros:**
- Zero additional infrastructure — `log.Printf` is built-in
- Useful for debugging during development

**Cons:**
- Unstructured logs can't be queried programmatically — "show me the audit trail for deposit X" requires grep
- No way to display the decision trace in the operator UI
- Logs are ephemeral — restart the server, lose the history
- Cannot satisfy the "operator actions are logged with full audit trail" requirement if logs aren't persisted and queryable

### Strategy B: Dedicated Event Log Table with Structured Entries

A single `deposit_events` table that records every significant action on a deposit. Each event has a type, actor, timestamp, and JSON payload with context-specific data.

```sql
CREATE TABLE deposit_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    event_type  TEXT NOT NULL,  -- enum: see below
    actor       TEXT NOT NULL,  -- "system", "vendor_stub", "operator:jsmith"
    payload     TEXT,           -- JSON with event-specific data
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

Event types and their payloads:

```
DEPOSIT_SUBMITTED     → { account_id, amount, images: [front_ref, back_ref] }
VENDOR_VALIDATED      → { scenario, response_code, micr_data, confidence, iqa_score }
VENDOR_REJECTED       → { scenario, reason, error_code }
RULES_EVALUATED       → { checks_applied: ["limit", "duplicate", "eligibility"], results: {...} }
RULES_REJECTED        → { rule, reason, threshold, actual_value }
FLAGGED_FOR_REVIEW    → { reason, risk_indicators }
OPERATOR_APPROVED     → { operator_id, notes }
OPERATOR_REJECTED     → { operator_id, reason, notes }
LEDGER_POSTED         → { ledger_entry_ids: [debit_id, credit_id], amount }
SETTLEMENT_BATCHED    → { batch_id, settlement_date, sequence_number }
SETTLEMENT_CONFIRMED  → { batch_id, bank_reference }
RETURN_RECEIVED       → { reason_code, original_amount }
REVERSAL_POSTED       → { reversal_entry_ids, fee_amount, net_debit }
INVESTOR_NOTIFIED     → { notification_type, channel }
```

**Pros:**
- Single source of truth for everything that happened to a deposit
- Query by transfer_id to get the full decision trace: `SELECT * FROM deposit_events WHERE transfer_id = ? ORDER BY created_at`
- Query by actor to get the operator audit trail: `SELECT * FROM deposit_events WHERE actor LIKE 'operator:%'`
- The operator UI can render the event timeline as a vertical feed — each event is a card with timestamp, actor, and details
- JSON payloads are flexible — each event type carries exactly the data it needs
- Settlement reconciliation can verify that every `SETTLEMENT_BATCHED` event has a corresponding `LEDGER_POSTED` event
- The `GET /api/v1/deposits/{id}/history` endpoint is a direct query on this table

**Cons:**
- JSON payloads aren't schema-enforced at the database level (mitigated by Go struct validation before insert)
- High insert volume in a production system (not a concern at MVP scale)
- Slightly more code than unstructured logging — each service method must emit an event

### Strategy C: Separate Audit Log + Decision Trace Systems

Two independent subsystems: (1) an `operator_audit_log` table for compliance (who did what), and (2) a `decision_trace` table for debugging (system pathway). Different schemas, different access patterns.

**Pros:**
- Clean separation of concerns — compliance vs. debugging
- Audit log can have stricter access controls in production

**Cons:**
- Two tables recording overlapping information about the same deposits
- The operator UI must query both tables to show a complete picture
- Inconsistency risk — if one table records an event and the other doesn't, which is the source of truth?
- Double the code for event emission

### Winning Recommendation: **Strategy B — Unified Event Log Table**

**Rationale:** A single `deposit_events` table serves both the audit trail (filter by `actor LIKE 'operator:%'`) and the decision trace (filter by `transfer_id`). Every significant action is one INSERT with a typed event and a JSON payload. The operator UI renders this as a timeline. Tests assert on event sequences: "after submitting deposit X, the events should be `DEPOSIT_SUBMITTED → VENDOR_VALIDATED → RULES_EVALUATED → LEDGER_POSTED`." The demo script can dump the event log for a deposit to show the reviewer the complete decision path. One table, one query pattern, two requirements satisfied.

---

## Question 17: Return/Reversal Notification Pipeline

### Why This Matters

The spec requires that when a check bounces, the system (1) reverses the ledger posting, (2) deducts a $30 fee, (3) transitions to "Returned" state, and (4) notifies the investor. These four actions must happen atomically or not at all — a partial reversal (money debited but no notification) is worse than no reversal.

### Strategy A: Synchronous Inline Processing

The return endpoint does everything in a single HTTP request handler: validates the return, creates reversal ledger entries, deducts the fee, updates the transfer state, and creates the notification — all in one database transaction.

```go
func (h *Handler) ProcessReturn(w http.ResponseWriter, r *http.Request) {
    tx, _ := h.db.Begin()
    defer tx.Rollback()

    // 1. Validate return (transfer exists, is in FundsPosted/Completed state)
    // 2. Create reversal DEBIT on investor account (original amount)
    // 3. Create fee DEBIT on investor account ($30)
    // 4. Create corresponding CREDIT entries on omnibus account
    // 5. Transition transfer state to Returned
    // 6. Log all events
    // 7. Create notification record

    tx.Commit()
}
```

**Pros:**
- All-or-nothing semantics via database transaction — either everything succeeds or nothing does
- Simple to reason about — the entire flow is in one function
- Easy to test: call the endpoint, check that all side effects occurred
- No background workers, no queues, no eventual consistency

**Cons:**
- The notification is inside the transaction — if notification creation fails (e.g., template rendering error), the entire reversal rolls back
- In production, you'd want to decouple notification from financial operations
- Long-running transaction if notification involves external calls (not an issue with synthetic notifications)

### Strategy B: Two-Phase — Financial Transaction + Async Notification

The return endpoint handles the financial operations (reversal, fee, state transition) in a database transaction. The notification is created as a pending record. A background goroutine polls for pending notifications and "sends" them (logs them for MVP).

**Pros:**
- Financial operations are atomic and decoupled from notification delivery
- If notification fails, the reversal isn't rolled back
- More production-realistic pattern

**Cons:**
- Adds a background worker and a `notifications` table
- Potential for "reversal posted but notification never sent" if the worker crashes
- More complex to test — must wait for async processing
- Over-engineering for an MVP where notifications are synthetic

### Strategy C: Event-Driven with In-Process Event Bus

Publish a `CheckReturned` event to an in-process event bus. Subscribers handle reversal posting, fee deduction, state transition, and notification independently.

**Pros:**
- Decoupled handlers — each concern subscribes independently
- Easy to add new side effects (e.g., "also alert the correspondent") without modifying the return handler

**Cons:**
- In-process event bus has no durability — events lost on crash
- Subscriber ordering matters (state transition must happen after ledger posting) — adds coordination complexity
- Debugging requires tracing through the event bus, not reading a linear function
- Abstraction layer that obscures what should be a straightforward 20-line function

### Winning Recommendation: **Strategy A — Synchronous Inline Processing, with one adjustment**

**Rationale:** For an MVP with synthetic notifications, the synchronous approach is correct. The entire reversal — ledger entries, fee deduction, state transition, event logging, and notification record creation — should happen in a single database transaction. This guarantees the financial invariant: if the reversal is committed, all associated records exist. The adjustment: create the notification as a database record (in the `deposit_events` table as an `INVESTOR_NOTIFIED` event) rather than actually "sending" it. The notification record proves the system would notify the investor; a real implementation would read from this table and dispatch via email/push. This keeps the transaction boundary clean while satisfying the spec.

**Implementation pattern:**
```go
func (s *ReturnService) ProcessReturn(transferID string, reasonCode string) error {
    tx, _ := s.db.Begin()
    defer tx.Rollback()

    transfer, _ := s.repo.GetTransferForUpdate(tx, transferID)
    if transfer.Status != FundsPosted && transfer.Status != Completed {
        return NewAPIError("STATE.INVALID_TRANSITION", "Can only return FundsPosted/Completed deposits")
    }

    // Reversal: debit investor, credit omnibus (original amount)
    s.ledger.Post(tx, transferID, transfer.InvestorAccount, transfer.OmnibusAccount, transfer.Amount, "REVERSAL")
    // Fee: debit investor, credit omnibus ($30)
    s.ledger.Post(tx, transferID, transfer.InvestorAccount, transfer.OmnibusAccount, ReturnFee, "RETURN_FEE")
    // State transition
    s.stateMachine.Transition(tx, transfer, Returned, "system", reasonCode)
    // Notification record
    s.events.Log(tx, transferID, "INVESTOR_NOTIFIED", "system", map[string]any{
        "type": "check_returned", "reason": reasonCode, "fee": ReturnFee,
    })

    return tx.Commit()
}
```

---

## Question 18: Project Structure and Package Layout

### Why This Matters

The rubric allocates 20 points to system design, which includes "clear separation of concerns." A reviewer opening the repo should immediately understand where to find the vendor stub, the funding service, the state machine, and the settlement engine — without reading a line of code. Package layout is architecture made visible.

### Strategy A: Flat Package Structure

All Go files in the root package. Files named by concern: `vendor_stub.go`, `funding_service.go`, `state_machine.go`, etc.

```
/
├── main.go
├── vendor_stub.go
├── funding_service.go
├── state_machine.go
├── ledger.go
├── settlement.go
├── operator.go
├── handlers.go
├── models.go
├── db.go
└── ...
```

**Pros:**
- No import cycles — everything is in one package
- Simple to navigate for a small project

**Cons:**
- All types share one namespace — naming collisions as the codebase grows
- No enforced boundaries — any function can call any other function
- A reviewer can't tell from the directory listing where the service boundaries are
- Files become large as each concern grows

### Strategy B: Domain-Oriented Package Layout

Packages map to the service boundaries defined in the spec. Each package owns its types, logic, and handlers. A shared `domain` package defines the transfer model, state machine, and amount type.

```
/
├── cmd/
│   └── server/
│       └── main.go              # Wires everything, starts HTTP server
├── internal/
│   ├── domain/
│   │   ├── transfer.go          # Transfer model, states, Amount type
│   │   ├── ledger.go            # LedgerEntry model
│   │   ├── state_machine.go     # Transition map + enforcement
│   │   └── errors.go            # Domain error codes
│   ├── vendor/
│   │   ├── stub.go              # Vendor stub with scenario routing
│   │   ├── responses.go         # Response types for each scenario
│   │   └── vendor_test.go
│   ├── funding/
│   │   ├── service.go           # Business rules, duplicate detection
│   │   ├── ledger_posting.go    # Double-entry posting logic
│   │   ├── account_resolver.go  # Investor → correspondent → omnibus lookup
│   │   └── funding_test.go
│   ├── operator/
│   │   ├── queue.go             # Review queue queries
│   │   ├── actions.go           # Approve/reject with audit logging
│   │   └── operator_test.go
│   ├── settlement/
│   │   ├── generator.go         # X9 JSON file builder
│   │   ├── cutoff.go            # EOD logic with injectable clock
│   │   └── settlement_test.go
│   ├── returns/
│   │   ├── processor.go         # Return handling, reversal, fee
│   │   └── returns_test.go
│   ├── store/
│   │   ├── sqlite.go            # Database setup, migrations
│   │   ├── repository.go        # Data access methods
│   │   └── seed.go              # Test data seeding
│   └── api/
│       ├── router.go            # Route registration
│       ├── handlers.go          # HTTP handlers (thin, delegate to services)
│       ├── middleware.go         # Auth, logging, error formatting
│       └── api_test.go          # Integration tests
├── web/
│   ├── index.html               # Dashboard
│   ├── submit.html              # Deposit submission form
│   ├── operator.html            # Operator review queue
│   └── static/                  # CSS, JS
├── config/
│   ├── correspondents.yaml      # Correspondent/omnibus config
│   └── investors.yaml           # Test investor seed data
├── scripts/
│   └── demo.sh                  # Deterministic demo script
├── docs/
│   ├── architecture.md
│   └── decision_log.md
├── reports/                     # Generated test/demo reports
├── data/                        # Runtime data (SQLite DB, images)
├── .env.example
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

**Pros:**
- Directory structure *is* the architecture diagram — each package maps to a spec section
- `internal/` prevents external imports, enforcing service boundaries at the compiler level
- Tests co-located with code — `vendor/vendor_test.go` is clearly testing the vendor stub
- `cmd/server/main.go` is the single entry point — wiring is explicit, no hidden dependency injection
- `web/` contains all UI files — `go:embed web/*` bundles them into the binary
- `config/` separates data from code — the correspondent and investor configs are visible at the top level
- A reviewer navigating the repo understands the architecture before reading any code

**Cons:**
- More directories and files than a flat layout
- Import paths are longer (`internal/funding`, `internal/vendor`)
- Requires thoughtful interface design to avoid circular dependencies

### Strategy C: Hexagonal / Ports-and-Adapters Architecture

Core domain logic in `internal/core/` with ports (interfaces) and adapters (implementations). HTTP handlers, database repos, and vendor clients are all adapters that implement ports.

**Pros:**
- Maximum testability — swap any adapter with a mock
- Domain logic has zero dependencies on infrastructure

**Cons:**
- Interface-per-dependency creates a proliferation of small files and types
- For a 7-service system, the port/adapter abstraction adds a layer of indirection that makes code harder to follow
- Reviewers must understand the hexagonal pattern to navigate the code
- Over-engineered for an MVP where the "adapters" will never be swapped (there's only one database, one HTTP framework)

### Winning Recommendation: **Strategy B — Domain-Oriented Package Layout**

**Rationale:** This layout is a direct translation of the spec's service boundaries into code. A reviewer looking at the repo sees `internal/vendor/`, `internal/funding/`, `internal/operator/`, `internal/settlement/`, `internal/returns/` — these are the exact components listed in the "clear separation of concerns" code quality requirement. The `internal/domain/` package centralizes the transfer model, state machine, and amount type — the shared language of the system. The `cmd/server/main.go` wiring file shows how everything connects, serving as executable architecture documentation.

---

## Question 19: Configuration and Environment Management

### Why This Matters

The spec requires `.env.example` with required environment variables, a configurable vendor stub, and per-correspondent settings. How you structure configuration determines whether `make dev` actually works on the first try for a reviewer who just cloned the repo.

### Strategy A: Environment Variables Only

All configuration via environment variables. The `.env.example` lists every variable. The application reads them at startup via `os.Getenv`.

```bash
# .env.example
PORT=8080
DB_PATH=./data/mcd.db
IMAGE_DIR=./data/images
EOD_CUTOFF_HOUR=18
EOD_CUTOFF_MINUTE=30
EOD_TIMEZONE=America/Chicago
DEPOSIT_LIMIT_CENTS=500000
RETURN_FEE_CENTS=3000
LOG_LEVEL=info
```

**Pros:**
- 12-factor app compliant — all config from environment
- `.env.example` is self-documenting
- Docker Compose reads `.env` files natively

**Cons:**
- Flat key-value pairs can't express structured data (correspondent configs, investor lists)
- 20+ environment variables become unwieldy
- No validation at startup — a missing variable causes a runtime panic deep in the code
- Can't express the correspondent → omnibus account mapping in env vars without ugly naming (`CORR_APEX_OMNIBUS=OMNI-APEX-001`)

### Strategy B: Layered Configuration — Env Vars for Infrastructure, Config Files for Domain

Environment variables for infrastructure settings (port, database path, log level). YAML config files for domain data (correspondents, investors, vendor stub scenarios). Startup validates all required config is present and fails fast with actionable error messages.

```go
type Config struct {
    // From environment
    Port           int    `env:"PORT" default:"8080"`
    DBPath         string `env:"DB_PATH" default:"./data/mcd.db"`
    ImageDir       string `env:"IMAGE_DIR" default:"./data/images"`
    LogLevel       string `env:"LOG_LEVEL" default:"info"`
    EODTimezone    string `env:"EOD_TIMEZONE" default:"America/Chicago"`

    // From config files
    Correspondents []Correspondent  // loaded from config/correspondents.yaml
    Investors      []Investor       // loaded from config/investors.yaml
}

func LoadConfig() (*Config, error) {
    cfg := &Config{}
    // Load env vars with defaults
    if err := loadEnv(cfg); err != nil {
        return nil, fmt.Errorf("config error: %w", err)
    }
    // Load YAML configs
    if err := loadYAML("config/correspondents.yaml", &cfg.Correspondents); err != nil {
        return nil, fmt.Errorf("failed to load correspondents: %w", err)
    }
    // Validate cross-references (every investor references a valid correspondent)
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("config validation failed: %w", err)
    }
    return cfg, nil
}
```

**Pros:**
- Clean separation: infrastructure config (env vars) vs. domain config (YAML files)
- Structured data (correspondents, investors) is natural in YAML — no flat-key gymnastics
- Startup validation catches misconfigurations before the server accepts requests
- Defaults mean `make dev` works with zero configuration — just clone and run
- `.env.example` stays small and focused on infrastructure
- Config files double as documentation — a reviewer reads `correspondents.yaml` to understand the data model

**Cons:**
- Two config sources to document
- Config loading code is more than `os.Getenv` — ~50 lines of setup
- YAML parsing requires a dependency (`gopkg.in/yaml.v3`)

### Strategy C: Centralized Configuration Service

A configuration management system (e.g., Consul, Vault, or a custom config server) that serves configuration to the application at runtime.

**Pros:**
- Dynamic config updates without restart
- Centralized management for multi-service deployments

**Cons:**
- Requires running a separate service — adds to `docker-compose.yml`
- Massive overkill for a single-binary MVP
- Configuration is hidden behind a service call — reviewers can't see it in the repo
- Adds a startup dependency and failure mode

### Winning Recommendation: **Strategy B — Layered Configuration**

**Rationale:** Environment variables are right for infrastructure (port, database path) because they vary by deployment. YAML files are right for domain data (correspondents, investors, vendor scenarios) because they're structured, version-controlled, and serve as documentation. The key feature is startup validation — the server refuses to start if a correspondent references a non-existent omnibus account or an investor references a non-existent correspondent. This means `make dev` either works completely or fails immediately with a clear error message. No silent misconfiguration that surfaces as a mysterious 500 error during the demo.

---

## Question 20: Concurrency Safety and Transaction Isolation

### Why This Matters

Even in an MVP, concurrent requests can corrupt financial data. Two requests to approve the same flagged deposit could double-post the ledger entry. A return notification arriving while a settlement batch is being generated could include a deposit that's about to be reversed. SQLite's concurrency model (single writer) provides some protection, but the application layer must also be correct.

### Strategy A: No Concurrency Controls — Rely on SQLite's Serialization

SQLite allows only one writer at a time. Concurrent write requests queue at the database level. No application-level locking.

**Pros:**
- SQLite handles write serialization automatically
- Zero application code for concurrency

**Cons:**
- SQLite serializes writes but not read-then-write sequences — two requests can both read a deposit as "Analyzing," then both attempt to transition to "Approved," and the second write overwrites the first
- No protection against double-posting: read balance → compute → write can race between two requests
- "Busy" errors under concurrent writes require retry logic that doesn't exist
- The absence of concurrency controls signals to a reviewer that you haven't considered race conditions

### Strategy B: Database-Level Transaction Isolation with SELECT FOR UPDATE Semantics

Use SQLite's `BEGIN IMMEDIATE` transaction mode (which acquires a write lock at transaction start, not first write) for all financial operations. Combine with application-level checks inside the transaction.

```go
func (s *FundingService) ApproveAndPost(transferID string, actor string) error {
    // BEGIN IMMEDIATE acquires write lock immediately
    tx, _ := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
    defer tx.Rollback()

    // Read transfer inside transaction — guaranteed consistent
    transfer, err := s.repo.GetTransfer(tx, transferID)
    if err != nil {
        return err
    }

    // Validate state transition inside the write lock
    if err := s.stateMachine.Transition(transfer, Approved, actor, "operator approval"); err != nil {
        return err // Another request already transitioned this deposit
    }

    // Post to ledger inside the same transaction
    if err := s.ledger.Post(tx, transferID, transfer.InvestorAccount, transfer.OmnibusAccount, transfer.Amount, "DEPOSIT"); err != nil {
        return err
    }

    // Update transfer state
    if err := s.repo.UpdateTransferStatus(tx, transferID, Approved); err != nil {
        return err
    }

    // Log events
    s.events.Log(tx, transferID, "OPERATOR_APPROVED", actor, nil)
    s.events.Log(tx, transferID, "LEDGER_POSTED", "system", nil)

    return tx.Commit()
}
```

**Pros:**
- `BEGIN IMMEDIATE` prevents two transactions from reading stale state and both writing
- All financial operations (state check, ledger post, state update) happen atomically
- No application-level mutex needed — the database transaction is the lock
- Retryable: if a transaction fails due to `SQLITE_BUSY`, the caller can retry
- Pattern is clear and auditable — every financial operation is wrapped in a transaction

**Cons:**
- `BEGIN IMMEDIATE` serializes all write operations — throughput limited to one write at a time
- Requires discipline: every financial handler must use `BeginTx`, not raw queries
- SQLite-specific behavior (`SQLITE_BUSY` retries) that wouldn't translate directly to Postgres

### Strategy C: Application-Level Distributed Locking

Use in-memory mutexes (`sync.Mutex`) keyed by transfer ID to prevent concurrent operations on the same deposit.

```go
var transferLocks sync.Map

func withTransferLock(transferID string, fn func() error) error {
    lock, _ := transferLocks.LoadOrStore(transferID, &sync.Mutex{})
    mu := lock.(*sync.Mutex)
    mu.Lock()
    defer mu.Unlock()
    return fn()
}
```

**Pros:**
- Fine-grained locking — operations on different deposits don't block each other
- No database-level contention

**Cons:**
- Locks are in-memory — lost on server restart (acceptable for MVP but fragile)
- Does not protect against database-level inconsistencies — if the application crashes between two database writes, the lock was held but the transaction is partial
- Two coordination mechanisms (app-level lock + DB transaction) is redundant for SQLite's single-writer model
- Adds complexity without adding safety over Strategy B

### Winning Recommendation: **Strategy B — Database-Level Transaction Isolation**

**Rationale:** For a SQLite-backed MVP, `BEGIN IMMEDIATE` transactions are the correct concurrency primitive. They provide the exact guarantee you need: no two transactions can read-then-write the same deposit concurrently. Every financial operation — approval, ledger posting, reversal, settlement batching — wraps its read-check-write sequence in a single transaction. The performance limitation (serialized writes) is irrelevant at MVP scale. The pattern is idiomatic, reviewable, and directly translatable to `SELECT FOR UPDATE` in PostgreSQL if the system were to scale.

**Critical invariant this protects:**
```
// This sequence is atomic — no concurrent request can interleave:
1. Read transfer (status = Analyzing)      ← inside BEGIN IMMEDIATE
2. Validate transition (Analyzing → Approved)
3. Post ledger entries (DEBIT + CREDIT)
4. Update transfer status
5. Log audit events
6. COMMIT                                   ← all or nothing
```

---

---

# ROUND 3 — OPERATIONS & POLISH

---

## Question 21: Data Seeding Strategy — How Does the System Bootstrap Itself?

### Why This Matters

When a reviewer runs `make dev`, the system must be immediately usable — correspondents exist, investors have accounts, the vendor stub has check images to return, and the operator queue has something to review. A system that starts empty and requires manual setup before anything can be demoed is a failed first impression. The spec says "one-command setup" — that includes data.

### Strategy A: SQL Seed Script Run at Startup

A `seed.sql` file loaded by the application on first boot. Contains INSERT statements for correspondents, investors, accounts, and sample transfers in various states.

**Pros:**
- Straightforward — raw SQL is explicit about what gets created
- Reviewer can read `seed.sql` to understand the test universe
- Runs once on empty database (idempotent check: skip if tables have rows)

**Cons:**
- SQL seed files are brittle — column order changes break them
- No logic — can't conditionally seed based on environment or generate synthetic images
- Doesn't create check images on disk (only database records)
- Hard to keep in sync with the Go model structs as the schema evolves

### Strategy B: Programmatic Seed Function with Scenario-Based Data

A Go function called at startup that creates a complete test universe: correspondents, investors with accounts mapped to magic prefixes, sample deposits in every state (including one mid-review, one completed, one returned), synthetic check images on disk, and pre-populated ledger entries for the completed deposits.

```go
func SeedDemoData(db *sql.DB, imageDir string) error {
    // Skip if already seeded
    if count, _ := countTransfers(db); count > 0 {
        return nil
    }

    // 1. Create correspondents (from config, but verify in DB)
    // 2. Create investors with magic-prefix accounts
    investors := []SeedInvestor{
        {ID: "INV-001", Name: "Alice Thompson", Correspondent: "CORR-APEX",
         Accounts: []string{"PASS-10001", "BLUR-10001", "GLARE-10001"}, Token: "tok_alice_001"},
        {ID: "INV-002", Name: "Bob Martinez", Correspondent: "CORR-BETA",
         Accounts: []string{"MICR-10001", "DUP-10001"}, Token: "tok_bob_002"},
        {ID: "INV-003", Name: "Carol Chen", Correspondent: "CORR-GAMMA",
         Accounts: []string{"MISMATCH-10001", "PASS-10002"}, Token: "tok_carol_003"},
        {ID: "INV-004", Name: "Dave Wilson", Correspondent: "CORR-APEX",
         Accounts: []string{"PASS-10003"}, Token: "tok_dave_004", Eligible: false},
    }

    // 3. Create sample deposits in various states for operator queue demo
    sampleDeposits := []SeedDeposit{
        {Account: "PASS-10001", Amount: 15000, Status: Completed,
         Description: "Happy path — fully settled"},
        {Account: "MICR-10001", Amount: 25000, Status: Analyzing,
         Description: "MICR failure — waiting in operator queue"},
        {Account: "MISMATCH-10001", Amount: 30000, Status: Analyzing,
         Description: "Amount mismatch — waiting in operator queue"},
        {Account: "PASS-10002", Amount: 10000, Status: Returned,
         Description: "Returned check — reversal posted with fee"},
    }

    // 4. Generate synthetic check images for each deposit
    // 5. Create ledger entries for completed/returned deposits
    // 6. Create deposit_events for full audit trail
    return nil
}
```

**Pros:**
- Reviewer runs `make dev`, opens `localhost:8080`, and immediately sees a populated dashboard with deposits in different states
- The operator queue already has flagged items — no need to submit deposits before demoing the review workflow
- Ledger entries exist for completed deposits — the balance view shows real numbers
- A returned deposit with reversal and fee is pre-seeded — the reviewer can inspect the audit trail without running the return flow manually
- Synthetic check images exist on disk — the operator UI displays them immediately
- Seed data is self-documenting: each deposit has a `Description` field explaining why it's there
- Idempotent — safe to restart the server without duplicating data

**Cons:**
- More code than a SQL file (~100-150 lines)
- Must be maintained alongside schema changes
- Seed data could mask bugs if it bypasses validation logic (mitigated by running seed through the same service layer)

### Strategy C: External Seed Data Archive (ZIP/Tar)

A pre-built archive containing a SQLite database file and check images. The startup script extracts the archive into `data/` if the directory is empty.

**Pros:**
- Instant startup — no seed logic to run
- Database is pre-built with exact data the demo expects

**Cons:**
- Binary database file in the repo — can't be diffed, reviewed, or version-controlled meaningfully
- If the schema changes, the archive must be rebuilt manually
- Reviewer can't see what's in the database without running the app
- Opaque — the opposite of the transparency the rubric rewards

### Winning Recommendation: **Strategy B — Programmatic Seed Function**

**Rationale:** The seed function is the demo's foundation. It creates a complete universe that makes every feature immediately demonstrable: the dashboard shows counts across all states, the operator queue has items waiting, the ledger has balances, and the audit trail has events. Running the seed through the service layer (not raw SQL) ensures the data is internally consistent — ledger entries balance, state transitions are logged, and events exist. The seed function also serves as documentation: a reviewer reading it understands every test scenario before running a single command.

---

## Question 22: Demo Script Narrative — How Does the Walkthrough Tell a Story?

### Why This Matters

The demo script is the reviewer's guided tour. A list of curl commands with no context is a chore to read. A scripted narrative that walks through scenarios with commentary, expected outcomes, and verification steps is the difference between a 7/10 and a 10/10 on developer experience.

### Strategy A: Raw Curl Commands in a Shell Script

A `demo.sh` file with sequential curl commands. Comments above each block explain the scenario.

```bash
# Submit a clean deposit
curl -X POST http://localhost:8080/api/v1/deposits ...
# Check status
curl http://localhost:8080/api/v1/deposits/...
```

**Pros:**
- Simple to write
- Directly executable

**Cons:**
- Reviewer must read comments to understand what's happening
- No visual feedback — raw JSON output scrolls by
- No pass/fail indication — reviewer must mentally verify each response
- If one command fails, the script continues with stale IDs

### Strategy B: Narrated Scenario Runner with Formatted Output

A `demo.sh` script structured as a sequence of named scenarios. Each scenario has a description, action, expected outcome, actual outcome, and pass/fail badge. The script captures transfer IDs from responses and threads them through subsequent steps. Output is formatted with section headers, colored status badges, and a final summary.

```bash
#!/bin/bash
set -euo pipefail

BASE="http://localhost:8080/api/v1"
PASS=0; FAIL=0; TOTAL=0

# ─── Utilities ───────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'

scenario() {
    local name="$1"; local description="$2"
    ((TOTAL++))
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}SCENARIO $TOTAL: $name${NC}"
    echo "  $description"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

check() {
    local label="$1"; local expected="$2"; local actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo -e "  ${GREEN}✓ PASS${NC} $label (got: $actual)"
        ((PASS++))
    else
        echo -e "  ${RED}✗ FAIL${NC} $label (expected: $expected, got: $actual)"
        ((FAIL++))
    fi
}

# ─── ACT 1: THE HAPPY PATH ──────────────────────────────────

scenario "Happy Path — Clean Check Deposit" \
    "Alice submits a \$150.00 check. Vendor validates clean. Rules pass. Operator reviews. Settlement generated."

echo "  → Submitting deposit..."
RESP=$(curl -s -X POST "$BASE/deposits" \
    -H "Authorization: Bearer tok_alice_001" \
    -H "Content-Type: application/json" \
    -d '{"account_id":"PASS-10001","amount":15000,"check_number":"1001"}')
TID=$(echo "$RESP" | jq -r '.transfer_id')
echo "  → Transfer ID: $TID"

sleep 1
STATUS=$(curl -s "$BASE/deposits/$TID" | jq -r '.status')
check "Status after validation" "Analyzing" "$STATUS"

echo "  → Operator approving deposit..."
curl -s -X POST "$BASE/operator/queue/$TID/approve" \
    -H "Authorization: Bearer tok_operator_001" > /dev/null
STATUS=$(curl -s "$BASE/deposits/$TID" | jq -r '.status')
check "Status after approval" "FundsPosted" "$STATUS"

echo "  → Generating settlement file (pre-cutoff)..."
BATCH=$(curl -s -X POST "$BASE/settlement/batches?as_of=2026-03-09T18:00:00-06:00")
ITEM_COUNT=$(echo "$BATCH" | jq '.file_control.item_count')
check "Settlement includes deposit" "1" "$ITEM_COUNT"

# ─── ACT 2: VENDOR REJECTION SCENARIOS ──────────────────────

scenario "IQA Failure — Blur" \
    "Alice submits a blurry check image. Vendor rejects with IQA_BLUR. Deposit moves to Rejected."
# ...

scenario "IQA Failure — Glare" \
    "Alice submits a glare-affected image. Vendor rejects with IQA_GLARE."
# ...

# ─── ACT 3: BUSINESS RULE ENFORCEMENT ───────────────────────

scenario "Over Deposit Limit" \
    "Carol tries to deposit \$6,000.00 (limit is \$5,000). Funding Service rejects."
# ...

# ─── ACT 4: OPERATOR REVIEW WORKFLOW ────────────────────────

scenario "MICR Failure → Operator Approval" \
    "Bob submits a check with unreadable MICR. Flagged for review. Operator approves."
# ...

scenario "Amount Mismatch → Operator Rejection" \
    "Carol submits with mismatched amount. Operator rejects after review."
# ...

# ─── ACT 5: THE RETURN PATH ─────────────────────────────────

scenario "Check Return and Reversal" \
    "A previously settled check bounces. System reverses posting, deducts \$30 fee."
# ...

# ─── ACT 6: INVARIANT VERIFICATION ──────────────────────────

scenario "Ledger Balance Reconciliation" \
    "Verify all DEBIT entries sum equals all CREDIT entries sum (double-entry invariant)."
# ...

# ─── SUMMARY ─────────────────────────────────────────────────
echo ""
echo "═══════════════════════════════════════════════════"
echo -e "  RESULTS: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC} out of $TOTAL checks"
echo "═══════════════════════════════════════════════════"
```

**Pros:**
- Reads like a screenplay — ACT 1 (happy path), ACT 2 (rejections), ACT 3 (rules), ACT 4 (operator), ACT 5 (returns), ACT 6 (invariants)
- Each scenario states *what's happening and why* before showing the commands
- Pass/fail badges give instant visual feedback
- Transfer IDs thread through — operator approves the exact deposit that was submitted
- Final summary is a one-line scorecard for the submission
- The invariant verification act proves the system is financially correct, not just functionally working
- Output can be piped to a file for the `/reports` deliverable

**Cons:**
- More scripting effort (~200 lines of bash)
- Depends on `jq` for JSON parsing (standard on most systems, but worth noting in README)
- Timing-sensitive — `sleep 1` between submission and status check assumes processing completes in time

### Strategy C: Interactive TUI Demo

A terminal-based interactive demo with menus: "Press 1 for Happy Path, Press 2 for Blur Failure, Press 3 for Operator Review..."

**Pros:**
- Reviewer chooses what to explore
- Replayable — run any scenario multiple times

**Cons:**
- Can't be automated — reviewers must click through each option
- Not scriptable for CI or report generation
- More code for the TUI framework than the actual demo logic
- Non-deterministic — different reviewers see different things

### Winning Recommendation: **Strategy B — Narrated Scenario Runner**

**Rationale:** The demo script is a performance piece. The "act" structure walks the reviewer through a complete narrative arc: everything works (Act 1), things fail gracefully (Acts 2-3), humans intervene (Act 4), money gets returned (Act 5), and the math checks out (Act 6). The pass/fail badges mean the reviewer doesn't need to interpret raw JSON — green checkmarks tell the story. The invariant verification act (Act 6) is the closer: it proves the system isn't just functional but financially sound. Pipe the output to `/reports/demo_results.txt` for the submission.

---

## Question 23: Test Architecture — Unit Tests vs. Integration Tests vs. Both

### Why This Matters

The spec requires "minimum 10 tests" covering happy paths, vendor scenarios, business rules, state machine transitions, reversals, and settlement contents. The rubric allocates 10 points to "tests and evaluation rigor — all paths exercised; test report generated." How you structure these tests determines whether they prove correctness or just prove the code runs.

### Strategy A: Unit Tests Only

Test each package in isolation. Mock all dependencies (database, vendor stub, other services). Focus on input/output behavior of individual functions.

```go
func TestStateMachine_ValidTransition(t *testing.T) {
    sm := NewStateMachine()
    transfer := &Transfer{Status: Analyzing}
    err := sm.Transition(transfer, Approved, "operator:1", "approved")
    assert.NoError(t, err)
    assert.Equal(t, Approved, transfer.Status)
}

func TestStateMachine_InvalidTransition(t *testing.T) {
    sm := NewStateMachine()
    transfer := &Transfer{Status: Requested}
    err := sm.Transition(transfer, Completed, "system", "skip")
    assert.Error(t, err)  // Can't skip from Requested to Completed
}
```

**Pros:**
- Fast — no database setup, no HTTP server
- Isolated — failures point to the exact function that broke
- Easy to write 10+ tests quickly

**Cons:**
- Mocks can diverge from real behavior — a test passes but the system is broken
- Can't verify cross-service invariants (e.g., "a deposit that passes validation also passes business rules and gets a ledger entry")
- Doesn't test the actual HTTP API or JSON serialization
- A test that mocks the database can't verify transaction atomicity

### Strategy B: Integration Test Suite with Real Database + Targeted Unit Tests

Integration tests spin up a real SQLite database (in-memory for speed), create the full service stack, and exercise scenarios through the HTTP API. Unit tests cover pure logic (state machine, amount parsing, cutoff calculation) where integration testing is unnecessary.

```go
// Integration test: full happy path
func TestIntegration_HappyPath(t *testing.T) {
    app := setupTestApp(t)  // In-memory SQLite, real services, HTTP test server
    defer app.Cleanup()

    // Submit deposit
    resp := app.POST("/api/v1/deposits", DepositRequest{
        AccountID: "PASS-10001", Amount: 15000, CheckNumber: "5001",
    }, "tok_alice_001")
    assert.Equal(t, 201, resp.StatusCode)
    transferID := resp.JSON()["transfer_id"].(string)

    // Wait for async processing
    app.WaitForStatus(t, transferID, "Analyzing", 2*time.Second)

    // Approve
    resp = app.POST("/api/v1/operator/queue/"+transferID+"/approve", nil, "tok_operator_001")
    assert.Equal(t, 200, resp.StatusCode)

    // Verify ledger entries
    entries := app.GET("/api/v1/accounts/PASS-10001/ledger").JSON()
    assert.Equal(t, 1, len(entries["entries"].([]any)))

    // Verify deposit events
    history := app.GET("/api/v1/deposits/"+transferID+"/history").JSON()
    eventTypes := extractEventTypes(history)
    assert.Equal(t, []string{
        "DEPOSIT_SUBMITTED", "VENDOR_VALIDATED", "RULES_EVALUATED",
        "OPERATOR_APPROVED", "LEDGER_POSTED",
    }, eventTypes)
}

// Unit test: pure state machine logic
func TestStateMachine_AllValidTransitions(t *testing.T) { ... }
func TestStateMachine_AllInvalidTransitions(t *testing.T) { ... }

// Unit test: amount parsing
func TestParseAmount_ValidInputs(t *testing.T) { ... }
func TestParseAmount_RejectsNegative(t *testing.T) { ... }
func TestParseAmount_RejectsFractionalCents(t *testing.T) { ... }

// Unit test: cutoff calculation
func TestCutoff_BeforeCutoff_ReturnsToday(t *testing.T) { ... }
func TestCutoff_AfterCutoff_ReturnsNextBusinessDay(t *testing.T) { ... }
func TestCutoff_FridayEvening_ReturnsMonday(t *testing.T) { ... }
```

**Recommended test manifest (minimum 15 tests):**

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

**Pros:**
- Integration tests prove the system works end-to-end — not just that individual functions return correct values
- In-memory SQLite makes integration tests fast (milliseconds, not seconds)
- HTTP test server (`httptest.NewServer`) means tests exercise the real API, including JSON serialization, middleware, and error formatting
- The test manifest directly maps to rubric criteria — every required scenario has a named test
- Unit tests for pure logic (state machine, amounts, cutoff) provide fast, precise feedback
- Test 18 (`LedgerInvariant_DebitsEqualCredits`) is the killer test — it proves financial correctness across all operations

**Cons:**
- More setup code than pure unit tests (test helpers, HTTP client, database setup)
- Integration tests are slightly slower (still under 1 second with in-memory SQLite)
- Must maintain test helpers as the API evolves

### Strategy C: Property-Based / Fuzz Testing

Use Go's built-in fuzz testing to generate random deposit amounts, account numbers, and state sequences. Verify invariants hold across all generated inputs.

**Pros:**
- Finds edge cases human-written tests miss
- Proves invariants hold for arbitrary inputs

**Cons:**
- Fuzz tests are slow and non-deterministic — bad for a demo
- Output is hard to interpret — a failure gives you a random byte sequence, not a readable scenario
- Doesn't satisfy the "deterministic demo scripts that exercise all paths" requirement
- Adding fuzz tests on top of a solid integration suite is fine, but they can't replace named scenario tests

### Winning Recommendation: **Strategy B — Integration Tests + Targeted Unit Tests**

**Rationale:** The rubric says "all paths exercised" — integration tests through the real HTTP API are the only way to prove this. A unit test that mocks the database can pass while the actual SQL query is wrong. The test manifest above covers all 7 vendor scenarios, both duplicate detection layers, operator approve/reject, the return/reversal path, settlement file contents, auth failures, and the ledger balance invariant. That's 18 tests — well above the minimum 10 — with each one traceable to a specific rubric criterion. The `TestLedgerInvariant_DebitsEqualCredits` test is the crown jewel: run all scenarios, then verify that every cent that entered the system is accounted for.

---

## Question 24: Pipeline Orchestration — How Does a Deposit Flow Through Multiple Services?

### Why This Matters

A deposit touches 4 services in sequence: submission → vendor validation → funding rules → ledger posting. The question is how to wire these together. A synchronous call chain is simple but couples everything. An async pipeline is flexible but harder to trace. The choice affects testability, error handling, and the state machine's behavior.

### Strategy A: Synchronous Request-Response Chain

The deposit submission handler calls the vendor stub, then the funding service, then the ledger posting — all synchronously within the same HTTP request. The response includes the final state.

```go
func (h *Handler) SubmitDeposit(w http.ResponseWriter, r *http.Request) {
    deposit := parseRequest(r)                    // → Requested
    vendorResult := h.vendor.Validate(deposit)    // → Validating → pass/fail
    if vendorResult.Failed() {
        h.reject(deposit, vendorResult.Error())   // → Rejected
        return
    }
    rulesResult := h.funding.Apply(deposit)       // → Analyzing → pass/fail
    if rulesResult.Failed() {
        h.reject(deposit, rulesResult.Error())    // → Rejected
        return
    }
    if rulesResult.NeedsReview() {
        h.flag(deposit, rulesResult.Reason())     // stays in Analyzing
        return
    }
    h.ledger.Post(deposit)                         // → FundsPosted
    respond(w, deposit)
}
```

**Pros:**
- Entire flow visible in one function — a reviewer reads top to bottom and understands the pipeline
- Error handling is straightforward — any failure returns immediately
- State transitions happen in order, in the same transaction
- No background workers, no queues, no race conditions

**Cons:**
- Client blocks until the entire pipeline completes — if vendor validation is slow (real-world scenario), the mobile app hangs
- Tight coupling — changing the pipeline order requires modifying the handler
- Hard to add async steps later (e.g., async vendor callback)

### Strategy B: Internal Pipeline with Step Functions

Define the pipeline as a sequence of named steps. Each step receives the deposit, performs its work, and returns a result that determines the next step. The pipeline runner executes steps in sequence but each step is a discrete, testable unit.

```go
type PipelineStep func(ctx context.Context, deposit *Transfer) (PipelineAction, error)

type PipelineAction int
const (
    Continue PipelineAction = iota  // proceed to next step
    HaltRejected                     // stop — deposit rejected
    HaltFlagged                      // stop — needs operator review
    HaltApproved                     // stop — ready for ledger posting
)

func BuildDepositPipeline(vendor *VendorStub, funding *FundingService, ledger *LedgerService) []PipelineStep {
    return []PipelineStep{
        // Step 1: Vendor Validation
        func(ctx context.Context, d *Transfer) (PipelineAction, error) {
            result, err := vendor.Validate(d)
            if err != nil { return HaltRejected, err }
            if result.NeedsReview { return HaltFlagged, nil }
            d.VendorData = result.Data
            return Continue, nil
        },
        // Step 2: Business Rules
        func(ctx context.Context, d *Transfer) (PipelineAction, error) {
            result, err := funding.ApplyRules(d)
            if err != nil { return HaltRejected, err }
            if result.NeedsReview { return HaltFlagged, nil }
            return Continue, nil
        },
        // Step 3: Ledger Posting
        func(ctx context.Context, d *Transfer) (PipelineAction, error) {
            return HaltApproved, ledger.Post(d)
        },
    }
}

func (p *Pipeline) Execute(ctx context.Context, deposit *Transfer) error {
    for i, step := range p.steps {
        action, err := step(ctx, deposit)
        p.logStep(deposit.ID, i, action, err)  // audit trail
        switch action {
        case Continue:
            continue
        case HaltRejected:
            return p.sm.Transition(deposit, Rejected, "system", err.Error())
        case HaltFlagged:
            return nil // stays in Analyzing, appears in operator queue
        case HaltApproved:
            return p.sm.Transition(deposit, FundsPosted, "system", "all checks passed")
        }
    }
    return nil
}
```

**Pros:**
- Each step is independently testable — `step(ctx, deposit)` returns a predictable action
- Pipeline runner handles state transitions uniformly — no scattered transition logic
- Adding a step (e.g., "compliance screening") means appending to the slice — no handler refactoring
- The `logStep` call at each step creates the per-deposit decision trace automatically
- Steps can be reordered or conditionally included based on configuration
- The pipeline definition itself (`BuildDepositPipeline`) is readable documentation of the processing order

**Cons:**
- Slightly more abstraction than a raw handler — the pipeline runner is ~30 lines of framework code
- Step function signatures must be consistent — limits flexibility per step
- Debugging requires understanding the pipeline model (mitigated by step logging)

### Strategy C: Async Event-Driven Pipeline with Goroutines

Each service runs as a goroutine. Channels connect them. A deposit is submitted to the first channel; each goroutine reads, processes, and pushes to the next channel.

**Pros:**
- Natural concurrency — services process in parallel where possible
- Decoupled — each service only knows about its input/output channels

**Cons:**
- Channels add complexity: buffering, closing, error propagation across goroutines
- A failure in step 3 must propagate back to step 1's error handler — channel-based error propagation is messy
- State machine transitions from multiple goroutines require synchronization
- Debugging channel-based pipelines is notoriously difficult
- The benefits of parallelism are irrelevant for a sequential validation pipeline

### Winning Recommendation: **Strategy B — Internal Pipeline with Step Functions**

**Rationale:** The step function pipeline gives you the clarity of Strategy A (sequential, readable) with the extensibility of a proper abstraction. Each step is a function you can test in isolation. The pipeline runner handles state transitions uniformly, so there's no risk of a step forgetting to transition. The `logStep` call at every step means the per-deposit decision trace (observability requirement) is built into the pipeline itself — no separate logging code needed. And the `BuildDepositPipeline` function is executable documentation: a reviewer reads the step list and knows exactly what happens to a deposit.

---

## Question 25: Observability — How Do You Surface Failures and Performance in a Debuggable Way?

### Why This Matters

The spec requires "per-deposit decision trace," "differentiate between deposit sources in logs," and "monitor for missing or delayed settlement files." The observability rubric is embedded in the operator workflow category (10 pts). A system that works but can't explain *why* it made a decision is incomplete.

### Strategy A: Standard Library Logging (`log.Printf`)

Use Go's `log` package with prefixed messages. Each service prefixes its log output with the service name.

```go
log.Printf("[vendor] deposit=%s scenario=CLEAN_PASS micr=%s", depositID, micrData)
log.Printf("[funding] deposit=%s rule=LIMIT result=PASS threshold=500000 amount=15000", depositID)
```

**Pros:**
- Zero dependencies
- Greppable by deposit ID or service name

**Cons:**
- Unstructured — parsing requires regex
- No log levels — debug messages mixed with errors
- No machine-readable format for automated analysis
- Doesn't satisfy "per-deposit decision trace" if logs aren't queryable by deposit

### Strategy B: Structured Logging with `slog` + Correlation IDs, Backed by the Event Log

Use Go 1.21's `slog` package for structured, leveled logging with JSON output. Every log entry includes a `deposit_id` field for correlation. The structured logs complement (not replace) the `deposit_events` table — logs are for real-time debugging, events are for the audit trail and UI.

```go
type DepositLogger struct {
    base *slog.Logger
}

func (l *DepositLogger) ForDeposit(depositID string) *slog.Logger {
    return l.base.With(
        slog.String("deposit_id", depositID),
        slog.String("trace_id", generateTraceID()),
    )
}

// Usage in vendor stub:
func (v *VendorStub) Validate(deposit *Transfer) (*VendorResult, error) {
    log := v.logger.ForDeposit(deposit.ID)
    log.Info("vendor validation started",
        slog.String("account", deposit.AccountID),
        slog.String("scenario", v.determineScenario(deposit)),
    )
    result := v.processScenario(deposit)
    log.Info("vendor validation completed",
        slog.String("result", result.Status),
        slog.String("error_code", result.ErrorCode),
        slog.Float64("confidence", result.Confidence),
    )
    return result, nil
}
```

**Log output (JSON mode):**
```json
{"time":"2026-03-09T14:30:01Z","level":"INFO","msg":"vendor validation started","deposit_id":"DEP-001","trace_id":"abc123","account":"PASS-10001","scenario":"CLEAN_PASS"}
{"time":"2026-03-09T14:30:01Z","level":"INFO","msg":"vendor validation completed","deposit_id":"DEP-001","trace_id":"abc123","result":"PASS","error_code":"","confidence":0.98}
```

**Settlement monitoring:**
```go
func (s *SettlementService) CheckForMissedSettlement(clock Clock) {
    cutoff := getCutoffTime(clock.Now())
    if clock.Now().After(cutoff.Add(30 * time.Minute)) {
        pending := s.repo.CountFundsPostedDeposits()
        if pending > 0 {
            s.logger.Warn("settlement file may be delayed",
                slog.Int("pending_deposits", pending),
                slog.Time("cutoff_was", cutoff),
                slog.Duration("elapsed", clock.Now().Sub(cutoff)),
            )
        }
    }
}
```

**Pros:**
- `slog` is stdlib (Go 1.21+) — no external dependencies
- Structured JSON logs are greppable, parseable, and can be piped to `jq` for analysis
- `deposit_id` on every log entry enables `jq 'select(.deposit_id == "DEP-001")'` for per-deposit traces
- Complements the `deposit_events` table — logs for debugging, events for audit/UI
- Log levels (Info, Warn, Error) let you filter noise in the demo
- Settlement monitoring logs a warning if pending deposits exist 30 minutes past cutoff
- The `trace_id` enables request-level correlation across services within the pipeline

**Cons:**
- Requires Go 1.21+ (standard for any new project in 2026)
- JSON log output is less human-readable than plaintext (mitigated by `slog.NewTextHandler` for development)
- Must ensure every service method includes the deposit_id — discipline required

### Strategy C: Distributed Tracing with OpenTelemetry

Instrument the application with OpenTelemetry spans. Each pipeline step is a span. Run a Jaeger instance in Docker to visualize traces.

**Pros:**
- Industry-standard observability
- Visual trace diagrams show timing and hierarchy of operations

**Cons:**
- Requires Jaeger in `docker-compose.yml` — another service to start and wait for
- OpenTelemetry SDK adds significant dependency weight
- Reviewer must open Jaeger UI to see traces — not inline in the application
- Massive overkill for a single-binary MVP with synthetic load

### Winning Recommendation: **Strategy B — Structured Logging with `slog` + Correlation IDs**

**Rationale:** `slog` gives you structured, queryable logs with zero external dependencies. The `deposit_id` correlation on every log entry means `jq 'select(.deposit_id == "DEP-001")'` produces the complete decision trace for any deposit — fulfilling the observability requirement. The settlement monitoring function demonstrates you've considered operational health, not just happy-path functionality. In development, use `TextHandler` for readable console output; the demo script can switch to `JSONHandler` to pipe output to the report.

---

## Question 26: Makefile Design — What Does the Developer Experience Look Like?

### Why This Matters

The spec says "one-command setup (e.g., `make dev` or `docker compose up`)" and the rubric allocates 10 points to developer experience. The Makefile is the first file a reviewer interacts with. If `make dev` fails or requires manual steps, the first impression is ruined.

### Strategy A: Minimal Makefile with Build and Run

```makefile
build:
	go build -o bin/mcd-server ./cmd/server

run: build
	./bin/mcd-server
```

**Pros:**
- Simple — two targets
- Clear what each does

**Cons:**
- `make run` doesn't set up the database or seed data
- No test, demo, or report targets
- Reviewer must figure out the workflow themselves
- Missing `.env` file causes a silent failure

### Strategy B: Complete Developer Workflow Makefile

A Makefile that scripts the entire developer and reviewer experience — from setup to test to demo to report generation.

```makefile
.PHONY: dev test demo report clean help

# Default target — show available commands
help:
	@echo "Mobile Check Deposit System"
	@echo "═══════════════════════════════════════════"
	@echo "  make dev      Start the server (builds, seeds, runs)"
	@echo "  make test     Run all tests with coverage"
	@echo "  make demo     Run demo scenario script (server must be running)"
	@echo "  make report   Generate test + demo reports in /reports"
	@echo "  make clean    Remove build artifacts and data"
	@echo "  make docker   Build and run via Docker Compose"
	@echo "═══════════════════════════════════════════"

# One-command setup: build, ensure dirs, run with auto-seed
dev:
	@mkdir -p data/images reports
	@cp -n .env.example .env 2>/dev/null || true
	go build -o bin/mcd-server ./cmd/server
	@echo "Starting MCD server on http://localhost:8080"
	@echo "  Dashboard:  http://localhost:8080/"
	@echo "  Submit:     http://localhost:8080/submit"
	@echo "  Operator:   http://localhost:8080/operator"
	./bin/mcd-server

# Run tests with coverage report
test:
	@mkdir -p reports
	go test ./... -v -count=1 -coverprofile=reports/coverage.out 2>&1 | tee reports/test_output.txt
	go tool cover -func=reports/coverage.out | tail -1
	@echo "Test report saved to reports/test_output.txt"

# Run narrated demo script
demo:
	@chmod +x scripts/demo.sh
	./scripts/demo.sh 2>&1 | tee reports/demo_results.txt
	@echo "Demo report saved to reports/demo_results.txt"

# Generate all reports for submission
report: test demo
	@echo "All reports generated in /reports"
	@ls -la reports/

# Docker alternative
docker:
	docker compose up --build

# Clean up
clean:
	rm -rf bin/ data/ reports/*.txt reports/*.out
	@echo "Cleaned build artifacts and data"
```

**Pros:**
- `make help` (default target) shows the reviewer everything they can do — no README scanning required
- `make dev` does everything: creates directories, copies `.env.example` to `.env`, builds, prints URLs, and runs
- `make test` produces a coverage report and saves output for the `/reports` deliverable
- `make demo` runs the scenario script and saves output for submission
- `make report` chains test + demo for a single-command submission preparation
- URLs printed at startup tell the reviewer exactly where to look
- `make docker` provides an alternative path for reviewers who prefer containers
- `make clean` resets everything — safe to re-run the demo from scratch

**Cons:**
- More Makefile to maintain
- `make demo` requires the server to be running separately (documented in help output)
- macOS vs. Linux `cp -n` behavior may differ slightly (but works for this use case)

### Strategy C: Just Use Docker Compose for Everything

No Makefile. The `docker-compose.yml` file builds and runs the application. All commands go through `docker compose exec`.

**Pros:**
- Guaranteed consistent environment — no "works on my machine" issues
- Single `docker compose up` starts everything

**Cons:**
- Reviewers without Docker installed can't run the project — friction
- Docker build adds 30-60 seconds to the feedback loop on every code change
- `docker compose exec` for running tests and demos is verbose
- Logs are mixed between Docker and application output
- Some reviewers prefer running binaries directly for faster iteration

### Winning Recommendation: **Strategy B — Complete Developer Workflow Makefile**

**Rationale:** The Makefile is the project's command palette. `make help` as the default target means a reviewer who types `make` (with no arguments) immediately sees every available action. `make dev` fulfills the one-command setup requirement — it handles directory creation, config file setup, building, and running. The `make report` target chains tests and demo into a single command that populates the `/reports` directory for submission. And `make docker` exists as a fallback. This Makefile directly maximizes the 10 developer experience rubric points.

---

## Question 27: Database Schema Migration Strategy

### Why This Matters

The schema will evolve during development. Adding a column, renaming a table, or adding an index means the database must be updated. For an MVP, this seems trivial — but a reviewer who clones the repo and runs `make dev` on a stale database expects it to work. The migration strategy also signals engineering maturity.

### Strategy A: Drop and Recreate on Every Startup

On startup, drop all tables and recreate from the schema definition. Re-seed data every time.

**Pros:**
- Always running the latest schema — no migration conflicts
- Trivially simple — `DROP TABLE IF EXISTS; CREATE TABLE`

**Cons:**
- Loses any data from previous runs (including demo state the reviewer was exploring)
- Slow startup if seed data is complex
- Can't demonstrate data persistence — a server restart wipes everything
- Signals to a reviewer that you haven't considered data lifecycle

### Strategy B: Schema Version Table with Forward-Only Migrations

A `schema_migrations` table tracks which migrations have been applied. On startup, the application runs any unapplied migrations in order. Migrations are Go functions, not SQL files — they can include data transformations.

```go
var migrations = []Migration{
    {Version: 1, Name: "initial_schema", Up: func(tx *sql.Tx) error {
        _, err := tx.Exec(`
            CREATE TABLE IF NOT EXISTS transfers (...);
            CREATE TABLE IF NOT EXISTS ledger_entries (...);
            CREATE TABLE IF NOT EXISTS deposit_events (...);
        `)
        return err
    }},
    {Version: 2, Name: "add_check_number_index", Up: func(tx *sql.Tx) error {
        _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_transfers_check ON transfers(micr_routing, micr_account, check_number)`)
        return err
    }},
}

func RunMigrations(db *sql.DB) error {
    db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
    for _, m := range migrations {
        var exists bool
        db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", m.Version).Scan(&exists)
        if !exists {
            tx, _ := db.Begin()
            if err := m.Up(tx); err != nil {
                tx.Rollback()
                return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
            }
            tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version)
            tx.Commit()
            log.Printf("Applied migration %d: %s", m.Version, m.Name)
        }
    }
    return nil
}
```

**Pros:**
- Schema updates are incremental — existing data survives server restarts
- Each migration runs in a transaction — partial migrations are rolled back
- Migration history is visible in the `schema_migrations` table
- Reviewer can restart the server without losing their demo state
- The migration log at startup shows exactly what schema version the database is at
- No external migration tool (no `golang-migrate`, no `goose`) — just a slice of functions

**Cons:**
- More code than a simple `CREATE TABLE` (~50 lines for the runner)
- Forward-only — no rollback (acceptable for MVP, note in decision log)
- Must add a new migration entry for every schema change during development

### Strategy C: External Migration Tool (goose, golang-migrate)

Use a third-party migration tool with SQL files in a `/migrations` directory. Run migrations via CLI command before starting the server.

**Pros:**
- Well-tested migration runner with rollback support
- SQL migration files are visible in the repo
- Standard pattern in production Go applications

**Cons:**
- Adds an external dependency and a CLI tool to install
- `make dev` must run `goose up` before starting the server — an extra step
- Migration CLI may not be installed on the reviewer's machine
- Over-engineered for an MVP with a single-digit number of tables

### Winning Recommendation: **Strategy B — Embedded Forward-Only Migrations**

**Rationale:** The migration runner is ~50 lines of code that pays for itself in reviewer experience. The server starts, runs any pending migrations, seeds if empty, and is ready. No external tools, no manual steps, no stale schemas. The `schema_migrations` table is a small but visible signal of engineering maturity. If the schema changes during development (and it will), adding a migration is one function in a slice. The reviewer benefits because they can restart the server without losing data, and the startup log shows the current schema version.

---

## Question 28: Contribution Type Handling for Retirement Accounts

### Why This Matters

The spec requires "contribution type defaults (individual contribution for retirement-type accounts)" and "ability to override contribution type defaults if needed" in the operator workflow. This is a domain-specific business rule that affects ledger posting metadata and must be configurable per correspondent.

### Strategy A: Ignore Contribution Types Entirely

Treat all deposits as simple cash deposits. No contribution type field on the transfer record.

**Pros:**
- Simpler data model
- Fewer fields to validate and display

**Cons:**
- The spec explicitly calls it out as a business rule — ignoring it is a rubric miss
- Missing an opportunity to demonstrate domain understanding of brokerage account types

### Strategy B: Default-and-Override Pattern with Account Type Detection

Each account has a `type` field (INDIVIDUAL, IRA_TRADITIONAL, IRA_ROTH, 401K, etc.). When the account type is retirement-related, the funding service automatically assigns a contribution type based on the correspondent's configured default. The operator can override this during review.

```go
type AccountType string
const (
    AccountIndividual   AccountType = "INDIVIDUAL"
    AccountIRATraditional AccountType = "IRA_TRADITIONAL"
    AccountIRARoth      AccountType = "IRA_ROTH"
    Account401K         AccountType = "401K"
)

type ContributionType string
const (
    ContribIndividual ContributionType = "INDIVIDUAL"
    ContribEmployer   ContributionType = "EMPLOYER"
    ContribRollover   ContributionType = "ROLLOVER"
)

func (f *FundingService) ResolveContributionType(account *Account, correspondent *Correspondent) ContributionType {
    if !account.IsRetirement() {
        return ""  // Non-retirement accounts don't have contribution types
    }
    // Default from correspondent config
    return correspondent.RetirementContributionDefault
}
```

**Operator override in the review UI:**
```
┌─────────────────────────────────────────────┐
│ Deposit Review: DEP-007                      │
│                                              │
│ Account: IRA-TRAD-5001 (IRA Traditional)    │
│ Amount: $2,500.00                            │
│ Contribution Type: INDIVIDUAL (default)      │
│                                              │
│ [Override: ▼ INDIVIDUAL / EMPLOYER / ROLLOVER]│
│                                              │
│ [Approve]  [Reject]                          │
└─────────────────────────────────────────────┘
```

**Seed data to demonstrate this:**
```yaml
investors:
  - id: "INV-005"
    name: "Eve Nakamura"
    correspondent: "CORR-APEX"
    accounts:
      - id: "IRA-TRAD-5001"
        type: IRA_TRADITIONAL
        # Will default to INDIVIDUAL contribution per CORR-APEX config
      - id: "PASS-10004"
        type: INDIVIDUAL
        # No contribution type applies
```

**Pros:**
- Directly implements the spec's "contribution type defaults" and "operator override" requirements
- The correspondent config drives the default — demonstrating parameterized business rules
- The operator UI shows the default with an override dropdown — demonstrating the review workflow
- Non-retirement accounts simply skip the contribution logic — clean branching
- A test can verify: IRA account → default contribution type assigned → operator overrides → ledger entry carries the override

**Cons:**
- Adds fields to the account model, transfer record, and operator UI
- Must seed retirement-type accounts in the demo data (2-3 additional accounts)
- Contribution type validation (e.g., "EMPLOYER" only valid for 401K) adds business rule complexity

### Strategy C: Full IRS Contribution Limit Enforcement

Track annual contribution totals per account. Enforce IRS limits ($7,000 IRA, $23,000 401K for 2026). Reject deposits that would exceed the annual limit.

**Pros:**
- Maximum domain fidelity — demonstrates deep understanding of retirement account rules
- Would impress a reviewer with brokerage domain knowledge

**Cons:**
- The spec says "contribution type defaults" — not "contribution limit enforcement"
- Annual limit tracking requires year-to-date aggregation across all deposits, not just the current one
- IRS limits change annually — hardcoding 2026 values is fragile
- Massive scope creep for an MVP — time better spent on core requirements
- The spec already has a $5,000 deposit limit as the MVP rule; adding IRS limits on top creates confusion about which limit applies

### Winning Recommendation: **Strategy B — Default-and-Override Pattern**

**Rationale:** The spec asks for two things: a default contribution type for retirement accounts (driven by correspondent config) and an operator override. Strategy B implements exactly this — no more, no less. It demonstrates that the funding service applies domain-aware defaults, the operator can intervene, and the ledger entry carries the final contribution type. Adding 2-3 retirement accounts to the seed data gives the demo a scenario to walk through. The override dropdown in the operator UI is a visible proof point for the "operator can override contribution type defaults" requirement.

---

## Question 29: Risk Scoring and Flagging Logic — How Does the System Decide What Needs Review?

### Why This Matters

The spec requires that flagged deposits appear in the operator queue with "risk indicators and Vendor Service scores." But it doesn't define what a "risk score" is or how flagging works beyond specific vendor scenarios (MICR failure, amount mismatch). This is an opportunity to demonstrate domain thinking by building a coherent risk model that drives the operator queue's sort order and urgency indicators.

### Strategy A: Binary Flag — Review or Don't

Deposits are either flagged for review or not. No scoring, no gradation. The operator sees all flagged items in FIFO order.

**Pros:**
- Simplest implementation — a boolean `needs_review` column
- Clear semantics — no ambiguity about what the score means

**Cons:**
- All flagged deposits look equally urgent — a MICR failure and an amount mismatch that's off by $0.01 get the same treatment
- The spec says "risk scores" — a boolean doesn't qualify
- No way to prioritize the operator queue

### Strategy B: Composite Risk Score from Multiple Signals

Compute a risk score (0-100) from multiple weighted signals. The score determines (1) whether the deposit is flagged for review and (2) the sort order in the operator queue. Signals come from both the vendor stub and the funding service.

```go
type RiskAssessment struct {
    Score       int                `json:"score"`       // 0-100
    Level       string             `json:"level"`       // LOW, MEDIUM, HIGH, CRITICAL
    Signals     []RiskSignal       `json:"signals"`     // individual risk factors
    NeedsReview bool               `json:"needs_review"`
}

type RiskSignal struct {
    Source     string `json:"source"`      // "vendor", "funding", "rules"
    Factor    string `json:"factor"`      // "micr_confidence", "amount_delta", "velocity"
    Value     string `json:"value"`       // "0.45", "$12.50", "3 deposits/hour"
    Weight    int    `json:"weight"`      // contribution to total score
    Threshold string `json:"threshold"`   // what would be acceptable
}

func (f *FundingService) AssessRisk(deposit *Transfer, vendorResult *VendorResult) *RiskAssessment {
    var signals []RiskSignal
    totalScore := 0

    // Signal 1: MICR confidence (from vendor)
    if vendorResult.MICRConfidence < 0.90 {
        weight := int((1.0 - vendorResult.MICRConfidence) * 40)  // max 40 points
        signals = append(signals, RiskSignal{
            Source: "vendor", Factor: "micr_confidence",
            Value: fmt.Sprintf("%.2f", vendorResult.MICRConfidence),
            Weight: weight, Threshold: "≥ 0.90",
        })
        totalScore += weight
    }

    // Signal 2: Amount discrepancy (from vendor OCR vs. entered)
    if vendorResult.OCRAmount != deposit.Amount {
        delta := abs(vendorResult.OCRAmount - deposit.Amount)
        weight := min(30, int(float64(delta)/100))  // $1 delta = 1 point, max 30
        signals = append(signals, RiskSignal{
            Source: "vendor", Factor: "amount_delta",
            Value: Amount(delta).ToDollars(), Weight: weight, Threshold: "$0.00",
        })
        totalScore += weight
    }

    // Signal 3: Deposit amount relative to limit (high-value flag)
    limitRatio := float64(deposit.Amount) / float64(deposit.DepositLimit)
    if limitRatio > 0.80 {
        weight := int((limitRatio - 0.80) * 100)  // max 20 points
        signals = append(signals, RiskSignal{
            Source: "funding", Factor: "limit_proximity",
            Value: fmt.Sprintf("%.0f%% of limit", limitRatio*100),
            Weight: weight, Threshold: "< 80% of limit",
        })
        totalScore += weight
    }

    // Signal 4: First-time depositor
    if deposit.IsFirstDeposit {
        signals = append(signals, RiskSignal{
            Source: "funding", Factor: "first_deposit",
            Value: "true", Weight: 10, Threshold: "N/A",
        })
        totalScore += 10
    }

    level := "LOW"
    needsReview := false
    switch {
    case totalScore >= 60:
        level = "CRITICAL"; needsReview = true
    case totalScore >= 40:
        level = "HIGH"; needsReview = true
    case totalScore >= 20:
        level = "MEDIUM"; needsReview = true
    default:
        level = "LOW"
    }

    return &RiskAssessment{
        Score: min(100, totalScore), Level: level,
        Signals: signals, NeedsReview: needsReview,
    }
}
```

**Operator queue display:**
```
┌─────────────────────────────────────────────────────────────────────────┐
│ Operator Review Queue (3 items)                          [Filter ▼]   │
├────────┬──────────┬────────┬───────┬──────────────────────────────────┤
│ ID     │ Amount   │ Score  │ Level │ Top Risk Signal                  │
├────────┼──────────┼────────┼───────┼──────────────────────────────────┤
│ DEP-12 │ $2,500   │ 65     │ CRIT  │ MICR confidence: 0.42 (< 0.90) │
│ DEP-09 │ $4,800   │ 45     │ HIGH  │ 96% of deposit limit            │
│ DEP-15 │ $1,200   │ 25     │ MED   │ Amount delta: $12.50            │
└────────┴──────────┴────────┴───────┴──────────────────────────────────┘
```

**Pros:**
- Transforms the operator queue from a flat list into a prioritized, actionable dashboard
- Each signal is transparent — the operator sees *why* the deposit was flagged, not just that it was
- Signals from both vendor (MICR confidence, OCR delta) and funding (limit proximity, first deposit) demonstrate cross-service risk aggregation
- The score drives sort order — critical items surface first
- Fully testable: `AssessRisk(deposit, vendorResult)` returns a deterministic score based on inputs
- The `RiskSignal` struct is self-documenting — each factor explains itself with value and threshold
- Directly satisfies "risk indicators and Vendor Service scores" requirement

**Cons:**
- Weights and thresholds are somewhat arbitrary for an MVP (mitigated by documenting in decision log)
- More code than a boolean flag (~60 lines for the risk assessment function)
- The risk model is a design choice that should be explained in the decision log

### Strategy C: Machine Learning Risk Model

Train a classifier on historical check deposit data to predict return probability. Use the prediction score as the risk score.

**Pros:**
- Most sophisticated approach — data-driven risk assessment

**Cons:**
- The spec explicitly says "AI/ML Frameworks: Not required"
- No historical data exists — it's all synthetic
- Training a model for an MVP with 5 test investors is absurd
- Adds Python/TensorFlow dependency to a Go project
- Reviewers would see this as misplaced effort

### Winning Recommendation: **Strategy B — Composite Risk Score**

**Rationale:** The risk score transforms the operator queue from a boring list into an intelligent triage tool. Four signals (MICR confidence, amount delta, limit proximity, first deposit) are enough to demonstrate the concept without overcomplicating the system. The operator sees a score, a severity level, and the individual signals that contributed — making review decisions faster and more informed. The scoring function is deterministic and testable: a MICR confidence of 0.42 always produces the same risk score. This directly addresses the "risk indicators and Vendor Service scores" requirement with visible, understandable output.

---

## Question 30: Decision Log Structure — How Do You Maximize Rubric Points on Documentation?

### Why This Matters

The spec requires a decision log documenting "key decisions and alternatives considered." The rubric scores system design (20 pts) partly based on "trade-off rationale." A well-structured decision log isn't just documentation — it's a rubric cheat sheet that tells the reviewer "I considered alternatives and chose deliberately."

### Strategy A: Prose Paragraphs

A narrative document explaining major decisions in paragraph form. "We chose Go because..."

**Pros:**
- Natural writing style
- Can explain nuanced reasoning

**Cons:**
- Hard to scan — a reviewer looking for "why SQLite?" must read through paragraphs
- No consistent structure — some decisions get detailed treatment, others get a sentence
- Doesn't demonstrate systematic evaluation of alternatives

### Strategy B: ADR (Architectural Decision Record) Format

Use the industry-standard ADR format for each significant decision. Each ADR has a consistent structure: context, decision, alternatives considered, consequences.

```markdown
# Decision Log

## ADR-001: Programming Language — Go

**Status:** Accepted
**Date:** 2026-03-09

**Context:**
The system requires a single-binary deployment for one-command setup, explicit control
flow for code reviewability, and lightweight concurrency for simulating async vendor
interactions. The spec accepts Go or Java.

**Decision:**
Go single-binary monolith. All services (vendor stub, funding service, settlement
engine, operator API) compiled into one binary using Go's embed package for static
assets.

**Alternatives Considered:**
- *Java + Spring Boot:* Richer ORM (Hibernate) and validation framework, but JVM
  startup time, 200MB+ memory footprint, and annotation-driven control flow reduce
  reviewability. Multi-stage Docker build required.
- *Hybrid Go + Java:* Best-of-both but two build systems, two test harnesses, and
  mandatory Docker orchestration. Over-engineered for MVP scope.

**Consequences:**
- (+) `make dev` compiles and runs in < 5 seconds
- (+) Explicit control flow — no framework magic to trace through
- (+) Single binary deployment — no runtime dependencies
- (-) More verbose error handling than Java's exceptions
- (-) Hand-written SQL instead of ORM — mitigated by sqlc or raw database/sql

---

## ADR-002: Data Store — SQLite with Double-Entry Ledger

**Status:** Accepted
**Date:** 2026-03-09

**Context:**
The ledger must enforce that debits equal credits, support immutable audit history,
and enable settlement reconciliation. The spec allows SQLite, JSON, or equivalent.

**Decision:**
SQLite with three core tables: transfers (lifecycle), ledger_entries (double-entry
financial records), and deposit_events (audit trail). All amounts stored as integer
cents (int64).

**Alternatives Considered:**
- *Flat transfer table:* No independent debit/credit verification. Cannot reconcile
  ledger against settlement file. Fails the financial accuracy requirement.
- *Event-sourced ledger:* Perfect audit trail but requires projection layer for
  balance queries. SQLite is not a natural event store. Over-engineered for MVP.

**Consequences:**
- (+) Double-entry invariant enforceable in DB transactions
- (+) Balance queries via simple aggregation on ledger_entries
- (+) Reversals create new entries (immutable history)
- (-) More tables and insert logic than a flat model
- (-) SQLite single-writer serializes concurrent posts (acceptable at MVP scale)

---

## ADR-003: Vendor Stub — Magic Account Prefixes

...

## ADR-004: Settlement Format — Structured JSON Mirroring X9

...
```

**Recommended ADRs for the submission (10 total):**

| ADR | Decision | Why It Matters to the Rubric |
|---|---|---|
| 001 | Language: Go | System design (20 pts) — justified choice |
| 002 | Data store: SQLite double-entry | Core correctness (25 pts) — financial accuracy |
| 003 | Vendor stub: Magic account prefixes | Stub quality (15 pts) — configurability |
| 004 | Settlement: X9 JSON hierarchy | Core correctness (25 pts) — domain knowledge |
| 005 | State machine: Hardcoded map | System design (20 pts) — simplicity rationale |
| 006 | Amount representation: Integer cents | Core correctness (25 pts) — precision |
| 007 | Risk scoring: Composite model | Operator workflow (10 pts) — risk indicators |
| 008 | API design: Resource-oriented REST | System design (20 pts) — API architecture |
| 009 | Concurrency: BEGIN IMMEDIATE | Core correctness (25 pts) — transaction safety |
| 010 | Pipeline: Step functions | System design (20 pts) — separation of concerns |

**Pros:**
- Each ADR maps to a rubric category — the reviewer can see you optimized for what matters
- Consistent structure (Context → Decision → Alternatives → Consequences) means the reviewer knows where to look
- "Alternatives Considered" directly satisfies the "alternatives considered" deliverable requirement
- Consequences section shows self-awareness — listing negatives demonstrates honesty and maturity
- 10 ADRs covering the 10 most impactful decisions is comprehensive without being exhausting
- The format is an industry standard (see Michael Nygard's ADR pattern) — signals engineering maturity

**Cons:**
- More writing effort than prose paragraphs
- Must keep ADRs consistent in depth — a thorough ADR-001 followed by a thin ADR-010 looks rushed
- The ADR format is slightly more formal than a typical MVP warrants (but the rubric rewards it)

### Strategy C: Comparison Matrix Spreadsheet

A table with rows for each decision and columns for each alternative, scored on criteria like "complexity," "correctness," and "reviewability."

**Pros:**
- Dense — all decisions visible at once
- Quantitative — scores enable comparison

**Cons:**
- Scores are arbitrary without context — "Go scored 8/10 on simplicity" means nothing without explanation
- No room for nuanced reasoning — a cell in a spreadsheet can't explain why integer cents matter
- Doesn't satisfy the "alternatives considered" requirement — a score isn't an explanation
- Format doesn't match developer expectations for a decision log

### Winning Recommendation: **Strategy B — ADR Format**

**Rationale:** The ADR format is purpose-built for this deliverable. Each record answers exactly what the rubric asks: what did you decide, what alternatives did you consider, and why did you choose this path? The 10-ADR structure maps directly to the rubric categories, making it easy for a reviewer to trace each design choice to its justification. The "Consequences" section with both positives and negatives demonstrates engineering judgment — you didn't just pick the easiest option, you understood the tradeoffs.

---

---

# CONSOLIDATED SUMMARY

---

## Decision Matrix — All 30 Decisions

### Round 1 — Foundations

| # | Question | Winning Strategy | Key Rationale |
|---|---|---|---|
| 1 | Language choice | Go single-binary | One-command setup, explicit control flow, no framework magic |
| 2 | Data store / ledger | SQLite double-entry (transfers + ledger_entries) | Enforces debit=credit invariant, immutable audit trail, reconciliation-ready |
| 3 | Vendor stub mechanism | Magic account prefixes + header override | Self-documenting tests, deterministic, no hidden state |
| 4 | Operator UI | Embedded single-page web UI | Check images require visual rendering; Go embed preserves one-binary setup |
| 5 | State machine | Hardcoded transition map | 8 states fit in 15 lines; pure, testable, no library overhead |
| 6 | Settlement file format | Structured JSON mirroring X9 hierarchy | Demonstrates domain knowledge while remaining human-readable |
| 7 | EOD cutoff handling | On-demand endpoint + injectable clock | Deterministic testing of time-dependent behavior without schedulers |
| 8 | Omnibus account lookup | Static config with 3 correspondents | Matches spec's "client config" language; parameterizes business rules |
| 9 | Duplicate detection | Composite key (routing + account + check# + amount) in 30-day window | Industry standard, clearly distinct from vendor-layer detection |
| 10 | Test/demo strategy | Shell demo script + Go test suite + Makefile targets | Reviewer sees everything in `make demo`; Go tests provide rigor |

### Round 2 — Implementation

| # | Question | Winning Strategy | Key Rationale |
|---|---|---|---|
| 11 | REST API design | Resource-oriented with sub-resources | Endpoint names trace the deposit lifecycle; `/api/v1/` prefix separates from UI |
| 12 | Error handling | Structured errors with domain codes | `VENDOR.IQA_BLUR` is assertable in tests, actionable in UI, traceable in logs |
| 13 | Image storage | File system with DB references | `http.FileServer` serves images; synthetic PNGs generated for seed data |
| 14 | Session validation | Static API key per investor | Satisfies spec's "validate session" with minimal auth overhead; enables test scenarios |
| 15 | Amount representation | Integer cents (`int64`) | Zero rounding errors; `==` comparison for reconciliation; industry standard |
| 16 | Audit / decision trace | Unified `deposit_events` table | One table serves both operator audit trail and per-deposit decision trace |
| 17 | Return notifications | Synchronous in single DB transaction | All-or-nothing reversal; notification as event record, not external dispatch |
| 18 | Project structure | Domain-oriented packages | Directory tree mirrors spec's service boundaries; architecture visible from `ls` |
| 19 | Configuration | Env vars (infra) + YAML (domain) | Startup validation; defaults for zero-config `make dev`; YAML as documentation |
| 20 | Concurrency safety | `BEGIN IMMEDIATE` transactions | Atomic read-check-write; no double-posting; no application-level locks needed |

### Round 3 — Operations & Polish

| # | Question | Winning Strategy | Key Rationale |
|---|---|---|---|
| 21 | Data seeding | Programmatic seed function with scenario data | Reviewer sees a populated system on first `make dev` |
| 22 | Demo narrative | Narrated scenario runner with acts and pass/fail | Demo reads like a story; output is the submission report |
| 23 | Test architecture | Integration tests (real DB) + targeted unit tests | 18 tests covering all paths; ledger invariant as crown jewel |
| 24 | Pipeline orchestration | Step function pipeline with typed actions | Each step testable; decision trace built into the runner |
| 25 | Observability | `slog` structured logging + correlation IDs | `jq` query per deposit; settlement monitoring included |
| 26 | Makefile design | Complete workflow with help, dev, test, demo, report | `make help` is the project's command palette |
| 27 | Schema migrations | Embedded forward-only migrations with version table | Server self-migrates on startup; no external tools |
| 28 | Contribution types | Default-and-override per account type | Demonstrates domain knowledge; operator override in UI |
| 29 | Risk scoring | Composite score from 4 weighted signals | Turns operator queue into intelligent triage tool |
| 30 | Decision log | ADR format — 10 records mapping to rubric categories | Directly maximizes the 20-point system design score |

---

## Three-Round Master Summary

### Round 1 — Foundations
| Decision | Winner |
|---|---|
| Language | Go single binary |
| Data store | SQLite double-entry (transfers + ledger_entries) |
| Vendor stub mechanism | Magic account prefixes + header override |
| Operator UI | Embedded single-page web UI |
| State machine | Hardcoded transition map |
| Settlement format | Structured JSON mirroring X9 hierarchy |
| EOD cutoff | On-demand endpoint + injectable clock |
| Omnibus lookup | Static config with 3 correspondents |
| Duplicate detection | Composite key in 30-day window |
| Test/demo strategy | Shell demo script + Go test suite |

### Round 2 — Implementation
| Decision | Winner |
|---|---|
| REST API design | Resource-oriented with sub-resources |
| Error handling | Structured domain error codes |
| Image storage | File system with DB references |
| Session validation | Static API key per investor |
| Amount representation | Integer cents (int64) |
| Audit / decision trace | Unified deposit_events table |
| Return notifications | Synchronous in single DB transaction |
| Project structure | Domain-oriented packages |
| Configuration | Env vars (infra) + YAML (domain) |
| Concurrency safety | BEGIN IMMEDIATE transactions |

### Round 3 — Operations & Polish
| Decision | Winner |
|---|---|
| Data seeding | Programmatic seed with scenario data |
| Demo narrative | Narrated scenario runner with acts |
| Test architecture | Integration + unit (18 tests) |
| Pipeline orchestration | Step function pipeline |
| Observability | slog + correlation IDs |
| Makefile design | Complete workflow Makefile |
| Schema migrations | Embedded forward-only migrations |
| Contribution types | Default-and-override pattern |
| Risk scoring | Composite 4-signal model |
| Decision log | ADR format (10 records) |

### The Stack at a Glance

```
                        30 DECISIONS → 1 COHERENT SYSTEM

     ┌─────────────────────────────────────────────────────────────┐
     │                     Go Single Binary                        │
     │                                                             │
     │  Language: Go          Packages: Domain-oriented            │
     │  Setup: make dev       Config: .env + YAML                  │
     │  DB: SQLite            Migrations: Embedded, forward-only   │
     │  Currency: int64 cents Concurrency: BEGIN IMMEDIATE         │
     │                                                             │
     │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
     │  │ Vendor   │  │ Funding  │  │ Operator │  │Settlement│  │
     │  │ Stub     │──│ Service  │──│ Review   │──│ Engine   │  │
     │  │          │  │          │  │          │  │          │  │
     │  │• Magic   │  │• Rules   │  │• Web UI  │  │• X9 JSON │  │
     │  │  prefixes│  │• Ledger  │  │• Risk    │  │• Clock   │  │
     │  │• 7 scen. │  │• D.E.    │  │  scoring │  │  inject  │  │
     │  └──────────┘  └──────────┘  └──────────┘  └──────────┘  │
     │                                                             │
     │  Pipeline: Step functions     Errors: Domain codes          │
     │  Audit: deposit_events        Logs: slog + correlation ID   │
     │  Images: File system          Auth: Static API keys         │
     │  Returns: Sync + atomic       Seed: Programmatic scenarios  │
     │  Risk: 4-signal composite     Contrib: Default + override   │
     │                                                             │
     │  Tests: 18 (integration + unit)                             │
     │  Demo: Narrated 6-act script                                │
     │  Docs: 10 ADRs + architecture + README                     │
     │  Makefile: help, dev, test, demo, report, docker, clean    │
     └─────────────────────────────────────────────────────────────┘
```

