package funding

import (
	"strings"
	"time"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
)

const duplicateWindow = 30 * 24 * time.Hour

// Engine validates API key, resolves accounts, and runs business rules (TICKET-005).
type Engine struct {
	cfg             *config.Config
	byAPIKey        map[string]config.Investor
	byCorrespondent map[string]config.Correspondent
	dup             DuplicateChecker
}

// NewEngine builds lookup maps from config. dup may be nil (duplicate check skipped).
func NewEngine(cfg *config.Config, dup DuplicateChecker) *Engine {
	byKey := make(map[string]config.Investor)
	for _, inv := range cfg.Investors {
		byKey[inv.APIKey] = inv
	}
	byCorr := make(map[string]config.Correspondent)
	for _, c := range cfg.Correspondents {
		byCorr[c.ID] = c
	}
	return &Engine{cfg: cfg, byAPIKey: byKey, byCorrespondent: byCorr, dup: dup}
}

// ValidateDeposit resolves investor by API key, checks eligibility, limit, duplicate, contribution type.
// micrRouting/micrAccount/checkNumber may be empty until MICR parsed; duplicate uses whatever is provided.
// currentTransferID, when non-empty, is excluded from the duplicate check (so the deposit being processed does not match itself).
func (e *Engine) ValidateDeposit(apiKey string, amountCents int64, micrRouting, micrAccount, checkNumber string, currentTransferID string) (*Context, error) {
	if apiKey == "" {
		return nil, domain.NewDomainError(domain.CodeFundingAccountNotFound, "missing API key")
	}
	inv, ok := e.byAPIKey[apiKey]
	if !ok {
		return nil, domain.NewDomainError(domain.CodeFundingAccountNotFound, "invalid API key")
	}
	if !inv.Eligible {
		return nil, domain.NewDomainError(domain.CodeFundingIneligible, "account not eligible for deposits")
	}
	corr, ok := e.byCorrespondent[inv.CorrespondentID]
	if !ok {
		return nil, domain.NewDomainError(domain.CodeFundingAccountNotFound, "correspondent not found")
	}
	if amountCents > corr.DepositLimitCents {
		return nil, domain.NewDomainError(domain.CodeFundingOverLimit, "deposit exceeds correspondent limit")
	}
	if e.dup != nil && checkNumber != "" {
		since := time.Now().Add(-duplicateWindow)
		var dup bool
		var err error
		if currentTransferID != "" {
			dup, err = e.dup.ExistsExcluding(micrRouting, micrAccount, checkNumber, amountCents, since, currentTransferID)
		} else {
			dup, err = e.dup.Exists(micrRouting, micrAccount, checkNumber, amountCents, since)
		}
		if err != nil {
			return nil, err
		}
		if dup {
			return nil, domain.NewDomainError(domain.CodeFundingDuplicate, "duplicate check within 30 days")
		}
	}
	var contributionType string
	if strings.EqualFold(inv.AccountType, "IRA") {
		contributionType = corr.DefaultContributionType
	}
	return &Context{
		Investor:         inv,
		Correspondent:    corr,
		OmnibusID:        corr.OmnibusAccountID,
		ContributionType: contributionType,
	}, nil
}
