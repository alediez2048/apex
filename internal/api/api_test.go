package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alediez2048/apex/internal/api"
	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/funding"
	"github.com/alediez2048/apex/internal/ledger"
	"github.com/alediez2048/apex/internal/pipeline"
	"github.com/alediez2048/apex/internal/store"
	"github.com/alediez2048/apex/internal/vendor"

	_ "github.com/mattn/go-sqlite3"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func testCfg() *config.Config {
	return &config.Config{
		Correspondents: []config.Correspondent{
			{ID: "CORR-APEX", DepositLimitCents: 500000, OmnibusAccountID: "OMNI-APEX-001", DefaultContributionType: "INDIVIDUAL"},
		},
		Investors: []config.Investor{
			{AccountID: "PASS-10001", APIKey: "tok_alice_001", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "IRA"},
			{AccountID: "BLUR-20001", APIKey: "tok_bob_002", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard"},
			{AccountID: "MICR-10001", APIKey: "tok_carol_003", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard"},
			{AccountID: "GLARE-20002", APIKey: "tok_frank_006", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard"},
			{AccountID: "DUP-30001", APIKey: "tok_dave_004", CorrespondentID: "CORR-APEX", Eligible: false, AccountType: "standard"},
			{AccountID: "PASS-99999", APIKey: "tok_suspended_007", CorrespondentID: "CORR-APEX", Eligible: false, AccountType: "standard"},
			{AccountID: "MISMATCH-10001", APIKey: "tok_eve_005", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard"},
		},
	}
}

func testDeps(t *testing.T, db *sql.DB) pipeline.Deps {
	t.Helper()
	cfg := testCfg()
	return pipeline.Deps{
		DB:            db,
		VendorStub:    vendor.NewStub(),
		FundingEngine: funding.NewEngine(cfg, nil),
	}
}

func seedTransfer(t *testing.T, db *sql.DB, id, account, corr string, amount int64, status domain.State) {
	t.Helper()
	tr := &domain.Transfer{
		ID:                id,
		InvestorAccountID: account,
		CorrespondentID:   corr,
		Amount:            domain.Amount(amount),
		Status:            domain.StateRequested,
		CheckNumber:       "",
	}
	if err := store.CreateTransfer(db, tr); err != nil {
		t.Fatal(err)
	}
	if status != domain.StateRequested {
		if err := store.UpdateTransferStatus(db, id, status); err != nil {
			t.Fatal(err)
		}
	}
}

func parseBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("failed to parse body: %s — %s", err, string(body))
	}
	return m
}

// --- Auth tests ---

func TestAuth_MissingToken(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	m := parseBody(t, w.Result())
	if m["code"] != "UNAUTHORIZED" {
		t.Fatalf("want UNAUTHORIZED code, got %v", m["code"])
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	req.Header.Set("Authorization", "Bearer bad_token_999")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// --- Error format tests ---

func TestErrorFormat_HasCodeAndMessage(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	m := parseBody(t, w.Result())
	if _, ok := m["code"]; !ok {
		t.Fatal("error body missing 'code'")
	}
	if _, ok := m["message"]; !ok {
		t.Fatal("error body missing 'message'")
	}
}

// --- Deposits: list ---

func TestDeposits_ListEmpty(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Result().Body)
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("want JSON array, got %s", string(body))
	}
	if len(arr) != 0 {
		t.Fatalf("want 0 items, got %d", len(arr))
	}
}

func TestDeposits_ListWithFilters(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-1", "PASS-10001", "CORR-APEX", 10000, domain.StateFundsPosted)
	seedTransfer(t, db, "tx-2", "PASS-10001", "CORR-APEX", 20000, domain.StateRejected)

	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits?status=FundsPosted", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	var arr []map[string]interface{}
	_ = json.Unmarshal(body, &arr)
	if len(arr) != 1 {
		t.Fatalf("want 1 item with status=FundsPosted, got %d", len(arr))
	}
}

// --- Deposits: get ---

func TestDeposits_GetByID(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-get", "PASS-10001", "CORR-APEX", 15000, domain.StateRequested)

	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits/tx-get", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := parseBody(t, w.Result())
	if m["transfer_id"] != "tx-get" {
		t.Fatalf("want transfer_id=tx-get, got %v", m["transfer_id"])
	}
}

func TestDeposits_GetNotFound(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits/tx-nonexistent", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// --- Deposits: history ---

func TestDeposits_History(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-hist", "PASS-10001", "CORR-APEX", 15000, domain.StateRequested)
	_ = store.InsertEvent(db, "tx-hist", "STATE_TRANSITION", "system", `{"from_state":"Requested","to_state":"Validating"}`)

	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits/tx-hist/history", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Result().Body)
	var arr []map[string]interface{}
	_ = json.Unmarshal(body, &arr)
	if len(arr) != 1 {
		t.Fatalf("want 1 event, got %d", len(arr))
	}
}

// --- Deposits: create (POST) ---

func TestDeposits_Create(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	payload := `{"account_id":"PASS-10001","amount_cents":15000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deposits", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — body: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["transfer_id"] == nil || m["transfer_id"] == "" {
		t.Fatal("missing transfer_id in response")
	}
	if m["status"] != string(domain.StateFundsPosted) {
		t.Fatalf("want status FundsPosted, got %v", m["status"])
	}
}

// --- Deposits: create BLUR → 422 Rejected ---

func TestDeposits_CreateBLUR_Rejected(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	payload := `{"account_id":"BLUR-20001","amount_cents":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deposits", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_bob_002")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d — body: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["status"] != string(domain.StateRejected) {
		t.Fatalf("want status Rejected, got %v", m["status"])
	}
	if m["action"] != "HaltRejected" {
		t.Fatalf("want action HaltRejected, got %v", m["action"])
	}
}

// --- Deposits: GET JSON shape ---

func TestDeposits_GetByID_JSONShape(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-shape", "PASS-10001", "CORR-APEX", 15000, domain.StateFundsPosted)

	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits/tx-shape", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := parseBody(t, w.Result())

	requiredFields := []string{"transfer_id", "investor_account_id", "correspondent_id", "amount_cents", "status", "created_at", "updated_at"}
	for _, f := range requiredFields {
		if _, ok := m[f]; !ok {
			t.Errorf("GET deposit response missing required field %q", f)
		}
	}
	if m["transfer_id"] != "tx-shape" {
		t.Errorf("want transfer_id=tx-shape, got %v", m["transfer_id"])
	}
	if m["status"] != "FundsPosted" {
		t.Errorf("want status=FundsPosted, got %v", m["status"])
	}
}

// --- Deposits: invalid date filter ---

func TestDeposits_ListInvalidDate(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits?from=garbage", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid date, got %d", w.Code)
	}
}

// --- Operator: queue ---

func TestOperator_QueueList(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-flag", "PASS-10001", "CORR-APEX", 5000, domain.StateAnalyzing)
	seedTransfer(t, db, "tx-done", "PASS-10001", "CORR-APEX", 3000, domain.StateFundsPosted)

	handler := api.OperatorHandler(cfg, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/operator/queue", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Result().Body)
	var arr []map[string]interface{}
	_ = json.Unmarshal(body, &arr)
	if len(arr) != 1 {
		t.Fatalf("want 1 flagged item, got %d", len(arr))
	}
}

// --- Operator: approve ---

func TestOperator_Approve(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-approve", "PASS-10001", "CORR-APEX", 5000, domain.StateAnalyzing)

	handler := api.OperatorHandler(cfg, deps)

	payload := `{"operator_id":"op1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/tx-approve/approve", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["status"] != string(domain.StateFundsPosted) {
		t.Fatalf("want FundsPosted, got %v", m["status"])
	}

	tr, _ := store.GetTransfer(db, "tx-approve")
	if tr.Status != domain.StateFundsPosted {
		t.Fatalf("DB status: want FundsPosted, got %s", tr.Status)
	}
}

// --- Operator: reject ---

func TestOperator_Reject(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-reject", "PASS-10001", "CORR-APEX", 5000, domain.StateAnalyzing)

	handler := api.OperatorHandler(cfg, deps)

	payload := `{"operator_id":"op2","reason":"suspicious check"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/tx-reject/reject", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", w.Code, w.Body.String())
	}

	tr, _ := store.GetTransfer(db, "tx-reject")
	if tr.Status != domain.StateRejected {
		t.Fatalf("DB status: want Rejected, got %s", tr.Status)
	}
}

// --- Operator: reject wrong state ---

func TestOperator_RejectWrongState(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	seedTransfer(t, db, "tx-bad", "PASS-10001", "CORR-APEX", 5000, domain.StateFundsPosted)

	handler := api.OperatorHandler(cfg, deps)

	payload := `{"operator_id":"op2","reason":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/tx-bad/reject", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

// --- Accounts: balance ---

func TestAccounts_Balance(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)

	ctx := context.Background()
	_ = ledger.Post(ctx, db, "tx-bal", "OMNI-APEX-001", "PASS-10001", 10000, "test")

	handler := api.AccountsHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/PASS-10001/balance", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := parseBody(t, w.Result())
	if m["account_id"] != "PASS-10001" {
		t.Fatalf("want PASS-10001, got %v", m["account_id"])
	}
	bal, ok := m["balance_cents"].(float64)
	if !ok || int64(bal) != 10000 {
		t.Fatalf("want balance_cents=10000, got %v", m["balance_cents"])
	}
}

// --- Accounts: ledger ---

func TestAccounts_Ledger(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)

	ctx := context.Background()
	_ = ledger.Post(ctx, db, "tx-led", "OMNI-APEX-001", "PASS-10001", 5000, "test")
	_ = ledger.Post(ctx, db, "tx-led2", "OMNI-APEX-001", "PASS-10001", 3000, "test2")

	handler := api.AccountsHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/PASS-10001/ledger", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Result().Body)
	var arr []map[string]interface{}
	_ = json.Unmarshal(body, &arr)
	if len(arr) != 2 {
		t.Fatalf("want 2 entries for PASS-10001 (2 CREDITs), got %d", len(arr))
	}
}

// --- Settlement (TICKET-010) ---

func TestSettlement_PostBatches_NoAuth_401(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestSettlement_PostBatches_NoDeposits_200(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 (no deposits to batch), got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["message"] != "no deposits to batch" {
		t.Fatalf("want message no deposits to batch, got %v", m["message"])
	}
}

func TestSettlement_PostBatches_CreatesBatch_NoDoubleBatch(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	seedTransfer(t, db, "tx-s1", "PASS-10001", "CORR-APEX", 15000, domain.StateFundsPosted)
	seedTransfer(t, db, "tx-s2", "PASS-10001", "CORR-APEX", 20000, domain.StateFundsPosted)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	batchID, _ := m["batch_id"].(string)
	if batchID == "" {
		t.Fatalf("want batch_id in response, got %v", m["batch_id"])
	}
	file, _ := m["file"].(map[string]interface{})
	if file == nil {
		t.Fatalf("want file in response")
	}
	fc, _ := file["file_control"].(map[string]interface{})
	if fc == nil {
		t.Fatalf("want file_control in file")
	}
	if amt, _ := fc["total_amount"].(float64); amt != 35000 {
		t.Fatalf("want total_amount 35000, got %v", fc["total_amount"])
	}

	// Second POST must not re-batch (no new batch or 0 items)
	w2 := httptest.NewRecorder()
	handler(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("want 200 (no deposits to batch) on second call, got %d", w2.Code)
	}
	m2 := parseBody(t, w2.Result())
	if m2["message"] != "no deposits to batch" {
		t.Fatalf("want no deposits to batch, got %v", m2["message"])
	}
}

func TestSettlement_GetBatch_404(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settlement/batches/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestSettlement_GetBatch_200(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	seedTransfer(t, db, "tx-g1", "PASS-10001", "CORR-APEX", 10000, domain.StateFundsPosted)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	postReq.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	wPost := httptest.NewRecorder()
	handler(wPost, postReq)
	if wPost.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", wPost.Code, wPost.Body.String())
	}
	m := parseBody(t, wPost.Result())
	batchID, _ := m["batch_id"].(string)
	if batchID == "" {
		t.Fatal("no batch_id in POST response")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settlement/batches/"+batchID, nil)
	getReq.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	wGet := httptest.NewRecorder()
	handler(wGet, getReq)
	if wGet.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", wGet.Code, wGet.Body.String())
	}
	body := wGet.Body.Bytes()
	var file map[string]interface{}
	if err := json.Unmarshal(body, &file); err != nil {
		t.Fatalf("GET response not JSON: %s", body)
	}
	if file["file_header"] == nil {
		t.Fatal("want file_header in GET batch response")
	}
	if file["file_control"] == nil {
		t.Fatal("want file_control in GET batch response")
	}
}

func TestSettlement_GetBatchItems_200(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.SettlementHandler(cfg, db)

	seedTransfer(t, db, "tx-i1", "PASS-10001", "CORR-APEX", 5000, domain.StateFundsPosted)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	postReq.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	wPost := httptest.NewRecorder()
	handler(wPost, postReq)
	if wPost.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", wPost.Code)
	}
	m := parseBody(t, wPost.Result())
	batchID, _ := m["batch_id"].(string)
	if batchID == "" {
		t.Fatal("no batch_id")
	}

	itemsReq := httptest.NewRequest(http.MethodGet, "/api/v1/settlement/batches/"+batchID+"/items", nil)
	itemsReq.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	wItems := httptest.NewRecorder()
	handler(wItems, itemsReq)
	if wItems.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", wItems.Code)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(wItems.Body.Bytes(), &list); err != nil {
		t.Fatal("items response not JSON array")
	}
	if len(list) != 1 {
		t.Fatalf("want 1 item, got %d", len(list))
	}
	if list[0]["transfer_id"] != "tx-i1" {
		t.Fatalf("want transfer_id tx-i1, got %v", list[0]["transfer_id"])
	}
}

// --- Returns (TICKET-011) ---

func TestReturns_FundsPosted_200(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-r1", "PASS-10001", "CORR-APEX", 15000, domain.StateFundsPosted)
	// Seed initial ledger posting (omnibus → investor) so there's something to reverse
	if err := ledger.Post(context.Background(), db, "tx-r1", "OMNI-APEX-001", "PASS-10001", 15000, "FREE"); err != nil {
		t.Fatal(err)
	}

	body := `{"transfer_id":"tx-r1","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["status"] != "Returned" {
		t.Fatalf("want status Returned, got %v", m["status"])
	}
	if m["fee_cents"] != float64(3000) {
		t.Fatalf("want fee_cents 3000, got %v", m["fee_cents"])
	}
	if m["reversal_amount_cents"] != float64(15000) {
		t.Fatalf("want reversal_amount_cents 15000, got %v", m["reversal_amount_cents"])
	}

	// Verify transfer is now Returned
	tr, _ := store.GetTransfer(db, "tx-r1")
	if tr.Status != domain.StateReturned {
		t.Fatalf("want transfer Returned, got %s", tr.Status)
	}

	// Verify 4 reversal ledger entries (+ 2 from initial posting = 6 total)
	entries, _ := store.ListLedgerEntriesByAccount(db, "PASS-10001")
	if len(entries) != 3 { // 1 CREDIT (initial) + 1 DEBIT (reversal) + 1 DEBIT (fee)
		t.Fatalf("want 3 entries for investor, got %d", len(entries))
	}

	// Verify events
	events, _ := store.ListEvents(db, "tx-r1")
	eventTypes := make(map[string]bool)
	for _, ev := range events {
		eventTypes[ev.EventType] = true
	}
	for _, want := range []string{"RETURN_RECEIVED", "REVERSAL_POSTED", "INVESTOR_NOTIFIED"} {
		if !eventTypes[want] {
			t.Fatalf("missing event %s", want)
		}
	}
}

func TestReturns_Completed_200(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-r2", "PASS-10001", "CORR-APEX", 20000, domain.StateCompleted)

	body := `{"transfer_id":"tx-r2","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["status"] != "Returned" {
		t.Fatalf("want Returned, got %v", m["status"])
	}
}

func TestReturns_InvalidState_409(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-r3", "PASS-10001", "CORR-APEX", 10000, domain.StateRequested)

	body := `{"transfer_id":"tx-r3","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["code"] != "STATE.INVALID_TRANSITION" {
		t.Fatalf("want STATE.INVALID_TRANSITION, got %v", m["code"])
	}
}

func TestReturns_DoubleReturn_409(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-r3b", "PASS-10001", "CORR-APEX", 5000, domain.StateFundsPosted)

	body := `{"transfer_id":"tx-r3b","reason":"NSF"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req1.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)

	w1 := httptest.NewRecorder()
	handler(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first return: want 200, got %d", w1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req2.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w2 := httptest.NewRecorder()
	handler(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("second return: want 409, got %d: %s", w2.Code, w2.Body.String())
	}
	m := parseBody(t, w2.Result())
	if m["code"] != "STATE.INVALID_TRANSITION" {
		t.Fatalf("want STATE.INVALID_TRANSITION, got %v", m["code"])
	}
}

func TestReturns_NotFound_404(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	body := `{"transfer_id":"tx-nonexistent","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReturns_LedgerInvariant(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-r4", "PASS-10001", "CORR-APEX", 25000, domain.StateFundsPosted)
	if err := ledger.Post(context.Background(), db, "tx-r4", "OMNI-APEX-001", "PASS-10001", 25000, "FREE"); err != nil {
		t.Fatal(err)
	}

	body := `{"transfer_id":"tx-r4","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+cfg.Investors[0].APIKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	// Verify global invariant: sum of all debits = sum of all credits
	var totalDebits, totalCredits int64
	rows, err := db.Query("SELECT entry_type, SUM(amount) FROM ledger_entries GROUP BY entry_type")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var entryType string
		var sum int64
		if err := rows.Scan(&entryType, &sum); err != nil {
			t.Fatal(err)
		}
		if entryType == "DEBIT" {
			totalDebits = sum
		} else {
			totalCredits = sum
		}
	}
	if totalDebits != totalCredits {
		t.Fatalf("ledger invariant violated: debits=%d credits=%d", totalDebits, totalCredits)
	}
}

// =============================================================================
// PRD Test Manifest (TICKET-013) — named integration tests per PRD §9 lines 720-741
// =============================================================================

// createDeposit is a test helper: POST /api/v1/deposits with given account/amount/token.
func createDeposit(t *testing.T, handler http.HandlerFunc, accountID string, amountCents int64, apiKey string) (int, map[string]interface{}) {
	t.Helper()
	payload, _ := json.Marshal(map[string]interface{}{"account_id": accountID, "amount_cents": amountCents})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deposits", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	m := parseBody(t, w.Result())
	return w.Code, m
}

// 1. TestIntegration_HappyPath — end-to-end deposit → FundsPosted → settlement
func TestIntegration_HappyPath(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	depositH := api.DepositsHandler(cfg, db, deps)
	settlementH := api.SettlementHandler(cfg, db)

	// Create deposit → FundsPosted
	code, m := createDeposit(t, depositH, "PASS-10001", 15000, "tok_alice_001")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	if m["status"] != "FundsPosted" {
		t.Fatalf("want FundsPosted, got %v", m["status"])
	}
	tid := m["transfer_id"].(string)

	// Settle
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	settlementH(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("settlement: want 201, got %d: %s", w.Code, w.Body.String())
	}
	sm := parseBody(t, w.Result())
	if sm["batch_id"] == nil || sm["batch_id"] == "" {
		t.Fatal("want batch_id in settlement response")
	}

	// Verify transfer has settlement_batch_id
	tr, _ := store.GetTransfer(db, tid)
	if tr.SettlementBatchID == "" {
		t.Fatal("want settlement_batch_id set on transfer")
	}
}

// 2. TestIntegration_VendorBlur — IQA blur → Rejected
func TestIntegration_VendorBlur(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, handler, "BLUR-20001", 5000, "tok_bob_002")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", code)
	}
	if m["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m["status"])
	}
}

// 3. TestIntegration_VendorGlare — IQA glare → Rejected
func TestIntegration_VendorGlare(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, handler, "GLARE-20002", 5000, "tok_frank_006")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", code)
	}
	if m["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m["status"])
	}
}

// 4. TestIntegration_VendorMICRFailure — MICR failure → Analyzing (flagged)
func TestIntegration_VendorMICRFailure(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, handler, "MICR-10001", 5000, "tok_carol_003")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	if m["status"] != "Analyzing" {
		t.Fatalf("want Analyzing (flagged), got %v", m["status"])
	}
	if m["action"] != "HaltFlagged" {
		t.Fatalf("want HaltFlagged, got %v", m["action"])
	}
}

// 5. TestIntegration_VendorDuplicate — vendor duplicate → Rejected
func TestIntegration_VendorDuplicate(t *testing.T) {
	cfg := testCfg()
	// Add a DUP-prefixed eligible account for this test
	cfg.Investors = append(cfg.Investors, config.Investor{
		AccountID: "DUP-99001", APIKey: "tok_dup_test", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard",
	})
	db := testDB(t)
	deps := pipeline.Deps{
		DB:            db,
		VendorStub:    vendor.NewStub(),
		FundingEngine: funding.NewEngine(cfg, nil),
	}
	handler := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, handler, "DUP-99001", 5000, "tok_dup_test")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", code)
	}
	if m["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m["status"])
	}
}

// 6. TestIntegration_VendorAmountMismatch — amount mismatch → Analyzing (flagged)
func TestIntegration_VendorAmountMismatch(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	code, m := createDeposit(t, handler, "MISMATCH-10001", 5000, "tok_eve_005")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	// MISMATCH triggers AMOUNT_MISMATCH outcome which is non-terminal → Analyzing
	if m["status"] != "Analyzing" {
		t.Fatalf("want Analyzing (flagged), got %v", m["status"])
	}
}

// 7. TestIntegration_OverDepositLimit — $6,000 vs $5,000 limit → Rejected
func TestIntegration_OverDepositLimit(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	// CORR-APEX limit is 500000 cents ($5,000). Submit $6,000 = 600000 cents.
	code, m := createDeposit(t, handler, "PASS-10001", 600000, "tok_alice_001")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", code)
	}
	if m["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m["status"])
	}
}

// 8. TestIntegration_FundingDuplicateDetection — same check twice → second rejected
func TestIntegration_FundingDuplicateDetection(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	dup := &funding.SQLDuplicateChecker{DB: db}
	deps := pipeline.Deps{
		DB:            db,
		VendorStub:    vendor.NewStub(),
		FundingEngine: funding.NewEngine(cfg, dup),
	}
	handler := api.DepositsHandler(cfg, db, deps)

	// First deposit succeeds
	code1, _ := createDeposit(t, handler, "PASS-10001", 15000, "tok_alice_001")
	if code1 != http.StatusCreated {
		t.Fatalf("first deposit: want 201, got %d", code1)
	}

	// Second deposit with same account (same MICR) → funding duplicate
	code2, m2 := createDeposit(t, handler, "PASS-10001", 15000, "tok_alice_001")
	if code2 != http.StatusUnprocessableEntity {
		t.Fatalf("second deposit: want 422, got %d", code2)
	}
	if m2["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m2["status"])
	}
}

// 9. TestIntegration_OperatorApprove — operator approves flagged → FundsPosted
func TestIntegration_OperatorApprove(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	// Create a flagged deposit via MICR failure
	depositH := api.DepositsHandler(cfg, db, deps)
	code, m := createDeposit(t, depositH, "MICR-10001", 10000, "tok_carol_003")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	tid := m["transfer_id"].(string)

	// Operator approves
	operatorH := api.OperatorHandler(cfg, deps)
	payload := `{"operator_id":"op1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/"+tid+"/approve", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	operatorH(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	tr, _ := store.GetTransfer(db, tid)
	if tr.Status != domain.StateFundsPosted {
		t.Fatalf("want FundsPosted, got %s", tr.Status)
	}
}

// 10. TestIntegration_OperatorReject — operator rejects flagged → Rejected
func TestIntegration_OperatorReject(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	depositH := api.DepositsHandler(cfg, db, deps)
	code, m := createDeposit(t, depositH, "MICR-10001", 10000, "tok_carol_003")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	tid := m["transfer_id"].(string)

	operatorH := api.OperatorHandler(cfg, deps)
	payload := `{"operator_id":"op2","reason":"suspicious"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/queue/"+tid+"/reject", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	operatorH(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	tr, _ := store.GetTransfer(db, tid)
	if tr.Status != domain.StateRejected {
		t.Fatalf("want Rejected, got %s", tr.Status)
	}
}

// 11. TestIntegration_ReturnAndReversal — return → reversal + $30 fee + Returned
func TestIntegration_ReturnAndReversal(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)

	// Create a deposit that reaches FundsPosted
	depositH := api.DepositsHandler(cfg, db, deps)
	code, m := createDeposit(t, depositH, "PASS-10001", 20000, "tok_alice_001")
	if code != http.StatusCreated {
		t.Fatalf("want 201, got %d", code)
	}
	tid := m["transfer_id"].(string)

	// Process return
	returnsH := api.ReturnsHandler(cfg, db)
	body := `{"transfer_id":"` + tid + `","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	returnsH(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	rm := parseBody(t, w.Result())
	if rm["status"] != "Returned" {
		t.Fatalf("want Returned, got %v", rm["status"])
	}
	if rm["fee_cents"] != float64(3000) {
		t.Fatalf("want fee_cents 3000, got %v", rm["fee_cents"])
	}
	if rm["reversal_amount_cents"] != float64(20000) {
		t.Fatalf("want reversal 20000, got %v", rm["reversal_amount_cents"])
	}
}

// 12. TestIntegration_SettlementFileContents — settlement file has correct X9 structure and totals
func TestIntegration_SettlementFileContents(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	depositH := api.DepositsHandler(cfg, db, deps)
	settlementH := api.SettlementHandler(cfg, db)

	// Create two deposits
	createDeposit(t, depositH, "PASS-10001", 10000, "tok_alice_001")
	createDeposit(t, depositH, "PASS-10001", 25000, "tok_alice_001")

	// Generate batch
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	settlementH(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	file, _ := m["file"].(map[string]interface{})
	if file == nil {
		t.Fatal("want file in response")
	}
	if file["file_header"] == nil {
		t.Fatal("want file_header")
	}
	fc, _ := file["file_control"].(map[string]interface{})
	if fc == nil {
		t.Fatal("want file_control")
	}
	if total, _ := fc["total_amount"].(float64); total != 35000 {
		t.Fatalf("want total_amount 35000, got %v", fc["total_amount"])
	}
	if count, _ := fc["item_count"].(float64); count != 2 {
		t.Fatalf("want item_count 2, got %v", fc["item_count"])
	}
}

// 13. TestIntegration_UnauthenticatedRequest — no token → 401
func TestIntegration_UnauthenticatedRequest(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deposits", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	m := parseBody(t, w.Result())
	if m["code"] != "UNAUTHORIZED" {
		t.Fatalf("want UNAUTHORIZED, got %v", m["code"])
	}
}

// 14. TestIntegration_IneligibleAccount — ineligible investor → 422 Rejected
func TestIntegration_IneligibleAccount(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	handler := api.DepositsHandler(cfg, db, deps)

	// PASS-99999 passes vendor validation (PASS- prefix) but has eligible=false
	code, m := createDeposit(t, handler, "PASS-99999", 5000, "tok_suspended_007")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", code)
	}
	if m["status"] != "Rejected" {
		t.Fatalf("want Rejected, got %v", m["status"])
	}
}

// 18. TestLedgerInvariant_DebitsEqualCredits — sum of all debits == sum of all credits
func TestLedgerInvariant_DebitsEqualCredits(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	depositH := api.DepositsHandler(cfg, db, deps)
	returnsH := api.ReturnsHandler(cfg, db)

	// Create deposits
	_, m1 := createDeposit(t, depositH, "PASS-10001", 15000, "tok_alice_001")
	_, m2 := createDeposit(t, depositH, "PASS-10001", 25000, "tok_alice_001")
	tid1 := m1["transfer_id"].(string)
	_ = m2["transfer_id"].(string)

	// Return one
	body := `{"transfer_id":"` + tid1 + `","reason":"NSF"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	returnsH(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("return: want 200, got %d", w.Code)
	}

	// Verify invariant
	var totalDebits, totalCredits int64
	rows, err := db.Query("SELECT entry_type, SUM(amount) FROM ledger_entries GROUP BY entry_type")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var entryType string
		var sum int64
		if err := rows.Scan(&entryType, &sum); err != nil {
			t.Fatal(err)
		}
		if entryType == "DEBIT" {
			totalDebits = sum
		} else {
			totalCredits = sum
		}
	}
	if totalDebits != totalCredits {
		t.Fatalf("ledger invariant violated: debits=%d credits=%d", totalDebits, totalCredits)
	}
}

// 19. TestSettlement_NoDoubleBatching — second batch returns zero items
func TestSettlement_NoDoubleBatching(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	deps := testDeps(t, db)
	depositH := api.DepositsHandler(cfg, db, deps)
	settlementH := api.SettlementHandler(cfg, db)

	createDeposit(t, depositH, "PASS-10001", 10000, "tok_alice_001")
	createDeposit(t, depositH, "PASS-10001", 20000, "tok_alice_001")

	// First batch
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlement/batches", nil)
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w1 := httptest.NewRecorder()
	settlementH(w1, req)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first batch: want 201, got %d", w1.Code)
	}

	// Second batch — no deposits to batch
	w2 := httptest.NewRecorder()
	settlementH(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("second batch: want 200, got %d", w2.Code)
	}
	m := parseBody(t, w2.Result())
	if m["message"] != "no deposits to batch" {
		t.Fatalf("want 'no deposits to batch', got %v", m["message"])
	}
}

// 20. TestReturnOnCompletedDeposit — late return on Completed deposit succeeds
func TestReturnOnCompletedDeposit(t *testing.T) {
	cfg := testCfg()
	db := testDB(t)
	handler := api.ReturnsHandler(cfg, db)

	seedTransfer(t, db, "tx-late", "PASS-10001", "CORR-APEX", 30000, domain.StateCompleted)
	if err := ledger.Post(context.Background(), db, "tx-late", "OMNI-APEX-001", "PASS-10001", 30000, "FREE"); err != nil {
		t.Fatal(err)
	}

	body := `{"transfer_id":"tx-late","reason":"UNAUTHORIZED"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/returns", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer tok_alice_001")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	m := parseBody(t, w.Result())
	if m["status"] != "Returned" {
		t.Fatalf("want Returned, got %v", m["status"])
	}
	if m["fee_cents"] != float64(3000) {
		t.Fatalf("want fee 3000, got %v", m["fee_cents"])
	}

	tr, _ := store.GetTransfer(db, "tx-late")
	if tr.Status != domain.StateReturned {
		t.Fatalf("want Returned in DB, got %s", tr.Status)
	}
}
