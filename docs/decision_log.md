# Architecture Decision Log

10 Architecture Decision Records (ADRs) covering the major design choices in the Apex Mobile Check Deposit System.

---

## ADR-001: Language — Go

**Context:** The PRD constrains language choice to Go or Java. The system requires a single deployable binary with embedded web assets, low-latency HTTP handling, and straightforward concurrency for future scaling.

**Decision:** Go, using the standard library (`net/http`, `embed`, `database/sql`) with minimal third-party dependencies (only `mattn/go-sqlite3` for the database driver and `joho/godotenv` for env loading).

**Alternatives:**
- Java with Spring Boot — heavier runtime, WAR/JAR packaging, more complex embedding of static assets.
- Go with a framework (Gin, Echo) — adds dependency surface for marginal routing convenience.

**Consequences:**
- Single static binary with zero runtime dependencies.
- `go:embed` eliminates the need for a separate asset server or Docker multi-stage build.
- CGO required for SQLite driver (trade-off: cross-compilation needs C toolchain).
- Standard library HTTP mux is sufficient for the route count; no framework lock-in.

---

## ADR-002: Data Store — SQLite with WAL

**Context:** The system needs ACID transactions for financial correctness (double-entry ledger invariant), migration tracking, and persistent audit trails. MVP runs on a single machine.

**Decision:** SQLite in WAL (Write-Ahead Logging) mode with `MaxOpenConns=1` and `busy_timeout=5000ms`. All write transactions use `BEGIN IMMEDIATE`.

**Alternatives:**
- PostgreSQL — production-grade but adds infrastructure dependency; overkill for single-machine MVP.
- In-memory maps — fast but no persistence, no transaction isolation, no crash recovery.
- Embedded key-value store (BoltDB, BadgerDB) — no SQL, harder to express relational queries and joins.

**Consequences:**
- Zero infrastructure: database is a single file (`data/apex.db`).
- WAL allows concurrent reads while writes are serialized — appropriate for MVP throughput.
- `BEGIN IMMEDIATE` acquires write lock at transaction start, preventing TOCTOU races in financial operations.
- Migration path to PostgreSQL is straightforward: standard SQL schema, `database/sql` interface.

---

## ADR-003: Vendor Stub — Deterministic Scenario Engine

**Context:** The PRD requires a vendor integration stub that simulates 7 check validation outcomes (clean pass, IQA blur, IQA glare, MICR failure, duplicate, amount mismatch, and header override). Results must be repeatable for testing and demos.

**Decision:** Deterministic scenario selection via magic account ID prefixes (`PASS-`, `BLUR-`, `GLARE-`, `MICR-`, `DUP-`, `MISMATCH-`). Optional `X-Vendor-Scenario` header overrides the prefix for ad-hoc testing.

**Alternatives:**
- Random/probabilistic outcomes — non-deterministic, impossible to write reliable tests or demos.
- Configuration file mapping account IDs to outcomes — more flexible but harder to understand and demo.
- Separate mock service — adds deployment complexity for no benefit in an MVP.

**Consequences:**
- Any account ID immediately communicates its expected outcome: `BLUR-20001` always fails with IQA blur.
- Tests and demos are fully reproducible without external state.
- MICR data (routing, account, check number) is generated deterministically from the account ID, enabling predictable duplicate detection testing.
- Header override allows testing any scenario against any account without changing configuration.

---

## ADR-004: Settlement — X9 ICL-Structured JSON

**Context:** Real settlement files use the X9.37 ICL (Image Cash Letter) binary format. The PRD requires "X9-style" settlement output that demonstrates understanding of the structure without implementing binary encoding.

**Decision:** JSON files mirroring the X9 ICL hierarchy: `file_header` → `cash_letters[]` → `bundles[]` → `check_details[]` → `file_control`. Settlement date uses 6:30 PM CT EOD cutoff with next-business-day rollover (weekends only).

**Alternatives:**
- Raw binary X9.37 — complex encoding, hard to inspect, no practical benefit for an MVP.
- Flat CSV/TSV — loses the hierarchical structure that demonstrates X9 understanding.
- No settlement output — misses a key evaluation criterion.

**Consequences:**
- File structure is human-readable and inspectable via standard JSON tools.
- `file_control.total_amount` equals the sum of all check detail amounts (reconciliation invariant).
- Double-batching is prevented by assigning `settlement_batch_id` to each transfer within the same database transaction.
- Injectable clock (`as_of` query parameter) enables testing cutoff logic without waiting for real time.

---

## ADR-005: State Machine — 8 Explicit States

**Context:** A deposit moves through multiple processing stages. The PRD defines a lifecycle from submission through settlement, with branches for rejection, operator review, and post-settlement returns.

**Decision:** 8 states with a hardcoded transition map: `Requested → Validating → Analyzing → Approved → FundsPosted → Completed → Returned | Rejected`. The `Approved` state is always persisted, even for auto-approved deposits.

**Alternatives:**
- Fewer states (e.g., merge Validating/Analyzing) — loses auditability of where a deposit is in the pipeline.
- Event-sourced state derivation — more complex, harder to query current state efficiently.
- State machine library — unnecessary abstraction for a fixed, well-understood lifecycle.

**Consequences:**
- `domain.Transition(from, to)` validates every state change; invalid transitions return `STATE.INVALID_TRANSITION`.
- Always persisting `Approved` ensures a complete audit trail — no phantom states.
- Terminal states (`Rejected`, `Returned`) cannot transition further; enforced at the domain level.
- Every transition is logged to `deposit_events` with actor attribution (system, operator, or return processor).

---

## ADR-006: Currency — Integer Cents

