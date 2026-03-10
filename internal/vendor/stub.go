package vendor

import (
	"fmt"
	"strings"

	"github.com/alediez2048/apex/internal/domain"
)

// Stub is the configurable vendor stub (deterministic, no network).
type Stub struct{}

// NewStub returns a vendor stub.
func NewStub() *Stub { return &Stub{} }

// scenarioFromAccountPrefix maps account ID prefix to scenario (before header override).
func scenarioFromAccountPrefix(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ScenarioCleanPass
	}
	switch {
	case strings.HasPrefix(accountID, "PASS-"):
		return ScenarioCleanPass
	case strings.HasPrefix(accountID, "BLUR-"):
		return ScenarioIQABlur
	case strings.HasPrefix(accountID, "GLARE-"):
		return ScenarioIQAGlare
	case strings.HasPrefix(accountID, "MICR-"):
		return ScenarioMICRFailure
	case strings.HasPrefix(accountID, "DUP-"):
		return ScenarioDuplicate
	case strings.HasPrefix(accountID, "MISMATCH-"):
		return ScenarioAmountMismatch
	default:
		return ScenarioCleanPass // default IQA pass then MICR path
	}
}

// Validate applies header override then prefix routing and returns a deterministic response.
func (s *Stub) Validate(req Request, headerScenario string) Response {
	scenario := strings.TrimSpace(headerScenario)
	if scenario == "" {
		scenario = scenarioFromAccountPrefix(req.AccountID)
	}
	idSuffix := strings.ReplaceAll(req.AccountID, "-", "")
	if idSuffix == "" {
		idSuffix = "unknown"
	}
	txnID := fmt.Sprintf("vnd-%s-%d", idSuffix, req.AmountCents)

	switch scenario {
	case ScenarioIQABlur:
		return Response{
			Outcome:             OutcomeIQAFailBlur,
			ErrorCode:           domain.CodeVendorIQABlur,
			Confidence:          0.10,
			VendorTransactionID: txnID,
			Message:             "image quality: blur detected",
		}
	case ScenarioIQAGlare:
		return Response{
			Outcome:             OutcomeIQAFailGlare,
			ErrorCode:           domain.CodeVendorIQAGlare,
			Confidence:          0.12,
			VendorTransactionID: txnID,
			Message:             "image quality: glare detected",
		}
	case ScenarioMICRFailure:
		return Response{
			Outcome:             OutcomeMICRReadFailure,
			ErrorCode:           domain.CodeVendorMICRFailure,
			Confidence:          0.42,
			VendorTransactionID: txnID,
			MICRRouting:         "021000021",
			MICRAccount:         "999999999",
			CheckNumber:         "0001",
			Message:             "MICR line unreadable",
		}
	case ScenarioDuplicate:
		return Response{
			Outcome:             OutcomeDuplicate,
			ErrorCode:           domain.CodeVendorDuplicate,
			Confidence:          0.99,
			VendorTransactionID: txnID,
			Message:             "check previously deposited",
		}
	case ScenarioAmountMismatch:
		// OCR amount differs from entered (e.g. entered 15000, OCR reads 12500)
		ocrCents := req.AmountCents - 2500
		if ocrCents <= 0 {
			ocrCents = req.AmountCents + 100
		}
		return Response{
			Outcome:             OutcomeAmountMismatch,
			ErrorCode:           domain.CodeVendorAmountMismatch,
			Confidence:          0.88,
			VendorTransactionID: txnID,
			MICRRouting:         "021000021",
			MICRAccount:         "123456789",
			CheckNumber:         "1001",
			OCRAmountCents:      ocrCents,
			Message:             "OCR amount differs from entered amount",
		}
	case ScenarioCleanPass:
		fallthrough
	default:
		return Response{
			Outcome:             OutcomeCleanPass,
			Confidence:          0.98,
			VendorTransactionID: txnID,
			MICRRouting:         "021000021",
			MICRAccount:         strings.TrimPrefix(req.AccountID, "PASS-") + "001",
			CheckNumber:         "1001",
			Message:             "clean pass",
		}
	}
}
