#!/usr/bin/env bash
# demo.sh — Narrated 6-act demo for Apex Mobile Check Deposit System
# Usage: ./scripts/demo.sh [BASE_URL]
# Requires: curl, jq

set -uo pipefail

BASE="${1:-http://localhost:8080}"
API="$BASE/api/v1"
KEY="tok_alice_001"
AUTH="Authorization: Bearer $KEY"
PASS=0
FAIL=0
TOTAL=0

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# --- Helpers ---
check_jq() {
    if ! command -v jq &>/dev/null; then
        echo -e "${RED}ERROR: jq is required but not installed.${NC}"
        echo "  macOS:  brew install jq"
        echo "  Ubuntu: sudo apt-get install jq"
        echo "  Other:  https://stedolan.github.io/jq/download/"
        exit 1
    fi
}

act() {
    echo ""
    echo -e "${CYAN}${BOLD}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  ACT $1: $2${NC}"
    echo -e "${CYAN}${BOLD}═══════════════════════════════════════════════════════════${NC}"
}

scene() {
    echo ""
    echo -e "${YELLOW}  ▸ $1${NC}"
}

assert_status() {
    local label="$1" expected="$2" actual="$3"
    TOTAL=$((TOTAL + 1))
    if [ "$expected" = "$actual" ]; then
        echo -e "    ${GREEN}✓ PASS${NC} — $label (HTTP $actual)"
        PASS=$((PASS + 1))
    else
        echo -e "    ${RED}✗ FAIL${NC} — $label (want $expected, got $actual)"
        FAIL=$((FAIL + 1))
    fi
}

assert_field() {
    local label="$1" field="$2" expected="$3" body="$4"
    TOTAL=$((TOTAL + 1))
    actual=$(echo "$body" | jq -r "$field" 2>/dev/null || echo "PARSE_ERROR")
    if [ "$expected" = "$actual" ]; then
        echo -e "    ${GREEN}✓ PASS${NC} — $label = $actual"
        PASS=$((PASS + 1))
    else
        echo -e "    ${RED}✗ FAIL${NC} — $label: want $expected, got $actual"
        FAIL=$((FAIL + 1))
    fi
}

# --- Preflight ---
check_jq

echo -e "${BOLD}Apex Mobile Check Deposit — Demo Script${NC}"
echo -e "Target: $BASE"
echo ""

# Check server is running
if ! curl -s "$BASE/health" >/dev/null 2>&1; then
    echo -e "${RED}ERROR: Server not reachable at $BASE${NC}"
    echo "Start the server first: make dev"
    exit 1
fi
echo -e "${GREEN}Server is running.${NC}"

# ═══════════════════════════════════════════════════════════
# ACT 1: Happy Path
# ═══════════════════════════════════════════════════════════
act 1 "Happy Path — Clean Deposit to FundsPosted"

scene "Submit a \$125.00 deposit for PASS-10001 (clean pass)"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/deposits" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"account_id":"PASS-10001","amount_cents":12500}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "POST /deposits" "201" "$HTTP"
assert_field "status" ".status" "FundsPosted" "$BODY"
TID=$(echo "$BODY" | jq -r '.transfer_id')

