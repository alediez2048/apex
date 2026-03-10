package domain

import "testing"

func TestTransferModel(t *testing.T) {
	var tr Transfer
	tr.Status = StateRequested
	tr.Amount = 15000
	tr.InvestorAccountID = "PASS-10001"
	tr.CorrespondentID = "CORR-APEX"
	if tr.Status != StateRequested {
		t.Errorf("Status = %v", tr.Status)
	}
	if tr.Amount != 15000 {
		t.Errorf("Amount = %v", tr.Amount)
	}
}

func TestReturnFeeCents(t *testing.T) {
	if ReturnFeeCents != 3000 {
		t.Errorf("ReturnFeeCents = %v, want 3000", ReturnFeeCents)
	}
}
