# TICKET-009: Operator Web UI — Implementation Plan

## Overview

Build an embedded single-page web UI (vanilla HTML/CSS/JS) compiled into the Go binary via `go:embed`. Four main views: **dashboard** (`/`), **deposit submission** (`/submit`), **operator review queue** (`/operator`), and **transfer detail** (`/transfers/:id`). Dashboard shows live benchmark KPIs; operator queue shows flagged deposits with risk scores, check images, MICR data, and approve/reject actions with optional contribution type override.

**Size:** L (1–2 days) | **Depends on:** TICKET-008 (APIs in place)

---

## Scope (from PRD)

- **Pages:** dashboard, `/submit`, `/operator`, `/transfers/:id`
- **Dashboard:** benchmark cards for gating correctness, settlement reconciliation, vendor scenario coverage, queue latency, return accuracy, deposit counts by state; values from live system (or test/demo artifacts), refresh on page load and after key actions
- **Operator queue:** flagged deposits sorted by risk score (highest first); each item: risk badge, amount, account, check images, MICR data; Approve / Reject buttons; contribution type dropdown (INDIVIDUAL, EMPLOYER, ROLLOVER)
- **Transfer detail:** full decision trace from `deposit_events` (inputs, vendor response, business rules, operator actions, settlement status)
- **Tech:** vanilla HTML/CSS/JS, `go:embed`, no front-end framework

---

## Current State

- **APIs (TICKET-008):** All needed endpoints exist:
  - `GET /api/v1/deposits` (list, filters: status, account, from, to)
  - `GET /api/v1/deposits/{id}`, `GET /api/v1/deposits/{id}/history`, `GET /api/v1/deposits/{id}/images/front|back`
  - `POST /api/v1/deposits` (create)
  - `GET /api/v1/operator/queue` (list; default status=Analyzing)
  - `POST /api/v1/operator/queue/{id}/approve` (body: `operator_id`, optional; header: `X-Operator-ID`)
  - `POST /api/v1/operator/queue/{id}/reject` (body: `operator_id`, `reason`)
- **Auth:** API uses Bearer token (investor API keys). Operator endpoints accept same Bearer; operator ID via `X-Operator-ID` or body.
- **Images:** Served at `/api/v1/deposits/{id}/images/front` and `.../back` (stub fallback if per-transfer image missing).
- **No `web/` directory yet;** root `/` currently returns plain text. No `go:embed` in use.

---

## Design

### 1. Embedding and routing

- Add `web/` directory: `web/index.html`, `web/submit.html`, `web/operator.html`, `web/transfers.html` (or a single `index.html` SPA with client-side routing).
- **Option A (SPA):** One `index.html` + JS that shows/hides sections or updates content based on path (`/`, `/submit`, `/operator`, `/transfers/xyz`). Simpler embedding (one entry).
- **Option B (multi-page):** Separate HTML files; server serves the right file for each path. Cleaner URLs and smaller initial load per page.
- **Recommendation:** SPA in one `index.html` with hash or path-based routing (e.g. `#/operator`, `#/transfers/tx-123`) to avoid conflicting with existing `/health` and `/api/v1/*`. Alternatively use a dedicated prefix (e.g. `/app`, `/ui`) and serve the SPA there so `/` can remain the dashboard at `/` or move dashboard to `/app` and redirect `/` → `/app`.

- **Serve static assets:** `go:embed` an `embed.FS` for `web/`; register a handler that serves `index.html` for app routes and static files (CSS, JS, images) by name. Use `fs.FS` and `http.FS()` for safe embedding.

### 2. API access from the browser

- All API calls from the UI need a **Bearer token**. Options:
  - **Demo/MVP:** Use a single configured “UI API key” (e.g. first investor’s key or a dedicated `ui_demo` key in config) and embed it in the served HTML/JS (e.g. a script tag with `window.API_KEY`). Simple but not for production.
  - **Login page:** Operator enters API key (and optional operator ID); store in `sessionStorage` and send on each request. Better for rubric “operator ID” attribution.
- **Recommendation for 009:** Config-driven “demo” API key (e.g. from env or config) injected into the page for development; operator ID entered in the operator UI (stored in sessionStorage) and sent as `X-Operator-ID` on approve/reject. Document that production would replace this with proper auth.

- **CORS:** Same-origin (UI and API on same port) so no CORS if UI is served from the same server.

