# Apex Mobile Check Deposit — 5-Minute Demo

**Format:** Screen recording (browser + terminal side by side)
**Pre-roll:** Server already running via `make dev`

---

## [0:00–0:30] One-Command Setup

**Terminal:**

```bash
make clean && make dev
```

> "Apex is a single Go binary — no Docker, no Node, no external services. `make dev` compiles, seeds demo data, and starts the server. SQLite database, embedded web UI, everything in one executable."

**Point at terminal output:** migrations applied, 8 seed scenarios populated, server listening.

---

## [0:30–1:30] Web UI — Submit + Operator Review

**Browser → http://localhost:8080**

> "Dashboard shows live deposit counts by state — pulled from the database, not hardcoded."

**Click Submit → enter `PASS-10001`, `12500` → Submit**

> "That deposit just ran through the full pipeline: vendor validation, funding rules, risk scoring, double-entry ledger posting. Status: FundsPosted."

**Click Submit → enter `MICR-10001`, `25000` → Submit**

> "This one has an unreadable MICR line. Vendor confidence was 0.42, so risk scoring flagged it for operator review."

**Click Operator → Approve the flagged deposit**

> "Operator queue sorted by risk score. Approve posts the ledger entries and logs the operator's identity. Click into a transfer to see the full decision trace — every state transition with actor and timestamp."

**Click View Detail on any transfer — show event trail**

---

## [1:30–2:30] Vendor Stub + Business Rules

**Terminal:**

```bash
curl -s localhost:8080/api/v1/deposits -H "Authorization: Bearer tok_alice_001" \
  -H "Content-Type: application/json" \
  -d '{"account_id":"BLUR-20001","amount_cents":5000}' | jq .
```

> "BLUR prefix triggers IQA blur rejection — 422. The vendor stub has 7 deterministic scenarios driven by account prefix. Fully reproducible, no randomness."

```bash
curl -s localhost:8080/api/v1/deposits -H "Authorization: Bearer tok_alice_001" \
  -H "Content-Type: application/json" \
  -d '{"account_id":"PASS-10001","amount_cents":600000}' | jq .
```

> "Six thousand dollars against a five thousand dollar limit — FUNDING.OVER_LIMIT. We also enforce 30-day duplicate detection on MICR routing, account, check number, and amount."

---

## [2:30–3:30] Settlement + Returns

**Terminal:**

```bash
curl -s -X POST localhost:8080/api/v1/settlement/batches \
  -H "Authorization: Bearer tok_alice_001" | jq .
```

> "Settlement batches all FundsPosted deposits into an X9 ICL-structured JSON file. Batch creation and ID assignment happen in the same transaction — double-batching is impossible."

```bash
curl -s -X POST localhost:8080/api/v1/returns -H "Authorization: Bearer tok_alice_001" \
  -H "Content-Type: application/json" \
  -d '{"transfer_id":"seed-001","reason":"NSF"}' | jq .
```

> "Return processing in a single transaction: reversal pair, thirty-dollar fee pair — four ledger entries total. Status moves to Returned. The ledger invariant holds: debits equal credits."

---

## [3:30–4:30] Test Suite

**Terminal:**

```bash
make demo-full
```

> "This builds a fresh server, runs the 6-act demo script, and shuts down. 24 assertions covering happy path, rejections, operator review, settlement, and returns — all passing."

> "We also have 92 unit and integration tests covering the PRD's 20 named scenarios. `make test` runs them all."

---

## [4:30–5:00] Wrap-Up

> "To recap — this is a complete deposit processing system: submission through settlement, operator review, return handling, and full financial auditability."

> "Single Go binary. SQLite with write-ahead logging. Double-entry ledger with balanced invariants. 92 tests. 10 architecture decision records. All documented in README, SUBMISSION.md, and the docs folder."

> "Thanks for watching."
