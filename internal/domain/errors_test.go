package domain

import (
	"testing"
)

func TestErrorCodesDefined(t *testing.T) {
	codes := []string{
		CodeVendorIQABlur,
		CodeVendorIQAGlare,
		CodeVendorMICRFailure,
		CodeVendorDuplicate,
		CodeVendorAmountMismatch,
		CodeFundingOverLimit,
		CodeFundingDuplicate,
		CodeFundingAccountNotFound,
		CodeFundingIneligible,
		CodeInvalidTransition,
		CodeSettlementCutoffPassed,
		CodeSystemInternal,
	}
	for _, c := range codes {
		if c == "" {
			t.Fatal("empty code constant")
		}
	}
}

func TestNewDomainError(t *testing.T) {
	e := NewDomainError(CodeFundingOverLimit, "over limit")
	if e.Code() != CodeFundingOverLimit {
		t.Errorf("Code() = %q", e.Code())
	}
}