### 3. Dashboard (benchmark cards)

- **KPIs (from PRD §3):** Gating correctness, Settlement reconciliation, Vendor scenario coverage, Operator queue response, Return/reversal accuracy, Test coverage, Setup time, Benchmark dashboard freshness.
- **Deposit counts by state:** Call `GET /api/v1/deposits` with no filters (or with `?status=` per state) and aggregate counts; or add a lightweight `GET /api/v1/stats/deposits-by-state` (or `/benchmark`) that returns counts and any precomputed metrics. Prefer one “stats” endpoint to avoid many list calls.
- **Implementation:** New optional endpoint `GET /api/v1/stats/dashboard` (or `/benchmark`) returning JSON: `deposits_by_state: { Requested: n, ... }`, optional placeholders for settlement reconciliation / return accuracy / queue latency (e.g. from DB or “N/A” until TICKET-010/011). Dashboard page fetches this on load and after “benchmark-affecting” actions (submit deposit, approve/reject, later: settlement, return). Refresh: either “Refresh” button or short polling (e.g. every 5s when tab visible).
- **Benchmark values “derived from live system”:** Use real `GET /api/v1/deposits` (and queue) for counts; other KPIs can show “Pass”/“N/A” from test run or config until TICKET-010/011 and TICKET-013.

### 4. Deposit submission form (`/submit`)

- Form fields: account_id (text or select from config), amount_cents (number).
- On submit: `POST /api/v1/deposits` with JSON body, Bearer token in header. Show success (transfer_id, status) or error (code, message).
- After success, optionally refresh dashboard and/or offer link to transfer detail or queue.

### 5. Operator queue (`/operator`)

- Fetch `GET /api/v1/operator/queue` (with optional `?account=`, `?from=`, `?to=`, `?status=`). Sort client-side by risk_score descending (API may already return sorted; if not, sort in JS).
- **Per item:** transfer_id, risk_score badge, amount, investor_account_id, check images (img src = `/api/v1/deposits/{id}/images/front` and `.../back` — need to pass auth; see below), MICR data (from transfer: micr_routing, micr_account, check_number).
- **Images with auth:** Browser requests to image URLs must include Bearer. Options: (1) Use fetch with credentials and same-origin; if API accepts a cookie, set token in cookie. (2) For same-origin, server could support a short-lived query param for image access (e.g. `?token=`) for GET only. (3) Or embed images as data URLs by fetching with fetch() and Bearer, then setting img src to blob URL or data URL. Prefer (3) for MVP: fetch images with fetch() + Authorization header, create object URL and set as img src.
- **Approve:** Button → POST `/api/v1/operator/queue/{id}/approve` with body `{ "operator_id": "<from sessionStorage>" }` and optional `contribution_type` (if API extended). On success, remove item from list or refresh queue.
- **Reject:** Button → prompt for reason → POST `.../reject` with `{ "operator_id": "...", "reason": "..." }`. On success, remove item or refresh.
- **Contribution type dropdown:** PRD requires INDIVIDUAL, EMPLOYER, ROLLOVER. If approve API does not yet accept contribution_type, extend it in 009: add optional `contribution_type` to approve body; in pipeline OperatorApprove, call UpdateTransferContribution before or after ledger post if provided.

### 6. Transfer detail (`/transfers/:id`)

- Fetch `GET /api/v1/deposits/{id}` and `GET /api/v1/deposits/{id}/history`. Render transfer fields and event list; for each event, show event_type, actor, created_at, and payload (formatted JSON or key fields). “Decision trace” = history as the ordered list of events (deposit inputs from transfer, vendor/business rules/operator/settlement from events).

### 7. Navigation and UX

- Global nav: links to Dashboard, Submit, Operator queue. From queue, link to transfer detail; from submit success, link to transfer or queue.
- Use minimal CSS (single stylesheet or inline critical styles) so the UI is readable and functional; no requirement for a design system.

### 8. File layout

- `web/index.html` — SPA shell (or dashboard); contains nav and placeholder divs for content.
- `web/app.js` — Routing, API helpers (auth header, base URL), dashboard fetch/render, submit form, queue fetch/render (with image fetch), transfer detail fetch/render, approve/reject handlers.
- `web/style.css` — Layout and typography for cards, queue list, forms.
- `internal/web` or `cmd/server` — Handler that serves embedded FS: for `/` (or `/app`) serve index.html; for `/app.js`, `/style.css` serve files; for `/operator`, `/submit`, `/transfers/*` serve index.html if SPA (so client router can run). Alternatively serve exact files and use multiple HTML files.
- New optional endpoint: `GET /api/v1/stats/dashboard` in `internal/api` or `cmd/server` returning counts and benchmark placeholders.

