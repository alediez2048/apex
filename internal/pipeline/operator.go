package pipeline

import (
	"context"
	"fmt"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/ledger"
	"github.com/alediez2048/apex/internal/store"
)

// OperatorApprove transitions a flagged transfer (Analyzing) to Approved → FundsPosted
// with actor=operator:{operatorID}. It performs ledger posting as part of the approval.
func OperatorApprove(ctx context.Context, deps Deps, cfg *config.Config, transferID, operatorID string) error {
	t, err := store.GetTransfer(deps.DB, transferID)
	if err != nil {
		return err
	}
	if t.Status != domain.StateAnalyzing {
		return domain.NewDomainError(domain.CodeInvalidTransition,
			fmt.Sprintf("cannot approve: transfer is %s, expected Analyzing", t.Status))
	}

	actor := "operator:" + operatorID

	// Analyzing → Approved
	if err := transition(deps.DB, t, domain.StateApproved, actor, "operator approved"); err != nil {
		return err
	}
	logEvent(deps.DB, t.ID, "DEPOSIT_APPROVED", actor, map[string]interface{}{
		"actor": actor,
	})

	// Resolve omnibus from correspondent
	var corr *config.Correspondent
	for i := range cfg.Correspondents {
		if cfg.Correspondents[i].ID == t.CorrespondentID {
			corr = &cfg.Correspondents[i]
			break
		}
	}
	if corr == nil {
		return fmt.Errorf("pipeline: correspondent %s not found", t.CorrespondentID)
	}

	// Ledger post
	if err := ledger.Post(ctx, deps.DB, t.ID, corr.OmnibusAccountID, t.InvestorAccountID, int64(t.Amount), "FREE"); err != nil {
		return fmt.Errorf("pipeline: operator ledger post: %w", err)
	}

	// Approved → FundsPosted
	if err := transition(deps.DB, t, domain.StateFundsPosted, actor, "ledger posted"); err != nil {
		return err
	}
	logEvent(deps.DB, t.ID, "LEDGER_POSTED", actor, map[string]interface{}{
		"from_account": corr.OmnibusAccountID,
		"to_account":   t.InvestorAccountID,
		"amount_cents": int64(t.Amount),
	})

	return nil
}

// OperatorReject transitions a flagged transfer (Analyzing) to Rejected
// with actor=operator:{operatorID}.
func OperatorReject(ctx context.Context, deps Deps, transferID, operatorID, reason string) error {
	t, err := store.GetTransfer(deps.DB, transferID)
	if err != nil {
		return err
	}
	if t.Status != domain.StateAnalyzing {
		return domain.NewDomainError(domain.CodeInvalidTransition,
			fmt.Sprintf("cannot reject: transfer is %s, expected Analyzing", t.Status))
	}

	actor := "operator:" + operatorID

	if err := transition(deps.DB, t, domain.StateRejected, actor, reason); err != nil {
		return err
	}
	logEvent(deps.DB, t.ID, "DEPOSIT_REJECTED", actor, map[string]interface{}{
		"actor":  actor,
		"reason": reason,
	})

	return nil
}
