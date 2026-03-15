package returns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/ledger"
	"github.com/alediez2048/apex/internal/store"
)

// ReturnFeeCents is the fixed $30.00 return fee per PRD.
const ReturnFeeCents int64 = 3000

// Result holds the outcome of a successful return for the API response.
type Result struct {
	TransferID         string `json:"transfer_id"`
	Status             string `json:"status"`
	ReversalAmountCents int64  `json:"reversal_amount_cents"`
	FeeCents           int64  `json:"fee_cents"`
	NetDebitCents      int64  `json:"net_debit_cents"`
}

// ProcessReturn handles the full return/reversal flow in a single database transaction:
// 1. Begin serializable transaction.
// 2. Read transfer inside tx (avoids TOCTOU race with concurrent return requests).
// 3. Validate state via domain.Transition (FundsPosted or Completed only).
// 4. Resolve correspondent omnibus account.
// 5. Post reversal pair (DEBIT investor / CREDIT omnibus for original amount).
// 6. Post fee pair (DEBIT investor / CREDIT omnibus for $30.00).
// 7. Transition transfer to Returned.
// 8. Log RETURN_RECEIVED, REVERSAL_POSTED, INVESTOR_NOTIFIED events.
// 9. Commit.
func ProcessReturn(ctx context.Context, db *sql.DB, cfg *config.Config, transferID, reason string) (*Result, error) {
	if reason == "" {
		reason = "NSF"
	}

	opts := &sql.TxOptions{Isolation: sql.LevelSerializable}
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("returns: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	t, err := store.GetTransferTx(tx, transferID)
	if err != nil {
		return nil, err
	}
	if err := domain.Transition(t.Status, domain.StateReturned); err != nil {
		return nil, err
	}

	var corr *config.Correspondent
	for i := range cfg.Correspondents {
		if cfg.Correspondents[i].ID == t.CorrespondentID {
			corr = &cfg.Correspondents[i]
			break
		}
	}
	if corr == nil {
		return nil, fmt.Errorf("returns: correspondent %s not found", t.CorrespondentID)
	}

	originalAmount := int64(t.Amount)
	netDebit := originalAmount + ReturnFeeCents

	// Reversal pair: DEBIT investor (clawback original), CREDIT omnibus
	if err := ledger.PostPairTx(tx, t.ID, t.InvestorAccountID, corr.OmnibusAccountID, originalAmount, "REVERSAL"); err != nil {
		return nil, fmt.Errorf("returns: reversal pair: %w", err)
	}

	// Fee pair: DEBIT investor ($30), CREDIT omnibus
	if err := ledger.PostPairTx(tx, t.ID, t.InvestorAccountID, corr.OmnibusAccountID, ReturnFeeCents, "RETURN_FEE"); err != nil {
		return nil, fmt.Errorf("returns: fee pair: %w", err)
	}

	if err := store.UpdateTransferStatusTx(tx, t.ID, domain.StateReturned); err != nil {
		return nil, fmt.Errorf("returns: update status: %w", err)
	}

	actor := "system"

	returnReceivedPayload, _ := json.Marshal(map[string]interface{}{
		"reason": reason,
		"actor":  actor,
	})
	if err := store.InsertEventTx(tx, t.ID, "RETURN_RECEIVED", actor, string(returnReceivedPayload)); err != nil {
		return nil, fmt.Errorf("returns: event RETURN_RECEIVED: %w", err)
	}

	reversalPayload, _ := json.Marshal(map[string]interface{}{
		"reversal_amount_cents": originalAmount,
		"fee_cents":             ReturnFeeCents,
	})
	if err := store.InsertEventTx(tx, t.ID, "REVERSAL_POSTED", actor, string(reversalPayload)); err != nil {
		return nil, fmt.Errorf("returns: event REVERSAL_POSTED: %w", err)
	}

	dollars := func(cents int64) string {
		return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
	}
	notifiedPayload, _ := json.Marshal(map[string]interface{}{
		"original_amount_cents": originalAmount,
		"fee_amount_cents":      ReturnFeeCents,
		"net_debit_cents":       netDebit,
		"reason_code":           reason,
		"message": fmt.Sprintf("Check returned (%s). %s reversed, %s fee. Net debit: %s.",
			reason, dollars(originalAmount), dollars(ReturnFeeCents), dollars(netDebit)),
	})
	if err := store.InsertEventTx(tx, t.ID, "INVESTOR_NOTIFIED", actor, string(notifiedPayload)); err != nil {
		return nil, fmt.Errorf("returns: event INVESTOR_NOTIFIED: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("returns: commit: %w", err)
	}

	return &Result{
		TransferID:          t.ID,
		Status:              string(domain.StateReturned),
		ReversalAmountCents: originalAmount,
		FeeCents:            ReturnFeeCents,
		NetDebitCents:       netDebit,
	}, nil
}

// IsNotFound reports whether the error is a "not found" from the store.
func IsNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}

// IsInvalidTransition reports whether the error is an invalid state transition.
func IsInvalidTransition(err error) bool {
	if errors.Is(err, domain.ErrInvalidTransition) {
		return true
	}
	var de *domain.DomainError
	if errors.As(err, &de) {
		return de.ErrCode == domain.CodeInvalidTransition
	}
	return false
}
