package funding

import (
	"testing"
	"time"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
)

// testEngine returns an engine with CORR-APEX (500000 limit, OMNI-APEX-001, INDIVIDUAL) and investors per PRD fixtures.
func testEngine(t *testing.T, dup DuplicateChecker) *Engine {
	t.Helper()
	cfg := &config.Config{
		Correspondents: []config.Correspondent{
			{ID: "CORR-APEX", DepositLimitCents: 500000, OmnibusAccountID: "OMNI-APEX-001", DefaultContributionType: "INDIVIDUAL"},
			{ID: "CORR-BETA", DepositLimitCents: 300000, OmnibusAccountID: "OMNI-BETA-001", DefaultContributionType: "EMPLOYER"},
		},
		Investors: []config.Investor{
			{AccountID: "PASS-10001", APIKey: "tok_alice_001", CorrespondentID: "CORR-APEX", Eligible: true, AccountType: "IRA"},
			{AccountID: "DUP-30001", APIKey: "tok_dave_004", CorrespondentID: "CORR-APEX", Eligible: false, AccountType: "standard"},
		},
	}
	return NewEngine(cfg, dup)
}

func TestEngine_MissingAPIKey_401Body(t *testing.T) {
	e := testEngine(t, nil)
	_, err := e.ValidateDeposit("", 100, "", "", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*domain.DomainError)
	if !ok || de.Code() != domain.CodeFundingAccountNotFound {
		t.Fatalf("want ACCOUNT_NOT_FOUND, got %v", err)
	}
}

func TestEngine_InvalidAPIKey(t *testing.T) {
	e := testEngine(t, nil)
	_, err := e.ValidateDeposit("bad", 100, "", "", "", "")
	de, _ := err.(*domain.DomainError)
	if de.Code() != domain.CodeFundingAccountNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestEngine_IneligibleDaveWilson(t *testing.T) {
	e := testEngine(t, nil)
	_, err := e.ValidateDeposit("tok_dave_004", 100, "", "", "", "")
	de, ok := err.(*domain.DomainError)
	if !ok || de.Code() != domain.CodeFundingIneligible {
		t.Fatalf("want INELIGIBLE, got %v", err)
	}
}

func TestEngine_OverLimit6000(t *testing.T) {
	e := testEngine(t, nil)
	_, err := e.ValidateDeposit("tok_alice_001", 600000, "", "", "", "")
	de, ok := err.(*domain.DomainError)
	if !ok || de.Code() != domain.CodeFundingOverLimit {
		t.Fatalf("want OVER_LIMIT, got %v", err)
	}
}

func TestEngine_DuplicateWithin30Days(t *testing.T) {
	mem := NewMemoryDuplicateChecker()
	mem.Record("021000021", "12345", "1001", 5000)
	e := testEngine(t, mem)
	_, err := e.ValidateDeposit("tok_alice_001", 5000, "021000021", "12345", "1001", "")
	de, ok := err.(*domain.DomainError)
	if !ok || de.Code() != domain.CodeFundingDuplicate {
		t.Fatalf("want DUPLICATE, got %v", err)
	}
}

func TestEngine_IRAGetsIndividual(t *testing.T) {
	e := testEngine(t, nil)
	ctx, err := e.ValidateDeposit("tok_alice_001", 10000, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ContributionType != "INDIVIDUAL" {
		t.Fatalf("want INDIVIDUAL, got %q", ctx.ContributionType)
	}
}

func TestEngine_OmnibusApex(t *testing.T) {
	e := testEngine(t, nil)
	ctx, err := e.ValidateDeposit("tok_alice_001", 10000, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.OmnibusID != "OMNI-APEX-001" {
		t.Fatalf("want OMNI-APEX-001, got %q", ctx.OmnibusID)
	}
}

func TestEngine_UnderLimitOK(t *testing.T) {
	e := testEngine(t, nil)
	ctx, err := e.ValidateDeposit("tok_alice_001", 500000, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil {
		t.Fatal("nil context")
	}
}

func TestMemoryDuplicate_OldRecordNotDuplicate(t *testing.T) {
	mem := NewMemoryDuplicateChecker()
	mem.RecordAt("r", "a", "c", 1, time.Now().Add(-40*24*time.Hour))
	ok, _ := mem.Exists("r", "a", "c", 1, time.Now().Add(-30*24*time.Hour))
	if ok {
		t.Fatal("should not duplicate outside window")
	}
}