**Context:** Financial amounts must avoid floating-point rounding errors. The system handles deposit amounts, ledger entries, return fees, and settlement totals.

**Decision:** All monetary values are stored and transmitted as `int64` cents. The `domain.Amount` type provides `ToDollars()` for display and `ParseAmount()` for string-to-cents conversion (rejects fractional cents).

**Alternatives:**
- `float64` — rounding errors accumulate; unacceptable for financial calculations.
- `math/big.Rat` — precise but verbose API; overkill when cent-level precision suffices.
- Decimal library (`shopspring/decimal`) — adds a dependency for marginal benefit over integer cents.

**Consequences:**
- `$150.00` is stored as `15000`; `$30.00` fee is `3000`. No rounding ambiguity.
- `ParseAmount("150.001")` returns an error — fractional cents are rejected at the boundary.
- Ledger invariant (`SUM(debits) == SUM(credits)`) is trivially verifiable with integer arithmetic.
- JSON API uses `amount_cents` field name to make the representation explicit to consumers.

---

## ADR-007: Risk Scoring — Threshold-Based MVP

**Context:** The operator review queue needs a mechanism to flag deposits that require human review. The PRD mentions risk scoring and confidence-based routing.

**Decision:** Single-signal MVP: vendor confidence score drives risk assessment. Confidence ≥ 0.9 → risk score 0 (LOW, auto-approve). Confidence < 0.9 → risk score 65 (CRITICAL, flag for operator review).

**Alternatives:**
- Multi-factor scoring (amount, account age, velocity, MICR quality) — more realistic but scope creep for MVP.
- ML-based scoring — explicitly excluded by PRD constraint C-6 ("no AI/ML frameworks").
- No risk scoring (all deposits auto-approved) — misses the operator workflow evaluation criterion.

**Consequences:**
- MICR failures (confidence 0.42) and amount mismatches are automatically flagged for review.
- Clean passes (confidence 0.98) flow straight through to FundsPosted.
- Risk score is stored on the transfer and displayed in the operator queue (sorted highest-first).
- The scoring function is isolated in `pipeline/risk.go` — easy to enhance without changing the pipeline.

---

## ADR-008: API — RESTful JSON with Structured Errors

**Context:** The system needs a consistent API surface for deposits, operator actions, settlement, returns, and account queries. Error responses must be machine-parseable.

**Decision:** RESTful JSON API under `/api/v1/` using Go's standard `net/http` mux. All error responses follow a uniform schema: `{code, message, transfer_id, details}`. Auth via Bearer token in the `Authorization` header.

**Alternatives:**
- gRPC — better for service-to-service; worse for browser/curl demos.
- GraphQL — flexible queries but adds complexity; fixed queries suffice for this domain.
- Framework router (gorilla/mux, chi) — marginal benefit for this route count; adds dependency.

**Consequences:**
- Domain error codes (e.g., `FUNDING.OVER_LIMIT`, `STATE.INVALID_TRANSITION`) appear in the `code` field — clients can switch on codes without parsing messages.
- Standard HTTP status codes: 201 (created), 200 (success), 401 (auth), 403 (forbidden), 404 (not found), 409 (conflict), 422 (validation), 500 (internal).
- Bearer token auth is simple and sufficient for MVP; maps directly to investor configuration in YAML.
- No versioned URL path beyond `/v1/` — single version is appropriate for MVP scope.

---

## ADR-009: Concurrency — Single-Writer Serialization

**Context:** Financial operations (ledger posting, return processing, settlement batching) require atomicity and isolation. The system may receive concurrent API requests.

**Decision:** SQLite single-writer model with `MaxOpenConns=1`. All write operations use `BEGIN IMMEDIATE` transactions. WAL mode allows concurrent reads.

**Alternatives:**
- Optimistic locking with version columns — more complex, still needs retry logic.
- Application-level mutex — error-prone, doesn't survive process restarts.
- PostgreSQL with row-level locking — production-grade but adds infrastructure.

**Consequences:**
- `BEGIN IMMEDIATE` acquires the write lock at transaction start, not at first write statement. This prevents TOCTOU races where two transactions read the same state and both try to write.
- Return processing reads transfer state and creates 4 ledger entries in a single transaction — no partial writes possible.
- Settlement batching queries unbatched transfers and assigns batch IDs atomically — no double-batching.
- Trade-off: write throughput is limited to one transaction at a time. Acceptable for MVP; PostgreSQL migration path is documented.

---

## ADR-010: Pipeline — Step-Function Orchestrator

**Context:** A deposit submission triggers multiple sequential operations: vendor validation, funding rules, risk scoring, and ledger posting. Each step may halt the pipeline (reject or flag for review).

**Decision:** Three-step pipeline runner in `internal/pipeline/runner.go`. Each step returns an action (`HaltRejected`, `HaltFlagged`, `HaltApproved`) that determines whether the pipeline continues. Every step persists state transitions and events before returning.

**Alternatives:**
- Event-driven saga with message queue — appropriate for distributed systems, overkill for single-process MVP.
- Monolithic handler — all logic in one function; harder to test, extend, and audit.
- Middleware chain — better for cross-cutting concerns (auth, logging) than sequential business steps.

**Consequences:**
- Each step is independently testable: vendor validation can be tested without funding rules.
- Pipeline halts are explicit: `HaltRejected` (terminal), `HaltFlagged` (needs operator), `HaltApproved` (continue to ledger).
- State transitions are persisted after each step — if the process crashes mid-pipeline, the deposit is in a valid intermediate state (e.g., `Validating`), not a phantom state.
- Operator approve reuses the same action/ledger posting logic as auto-approve, ensuring consistent behavior.
