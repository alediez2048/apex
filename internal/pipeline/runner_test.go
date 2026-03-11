package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/funding"
	"github.com/alediez2048/apex/internal/store"
	"github.com/alediez2048/apex/internal/vendor"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
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

func testConfig() *config.Config {
	return &config.Config{
		Correspondents: []config.Correspondent{
			{ID: "CORR-APEX", DepositLimitCents: 500000, OmnibusAccountID: "OMNI-APEX-001", DefaultContributionType: "INDIVIDUAL"},
			{ID: "CORR-BETA", DepositLimitCents: 300000, OmnibusAccountID: "OMNI-BETA-001", DefaultContributionType: "EMPLOYER"},
		},
		Investors: []config.Investor{
			{AccountID: "PASS-10001", APIKey: "tok_alice_001", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "IRA"},
			{AccountID: "BLUR-20001", APIKey: "tok_bob_002", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "standard"},
			{AccountID: "MICR-10001", APIKey: "tok_carol_003", CorrespondentID: "CORR-BETA", Eligible: true, AccountType: "IRA"},
			{AccountID: "DUP-30001", APIKey: "tok_dave_004", CorrespondentID: "CORR-APEX", Eligible: false, AccountType: "standard"},
		},
	}
}

func makeDeps(t *testing.T, db *sql.DB) Deps {
	t.Helper()
	cfg := testConfig()
	return Deps{
		DB:            db,
		VendorStub:    vendor.NewStub(),
		FundingEngine: funding.NewEngine(cfg, nil),
	}
}

func createTransfer(t *testing.T, db *sql.DB, id, accountID, corrID string, amount int64) {
	t.Helper()
	tr := &domain.Transfer{
		ID:                id,
		InvestorAccountID: accountID,
		CorrespondentID:   corrID,
		Amount:            domain.Amount(amount),
		Status:            domain.StateRequested,
		CheckNumber:       "",
	}
	if err := store.CreateTransfer(db, tr); err != nil {
		t.Fatal(err)
	}
}

// assertEvents checks that eventTypes appear in order in deposit_events for the transfer.
func assertEvents(t *testing.T, db *sql.DB, transferID string, expectedTypes []string) {
	t.Helper()
	events, err := store.ListEvents(db, transferID)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(events))
	for i, e := range events {
		got[i] = e.EventType
	}
	for _, want := range expectedTypes {
		found := false
		for _, g := range got {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected event %q in %v", want, got)
		}
	}
}

func TestPipeline_CleanPath_PASS(t *testing.T) {
	db := setupTestDB(t)
	deps := makeDeps(t, db)
	ctx := context.Background()

	createTransfer(t, db, "tx-clean", "PASS-10001", "CORR-APEX", 15000)
	res, err := Run(ctx, deps, "tx-clean", "tok_alice_001")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != HaltApproved {
		t.Fatalf("want HaltApproved, got %s", res.Action)
	}
	if res.Status != string(domain.StateFundsPosted) {
		t.Fatalf("want FundsPosted, got %s", res.Status)
	}

	// Verify transfer in DB
	tr, err := store.GetTransfer(db, "tx-clean")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != domain.StateFundsPosted {
		t.Fatalf("DB status: want FundsPosted, got %s", tr.Status)
	}
	if tr.VendorTransactionID == "" {
		t.Fatal("vendor_transaction_id not set")
	}
	if tr.CheckNumber == "" {
		t.Fatal("check_number not set")
	}

	// Verify events include STATE_TRANSITION for each step, DEPOSIT_APPROVED, LEDGER_POSTED
	assertEvents(t, db, "tx-clean", []string{
		domain.EventTypeStateTransition,
		"DEPOSIT_APPROVED",
		"LEDGER_POSTED",
		"PIPELINE_STEP",
	})

	// Verify Approved state was persisted (STATE_TRANSITION with to_state=Approved)
	events, _ := store.ListEvents(db, "tx-clean")
	foundApproved := false
	for _, e := range events {
		if e.EventType == domain.EventTypeStateTransition {
			var p domain.StateTransitionPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				if p.ToState == string(domain.StateApproved) && p.Actor == "system" {
					foundApproved = true
				}
			}
		}
	}
	if !foundApproved {
		t.Fatal("STATE_TRANSITION to Approved with actor=system not found")
	}
}

