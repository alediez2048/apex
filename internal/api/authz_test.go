package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alediez2048/apex/internal/api"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/funding"
	"github.com/alediez2048/apex/internal/store"
)

// Authorization: who may do what. Investors act on their own account only;
// the exception queue, settlement, returns and the ops dashboard are
// operator-only, and the operator recorded on a decision comes from the token.

const opKey = "tok_ops_001"

func do(t *testing.T, h http.HandlerFunc, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestAuthz_InvestorCannotApproveOrReject(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	seedTransfer(t, db, "tx-flagged", "MICR-10001", "CORR-APEX", 5000, domain.StateAnalyzing)
	h := api.OperatorHandler(cfg, deps)

	for _, action := range []string{"approve", "reject"} {
		w := do(t, h, http.MethodPost, "/api/v1/operator/queue/tx-flagged/"+action, "tok_alice_001", `{"operator_id":"op1"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s as investor: want 403, got %d: %s", action, w.Code, w.Body.String())
		}
	}
	if w := do(t, h, http.MethodGet, "/api/v1/operator/queue", "tok_alice_001", ""); w.Code != http.StatusForbidden {
		t.Fatalf("queue as investor: want 403, got %d", w.Code)
	}

	tr, _ := store.GetTransfer(db, "tx-flagged")
	if tr.Status != domain.StateAnalyzing {
		t.Fatalf("transfer must be untouched, got %s", tr.Status)
	}
}

func TestAuthz_OperatorIdentityComesFromToken(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	seedTransfer(t, db, "tx-a", "PASS-10001", "CORR-APEX", 5000, domain.StateAnalyzing)
	seedTransfer(t, db, "tx-b", "PASS-10001", "CORR-APEX", 5000, domain.StateAnalyzing)
	h := api.OperatorHandler(cfg, deps)

	// Claiming to be someone else is refused, in the body or the header.
	if w := do(t, h, http.MethodPost, "/api/v1/operator/queue/tx-a/approve", opKey, `{"operator_id":"someone-else"}`); w.Code != http.StatusForbidden {
		t.Fatalf("spoofed body operator_id: want 403, got %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/tx-a/approve", nil)
	req.Header.Set("Authorization", "Bearer "+opKey)
	req.Header.Set("X-Operator-ID", "someone-else")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("spoofed X-Operator-ID: want 403, got %d", w.Code)
	}

	// With no operator_id at all, the token's operator is recorded.
	w = do(t, h, http.MethodPost, "/api/v1/operator/queue/tx-b/approve", opKey, "")
	if w.Code != http.StatusOK {
		t.Fatalf("approve: want 200, got %d: %s", w.Code, w.Body.String())
	}
	if m := parseBody(t, w.Result()); m["message"] != "approved by operator:op1" {
		t.Fatalf("want decision recorded for op1, got %v", m["message"])
	}
}

func TestAuthz_InvestorSeesOnlyOwnDeposits(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	seedTransfer(t, db, "tx-alice", "PASS-10001", "CORR-APEX", 5000, domain.StateRequested)
	seedTransfer(t, db, "tx-bob", "BLUR-20001", "CORR-APEX", 7000, domain.StateRequested)
	h := api.DepositsHandler(cfg, db, deps)

	// Unfiltered list is scoped to the caller's account.
	w := do(t, h, http.MethodGet, "/api/v1/deposits", "tok_alice_001", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["transfer_id"] != "tx-alice" {
		t.Fatalf("investor list must contain only their own deposit, got %v", list)
	}

	// Asking for another account is refused.
	if w := do(t, h, http.MethodGet, "/api/v1/deposits?account=BLUR-20001", "tok_alice_001", ""); w.Code != http.StatusForbidden {
		t.Fatalf("list other account: want 403, got %d", w.Code)
	}

	// Another account's transfer looks like it doesn't exist.
	for _, path := range []string{"/api/v1/deposits/tx-bob", "/api/v1/deposits/tx-bob/history"} {
		if w := do(t, h, http.MethodGet, path, "tok_alice_001", ""); w.Code != http.StatusNotFound {
			t.Fatalf("GET %s as another investor: want 404, got %d", path, w.Code)
		}
	}
	if w := do(t, h, http.MethodGet, "/api/v1/deposits/tx-alice", "tok_alice_001", ""); w.Code != http.StatusOK {
		t.Fatalf("own transfer: want 200, got %d", w.Code)
	}
}

func TestAuthz_OperatorSeesEveryAccount(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	seedTransfer(t, db, "tx-alice", "PASS-10001", "CORR-APEX", 5000, domain.StateRequested)
	seedTransfer(t, db, "tx-bob", "BLUR-20001", "CORR-APEX", 7000, domain.StateRequested)
	h := api.DepositsHandler(cfg, db, deps)

	w := do(t, h, http.MethodGet, "/api/v1/deposits", opKey, "")
	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(list) != 2 {
		t.Fatalf("operator list: want 200 with 2 deposits, got %d with %d", w.Code, len(list))
	}
	if w := do(t, h, http.MethodGet, "/api/v1/deposits/tx-bob", opKey, ""); w.Code != http.StatusOK {
		t.Fatalf("operator get: want 200, got %d", w.Code)
	}
}

func TestAuthz_InvestorCannotDepositIntoAnotherAccount(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	h := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, h, "BLUR-20001", 5000, "tok_alice_001")
	if code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %v", code, m)
	}
	n, _ := store.ListTransfers(db, store.TransferFilters{})
	if len(n) != 0 {
		t.Fatalf("no transfer may be created, got %d", len(n))
	}
}

func TestAuthz_AccountsScopedToOwner(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	h := api.AccountsHandler(cfg, db)

	if w := do(t, h, http.MethodGet, "/api/v1/accounts/BLUR-20001/balance", "tok_alice_001", ""); w.Code != http.StatusForbidden {
		t.Fatalf("other account balance: want 403, got %d", w.Code)
	}
	if w := do(t, h, http.MethodGet, "/api/v1/accounts/PASS-10001/balance", "tok_alice_001", ""); w.Code != http.StatusOK {
		t.Fatalf("own balance: want 200, got %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, h, http.MethodGet, "/api/v1/accounts/BLUR-20001/ledger", opKey, ""); w.Code != http.StatusOK {
		t.Fatalf("operator ledger: want 200, got %d", w.Code)
	}
}

func TestAuthz_BackOfficeRoutesAreOperatorOnly(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	cases := []struct {
		name   string
		h      http.HandlerFunc
		method string
		path   string
	}{
		{"settlement", api.SettlementHandler(cfg, db), http.MethodPost, "/api/v1/settlement/batches"},
		{"returns", api.ReturnsHandler(cfg, db), http.MethodPost, "/api/v1/returns"},
		{"stats", api.StatsHandler(cfg, db), http.MethodGet, "/api/v1/stats/dashboard"},
	}
	for _, c := range cases {
		if w := do(t, c.h, c.method, c.path, "tok_alice_001", `{}`); w.Code != http.StatusForbidden {
			t.Fatalf("%s as investor: want 403, got %d", c.name, w.Code)
		}
	}
	if w := do(t, api.StatsHandler(cfg, db), http.MethodGet, "/api/v1/stats/dashboard", opKey, ""); w.Code != http.StatusOK {
		t.Fatalf("stats as operator: want 200, got %d", w.Code)
	}
}

func TestValidateRoute_ReturnsResult(t *testing.T) {
	cfg := testCfg()
	h := funding.Handler(funding.NewEngine(cfg, nil))

	w := do(t, h, http.MethodPost, "/api/v1/deposits/validate", "tok_alice_001", `{"amount_cents":5000}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["ok"] != true || m["account_id"] != "PASS-10001" {
		t.Fatalf("want ok result for PASS-10001, got %v", m)
	}
}