scene "Retrieve deposit detail"
RESP=$(curl -s -w "\n%{http_code}" "$API/deposits/$TID" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "GET /deposits/$TID" "200" "$HTTP"
assert_field "status" ".status" "FundsPosted" "$BODY"

scene "Check investor balance"
RESP=$(curl -s -w "\n%{http_code}" "$API/accounts/PASS-10001/balance" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "GET /accounts/balance" "200" "$HTTP"

# ═══════════════════════════════════════════════════════════
# ACT 2: Vendor Rejections
# ═══════════════════════════════════════════════════════════
act 2 "Vendor Rejections — IQA and Duplicate Failures"

scene "Submit BLUR deposit (IQA blur failure)"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/deposits" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"account_id":"BLUR-20001","amount_cents":5000}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "BLUR → 422 Rejected" "422" "$HTTP"

scene "Submit GLARE deposit (IQA glare failure)"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/deposits" \
    -H "Authorization: Bearer tok_frank_006" -H "Content-Type: application/json" \
    -d '{"account_id":"GLARE-20002","amount_cents":5000}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "GLARE → 422 Rejected" "422" "$HTTP"

# ═══════════════════════════════════════════════════════════
# ACT 3: Business Rules
# ═══════════════════════════════════════════════════════════
act 3 "Business Rules — Limits and Eligibility"

scene "Submit \$6,000 deposit (over \$5,000 limit)"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/deposits" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"account_id":"PASS-10001","amount_cents":600000}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "Over limit → 422 Rejected" "422" "$HTTP"
assert_field "status" ".status" "Rejected" "$BODY"

scene "Submit MICR failure deposit (flagged for review)"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/deposits" \
    -H "Authorization: Bearer tok_carol_003" -H "Content-Type: application/json" \
    -d '{"account_id":"MICR-10001","amount_cents":25000}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "MICR → 201 Flagged" "201" "$HTTP"
assert_field "status" ".status" "Analyzing" "$BODY"
FLAGGED_TID=$(echo "$BODY" | jq -r '.transfer_id')

# ═══════════════════════════════════════════════════════════
# ACT 4: Operator Review
# ═══════════════════════════════════════════════════════════
act 4 "Operator Review — Approve and Reject"

scene "List operator queue"
RESP=$(curl -s -w "\n%{http_code}" "$API/operator/queue" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "GET /operator/queue" "200" "$HTTP"

scene "Operator approves flagged deposit"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/operator/queue/$FLAGGED_TID/approve" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"operator_id":"demo-op"}')
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "Approve → 200" "200" "$HTTP"
assert_field "status" ".status" "FundsPosted" "$BODY"

# ═══════════════════════════════════════════════════════════
# ACT 5: Settlement
# ═══════════════════════════════════════════════════════════
act 5 "Settlement — Batch Generation"

scene "Generate settlement batch"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/settlement/batches" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "POST /settlement/batches" "201" "$HTTP"
BATCH_ID=$(echo "$BODY" | jq -r '.batch_id')

if [ "$BATCH_ID" != "null" ] && [ -n "$BATCH_ID" ]; then
    scene "Retrieve settlement file"
    RESP=$(curl -s -w "\n%{http_code}" "$API/settlement/batches/$BATCH_ID" -H "$AUTH")
    HTTP=$(echo "$RESP" | tail -1)
    BODY=$(echo "$RESP" | sed '$d')
    assert_status "GET /settlement/batches/$BATCH_ID" "200" "$HTTP"

    scene "List batch items"
    RESP=$(curl -s -w "\n%{http_code}" "$API/settlement/batches/$BATCH_ID/items" -H "$AUTH")
    HTTP=$(echo "$RESP" | tail -1)
    assert_status "GET /settlement/batches/$BATCH_ID/items" "200" "$HTTP"
fi

scene "Verify no double-batching"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/settlement/batches" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "Second batch → 200 (empty)" "200" "$HTTP"
assert_field "message" ".message" "no deposits to batch" "$BODY"

# ═══════════════════════════════════════════════════════════
# ACT 6: Return/Reversal
# ═══════════════════════════════════════════════════════════
act 6 "Return/Reversal — NSF Check Return"

scene "Process return on deposit $TID"
RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/returns" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d "{\"transfer_id\":\"$TID\",\"reason\":\"NSF\"}")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "POST /returns" "200" "$HTTP"
assert_field "status" ".status" "Returned" "$BODY"
assert_field "fee_cents" ".fee_cents" "3000" "$BODY"

scene "Verify event trail includes INVESTOR_NOTIFIED"
RESP=$(curl -s -w "\n%{http_code}" "$API/deposits/$TID/history" -H "$AUTH")
HTTP=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_status "GET /deposits/$TID/history" "200" "$HTTP"
TOTAL=$((TOTAL + 1))
if echo "$BODY" | jq -e '.[] | select(.EventType=="INVESTOR_NOTIFIED")' >/dev/null 2>&1; then
    echo -e "    ${GREEN}✓ PASS${NC} — INVESTOR_NOTIFIED event present"
    PASS=$((PASS + 1))
else
    echo -e "    ${RED}✗ FAIL${NC} — INVESTOR_NOTIFIED event missing"
    FAIL=$((FAIL + 1))
fi

# ═══════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════
echo ""
echo -e "${BOLD}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}  DEMO SUMMARY${NC}"
echo -e "${BOLD}═══════════════════════════════════════════════════════════${NC}"
echo -e "  Total:  $TOTAL"
echo -e "  ${GREEN}Passed: $PASS${NC}"
if [ "$FAIL" -gt 0 ]; then
    echo -e "  ${RED}Failed: $FAIL${NC}"
else
    echo -e "  Failed: 0"
fi
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All scenarios passed!${NC}"
else
    echo -e "${RED}${BOLD}$FAIL scenario(s) failed.${NC}"
    exit 1
fi