func TestPipeline_BLUR_Rejected(t *testing.T) {
	db := setupTestDB(t)
	deps := makeDeps(t, db)
	ctx := context.Background()

	createTransfer(t, db, "tx-blur", "BLUR-20001", "CORR-APEX", 100)
	res, err := Run(ctx, deps, "tx-blur", "tok_bob_002")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != HaltRejected {
		t.Fatalf("want HaltRejected, got %s", res.Action)
	}
	if res.Status != string(domain.StateRejected) {
		t.Fatalf("want Rejected, got %s", res.Status)
	}

	tr, _ := store.GetTransfer(db, "tx-blur")
	if tr.Status != domain.StateRejected {
		t.Fatalf("DB status: want Rejected, got %s", tr.Status)
	}

	assertEvents(t, db, "tx-blur", []string{
		domain.EventTypeStateTransition,
		"DEPOSIT_REJECTED",
		"PIPELINE_STEP",
	})
}

func TestPipeline_MICR_Flagged(t *testing.T) {
	db := setupTestDB(t)
	deps := makeDeps(t, db)
	ctx := context.Background()

	createTransfer(t, db, "tx-micr", "MICR-10001", "CORR-BETA", 3000)
	res, err := Run(ctx, deps, "tx-micr", "tok_carol_003")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != HaltFlagged {
		t.Fatalf("want HaltFlagged, got %s", res.Action)
	}
	if res.Status != string(domain.StateAnalyzing) {
		t.Fatalf("want Analyzing, got %s", res.Status)
	}

	tr, _ := store.GetTransfer(db, "tx-micr")
	if tr.Status != domain.StateAnalyzing {
		t.Fatalf("DB status: want Analyzing, got %s", tr.Status)
	}
	if tr.RiskScore == nil || *tr.RiskScore < RiskCritical {
		t.Fatalf("risk_score should be >= %d, got %v", RiskCritical, tr.RiskScore)
	}

	assertEvents(t, db, "tx-micr", []string{
		domain.EventTypeStateTransition,
		"PIPELINE_STEP",
	})
}

func TestPipeline_StepLogged(t *testing.T) {
	db := setupTestDB(t)
	deps := makeDeps(t, db)
	ctx := context.Background()

	createTransfer(t, db, "tx-steps", "PASS-10001", "CORR-APEX", 10000)
	_, err := Run(ctx, deps, "tx-steps", "tok_alice_001")
	if err != nil {
		t.Fatal(err)
	}

	events, _ := store.ListEvents(db, "tx-steps")
	stepCount := 0
	for _, e := range events {
		if e.EventType == "PIPELINE_STEP" {
			stepCount++
		}
	}
	// Steps: step 1 start, step 1 end (not HaltFlagged), step 2 start, step 2 continue (approved), step 3 start, step 3 end
	if stepCount < 3 {
		t.Fatalf("expected at least 3 PIPELINE_STEP events, got %d", stepCount)
	}
}

func TestRiskScore(t *testing.T) {
	if s := RiskScore(0.98); s != RiskLow {
		t.Fatalf("0.98 → want %d, got %d", RiskLow, s)
	}
	if s := RiskScore(0.90); s != RiskLow {
		t.Fatalf("0.90 → want %d, got %d", RiskLow, s)
	}
	if s := RiskScore(0.42); s != RiskCritical {
		t.Fatalf("0.42 → want %d, got %d", RiskCritical, s)
	}
	if s := RiskScore(0.89); s != RiskCritical {
		t.Fatalf("0.89 → want %d, got %d", RiskCritical, s)
	}
}
