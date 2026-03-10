package vendor

// HeaderScenario is the optional override header (PRD TICKET-004).
const HeaderScenario = "X-Vendor-Scenario"

// Outcome values returned by the stub.
const (
	OutcomeCleanPass       = "CLEAN_PASS"
	OutcomeIQAFailBlur     = "IQA_FAIL_BLUR"
	OutcomeIQAFailGlare    = "IQA_FAIL_GLARE"
	OutcomeMICRReadFailure = "MICR_READ_FAILURE"
	OutcomeDuplicate       = "DUPLICATE_DETECTED"
	OutcomeAmountMismatch  = "AMOUNT_MISMATCH"
)

// ScenarioHeader values for X-Vendor-Scenario override.
const (
	ScenarioCleanPass       = "CLEAN_PASS"
	ScenarioIQABlur         = "IQA_BLUR"
	ScenarioIQAGlare        = "IQA_GLARE"
	ScenarioMICRFailure     = "MICR_FAILURE"
	ScenarioDuplicate       = "DUPLICATE"
	ScenarioAmountMismatch = "AMOUNT_MISMATCH"
)

// Request is the input to the vendor stub (synthetic deposit validation).
type Request struct {
	AccountID   string // e.g. PASS-10001 — prefix selects scenario
	AmountCents int64  // user-entered amount; mismatch scenario returns different OCR amount
}

// Response is the stub result (machine-readable for pipeline and tests).
type Response struct {
	Outcome             string  `json:"outcome"`
	ErrorCode           string  `json:"error_code,omitempty"`
	Confidence          float64 `json:"confidence"`
	VendorTransactionID string  `json:"vendor_transaction_id"`
	MICRRouting         string  `json:"micr_routing,omitempty"`
	MICRAccount         string  `json:"micr_account,omitempty"`
	CheckNumber         string  `json:"check_number,omitempty"`
	OCRAmountCents      int64   `json:"ocr_amount_cents,omitempty"` // set when mismatch
	Message             string  `json:"message,omitempty"`
}
