package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/alediez2048/apex/internal/config"
	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/pipeline"
	"github.com/alediez2048/apex/internal/returns"
	"github.com/alediez2048/apex/internal/settlement"
	"github.com/alediez2048/apex/internal/store"
)

// scenario defines a deposit to seed through the pipeline.
type scenario struct {
	ID          string // transfer ID (deterministic for idempotency)
	AccountID   string // investor account (prefix determines vendor outcome)
	APIKey      string // investor's Bearer token
	AmountCents int64
	PostAction  postAction // what to do after pipeline finishes
}

type postAction int

const (
	actionNone     postAction = iota // leave in pipeline result state
	actionApprove                    // operator approve (for flagged items that should reach FundsPosted)
	actionSettle                     // approve + settle into a batch
	actionReturn                     // approve + process return/reversal
	actionComplete                   // approve + settle + mark Completed
)

// scenarios produces deposits in at least 4 states with 2+ flagged items in queue.
var scenarios = []scenario{
	// 1. Clean pass → FundsPosted (auto-approved by pipeline)
	{ID: "seed-001", AccountID: "PASS-10001", APIKey: "tok_alice_001", AmountCents: 15000, PostAction: actionNone},

	// 2. Clean pass → FundsPosted → settled → Completed
	{ID: "seed-002", AccountID: "PASS-10001", APIKey: "tok_alice_001", AmountCents: 20000, PostAction: actionComplete},

	// 3. Clean pass → FundsPosted → Returned (with reversal + $30 fee)
	{ID: "seed-003", AccountID: "PASS-10001", APIKey: "tok_alice_001", AmountCents: 10000, PostAction: actionReturn},

	// 4. BLUR → Rejected (IQA failure at vendor step)
	{ID: "seed-004", AccountID: "BLUR-20001", APIKey: "tok_bob_002", AmountCents: 5000, PostAction: actionNone},

	// 5. GLARE → Rejected (IQA failure at vendor step)
	{ID: "seed-005", AccountID: "GLARE-20002", APIKey: "tok_frank_006", AmountCents: 7500, PostAction: actionNone},

	// 6. MICR failure → Analyzing (flagged, stays in operator queue)
	{ID: "seed-006", AccountID: "MICR-10001", APIKey: "tok_carol_003", AmountCents: 25000, PostAction: actionNone},

	// 7. Amount mismatch → Analyzing (flagged, stays in operator queue)
	{ID: "seed-007", AccountID: "MISMATCH-10001", APIKey: "tok_eve_005", AmountCents: 30000, PostAction: actionNone},

	// 8. Another clean pass for variety (different correspondent)
	{ID: "seed-008", AccountID: "PASS-10001", APIKey: "tok_alice_001", AmountCents: 35000, PostAction: actionSettle},
}

// Run seeds the database with sample deposits if no seed data exists.
// Idempotent: skips entirely if seed-001 already exists.
func Run(ctx context.Context, db *sql.DB, cfg *config.Config, deps pipeline.Deps) error {
	// Idempotency check: if first seed transfer exists, skip
	if _, err := store.GetTransfer(db, "seed-001"); err == nil {
		slog.Info("seed: data already exists, skipping")
		return nil
	}

	slog.Info("seed: populating demo data", "scenarios", len(scenarios))

	for _, s := range scenarios {
		if err := runScenario(ctx, db, cfg, deps, s); err != nil {
			return fmt.Errorf("seed: scenario %s: %w", s.ID, err)
		}
	}

	slog.Info("seed: demo data populated successfully")
	return nil
}

func runScenario(ctx context.Context, db *sql.DB, cfg *config.Config, deps pipeline.Deps, s scenario) error {
	// Create the transfer
	t := &domain.Transfer{
		ID:                s.ID,
		InvestorAccountID: s.AccountID,
		CorrespondentID:   correspondentForAccount(cfg, s.AccountID),
		Amount:            domain.Amount(s.AmountCents),
		Status:            domain.StateRequested,
		CheckNumber:       fmt.Sprintf("CHK-%s", s.ID),
	}
	if err := store.CreateTransfer(db, t); err != nil {
		return fmt.Errorf("create transfer: %w", err)
	}

	// Run the pipeline (vendor validation → funding rules → ledger posting)
	res, err := pipeline.Run(ctx, deps, s.ID, s.APIKey)
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	slog.Info("seed: pipeline result", "id", s.ID, "action", res.Action.String(), "status", res.Status)

	// Post-pipeline actions
	switch s.PostAction {
	case actionNone:
		// Leave as-is (FundsPosted, Rejected, or Analyzing)

	case actionApprove:
		if err := approveIfFlagged(ctx, db, cfg, deps, s.ID); err != nil {
			return err
		}

	case actionSettle:
		if err := approveIfFlagged(ctx, db, cfg, deps, s.ID); err != nil {
			return err
		}
		if err := settleBatch(db); err != nil {
			return err
		}

	case actionComplete:
		if err := approveIfFlagged(ctx, db, cfg, deps, s.ID); err != nil {
			return err
		}
		if err := settleBatch(db); err != nil {
			return err
		}
		// Mark as Completed (post-settlement)
		if err := store.UpdateTransferStatus(db, s.ID, domain.StateCompleted); err != nil {
			return fmt.Errorf("complete: %w", err)
		}
		_ = store.InsertEvent(db, s.ID, domain.EventTypeStateTransition, "system",
			`{"from_state":"FundsPosted","to_state":"Completed","actor":"system","reason":"settlement confirmed"}`)

	case actionReturn:
		if err := approveIfFlagged(ctx, db, cfg, deps, s.ID); err != nil {
			return err
		}
		if _, err := returns.ProcessReturn(ctx, db, cfg, s.ID, "NSF"); err != nil {
			return fmt.Errorf("return: %w", err)
		}
	}

	return nil
}

// approveIfFlagged operator-approves a transfer if it's in Analyzing state.
func approveIfFlagged(ctx context.Context, db *sql.DB, cfg *config.Config, deps pipeline.Deps, id string) error {
	t, err := store.GetTransfer(db, id)
	if err != nil {
		return err
	}
	if t.Status == domain.StateAnalyzing {
		return pipeline.OperatorApprove(ctx, deps, cfg, id, "seed-operator", "")
	}
	return nil
}

// settleBatch creates a settlement batch for any unbatched FundsPosted transfers.
func settleBatch(db *sql.DB) error {
	now := time.Now().UTC()
	batchID, _, _, err := settlement.GenerateBatch(db, now)
	if err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	if batchID != "" {
		slog.Info("seed: settlement batch created", "batch_id", batchID)
	}
	return nil
}

// correspondentForAccount finds the correspondent ID for an investor account.
func correspondentForAccount(cfg *config.Config, accountID string) string {
	for _, inv := range cfg.Investors {
		if inv.AccountID == accountID {
			return inv.CorrespondentID
		}
	}
	return "CORR-APEX" // fallback
}
