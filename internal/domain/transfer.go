package domain

import "time"

// Transfer is the domain model for a deposit lifecycle (mirrors transfers table; PRD §7.2).
// Used by TICKET-004+ as the object passed through the pipeline.
type Transfer struct {
	ID                   string
	InvestorAccountID    string
	CorrespondentID      string
	Amount               Amount // int64 cents
	Status               State
	VendorTransactionID  string
	CheckNumber          string
	MICRRouting          string
	MICRAccount          string
	MICRData             string // JSON in DB; keep as string at domain layer until unmarshaled
	RiskScore            *int   // nil if not yet scored
	ContributionType     string
	SettlementBatchID    string // empty until batched
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
