package domain

// Error codes for API and logging (PRD §7.7). Constants are the machine-readable codes.
const (
	// VENDOR — vendor stub / image validation
	CodeVendorIQABlur         = "VENDOR.IQA_BLUR"
	CodeVendorIQAGlare        = "VENDOR.IQA_GLARE"
	CodeVendorMICRFailure     = "VENDOR.MICR_FAILURE"
	CodeVendorDuplicate       = "VENDOR.DUPLICATE"
	CodeVendorAmountMismatch  = "VENDOR.AMOUNT_MISMATCH"
	// FUNDING — business rules / account resolution
	CodeFundingOverLimit      = "FUNDING.OVER_LIMIT"
	CodeFundingDuplicate      = "FUNDING.DUPLICATE"
	CodeFundingAccountNotFound = "FUNDING.ACCOUNT_NOT_FOUND"
	CodeFundingIneligible      = "FUNDING.INELIGIBLE"
	// STATE — lifecycle / transitions
	CodeInvalidTransition = "STATE.INVALID_TRANSITION"
	// SETTLEMENT
	CodeSettlementCutoffPassed = "SETTLEMENT.CUTOFF_PASSED"
	// SYSTEM
	CodeSystemInternal = "SYSTEM.INTERNAL"
)

// DomainError is an error with a machine-readable code.
type DomainError struct {
	ErrCode string
	Message string
}

func (e *DomainError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.ErrCode
}

// Code returns the machine-readable error code (e.g. STATE.INVALID_TRANSITION).
func (e *DomainError) Code() string {
	return e.ErrCode
}

// NewDomainError builds a DomainError with the given code and optional message.
func NewDomainError(code, message string) *DomainError {
	return &DomainError{ErrCode: code, Message: message}
}

// ErrInvalidTransition is returned when a state transition is not allowed.
var ErrInvalidTransition = &DomainError{
	ErrCode: CodeInvalidTransition,
	Message: "requested state transition not allowed",
}