---

## Implementation Order

1. **Embed and serve static UI**
   - Create `web/` with `index.html`, `app.js`, `style.css`.
   - Add `//go:embed` in main (or `internal/web`) and register handler to serve embedded files; define route policy (e.g. `/` and `/submit`, `/operator`, `/transfers/*` serve index.html; `/app.js`, `/style.css` serve by name). Ensure `/api/v1` and `/health` still take precedence.

2. **Dashboard**
   - Add `GET /api/v1/stats/dashboard` (or similar) returning deposit counts by state (+ optional KPI placeholders). Implement in api or main.
   - In `app.js`, fetch dashboard API and render benchmark cards. Add “Refresh” and/or auto-refresh after 5s when visible.

3. **Deposit submission**
   - Form in HTML; on submit, POST to `/api/v1/deposits` with Bearer and body; show result or error. Optional: after success trigger dashboard refresh.

4. **Operator queue**
   - Fetch `GET /api/v1/operator/queue`; render list sorted by risk_score desc. Per item: risk badge, amount, account, MICR fields. Load check images via fetch with Bearer and set blob/object URL for img src. Implement Approve (with optional contribution_type) and Reject (prompt reason); call APIs and refresh list on success.

5. **Transfer detail**
   - Route `/transfers/:id`; fetch deposit + history; render transfer info and event list (decision trace).

6. **Contribution type on approve (if not present)**
   - Extend approve request body with optional `contribution_type`; in pipeline OperatorApprove, call store.UpdateTransferContribution when provided (before ledger post). Ensure dropdown in queue sends this value.

7. **Polish**
   - Ensure benchmark cards refresh after deposit submission, approve/reject (and later settlement/return when implemented). Document operator ID and API key handling for demo.

---

## Acceptance Criteria Mapping

| Criterion | Approach |
|-----------|----------|
| Opening `localhost:8080/` shows benchmark cards for gating correctness, settlement reconciliation, vendor scenario coverage, queue latency, return accuracy, deposit counts by state | Dashboard page + `/api/v1/stats/dashboard` (or equivalent) with counts and placeholders; cards rendered in UI. |
| Benchmark cards refresh after deposit submission, operator approval/rejection, settlement batch generation, and return processing | On submit/approve/reject success, re-fetch dashboard; optional polling; later wire settlement/return. |
| Benchmark values from live system / test artifacts, not hardcoded | Counts from API; other KPIs from config or “N/A”/“Pass” until tests/settlement/returns exist. |
| Opening `localhost:8080/operator` shows flagged deposits sorted by risk score (highest first) | Queue page; fetch queue API; sort by risk_score desc in JS (or API). |
| Each queue item: risk score badge, amount, account, check images, MICR data | Render from transfer; images via fetch+Bearer → blob URL. |
| Approve button posts to approve API and updates UI | POST approve with operator_id; on success remove item or refresh queue. |
| Reject button prompts for reason before posting | Confirm/prompt dialog; POST reject with reason. |
| Contribution type dropdown: INDIVIDUAL, EMPLOYER, ROLLOVER | Dropdown per item; send with approve if API extended. |
| Transfer detail shows full decision trace from deposit_events | Transfer detail page: deposit + history; render events with type, actor, payload, time. |

---

## Out of Scope / Later

- Full operator authentication (login page, session). MVP: config or sessionStorage for demo API key and operator ID.
- Settlement batch generation and return processing (TICKET-010, TICKET-011) — dashboard can show placeholders until those tickets exist.
- Automated “benchmark-affecting” test run to drive KPI values (TICKET-013); manual refresh or demo flow is enough for 009.

---

## Summary

- **New:** `web/` (index.html, app.js, style.css), embedded in binary; optional `GET /api/v1/stats/dashboard`; optional approve body field `contribution_type` and pipeline support.
- **Existing:** All operator and deposit APIs; reuse as-is except small approve extension.
- **Result:** Operator and dashboard UX in one binary, meeting PRD acceptance criteria for TICKET-009.
