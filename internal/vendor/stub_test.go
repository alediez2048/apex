package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alediez2048/apex/internal/domain"
)

func TestStub_PASS(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "PASS-10001", AmountCents: 15000}, "")
	if r.Outcome != OutcomeCleanPass {
		t.Errorf("outcome = %s", r.Outcome)
	}
	if r.Confidence < 0.95 {
		t.Errorf("confidence = %v want >= 0.95", r.Confidence)
	}
	if r.MICRRouting == "" || r.VendorTransactionID == "" {
		t.Errorf("missing MICR or txn id: %+v", r)
	}
}

func TestStub_BLUR(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "BLUR-20001", AmountCents: 100}, "")
	if r.Outcome != OutcomeIQAFailBlur || r.ErrorCode != domain.CodeVendorIQABlur {
		t.Errorf("blur: %+v", r)
	}
}

func TestStub_GLARE(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "GLARE-20002", AmountCents: 100}, "")
	if r.Outcome != OutcomeIQAFailGlare || r.ErrorCode != domain.CodeVendorIQAGlare {
		t.Errorf("glare: %+v", r)
	}
}

func TestStub_MICR(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "MICR-10001", AmountCents: 100}, "")
	if r.Outcome != OutcomeMICRReadFailure || r.ErrorCode != domain.CodeVendorMICRFailure {
		t.Errorf("micr: %+v", r)
	}
	if r.Confidence != 0.42 {
		t.Errorf("confidence = %v want 0.42", r.Confidence)
	}
}

func TestStub_DUP(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "DUP-30001", AmountCents: 100}, "")
	if r.Outcome != OutcomeDuplicate || r.ErrorCode != domain.CodeVendorDuplicate {
		t.Errorf("dup: %+v", r)
	}
}

func TestStub_MISMATCH(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "MISMATCH-10001", AmountCents: 15000}, "")
	if r.Outcome != OutcomeAmountMismatch || r.ErrorCode != domain.CodeVendorAmountMismatch {
		t.Errorf("mismatch: %+v", r)
	}
	if r.OCRAmountCents == 15000 {
		t.Errorf("OCR amount should differ from entered 15000, got %d", r.OCRAmountCents)
	}
}

func TestStub_HeaderOverride(t *testing.T) {
	s := NewStub()
	r := s.Validate(Request{AccountID: "PASS-10001", AmountCents: 100}, ScenarioIQABlur)
	if r.Outcome != OutcomeIQAFailBlur {
		t.Errorf("header override failed: %+v", r)
	}
	r2 := s.Validate(Request{AccountID: "BLUR-20001", AmountCents: 15000}, ScenarioCleanPass)
	if r2.Outcome != OutcomeCleanPass || r2.Confidence < 0.95 {
		t.Errorf("header override to clean: %+v", r2)
	}
}

func TestEnsureStubImages(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureStubImages(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stub_front.png", "stub_back.png"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}
