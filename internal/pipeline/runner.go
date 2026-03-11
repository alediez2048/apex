package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/alediez2048/apex/internal/domain"
	"github.com/alediez2048/apex/internal/funding"
	"github.com/alediez2048/apex/internal/ledger"
	"github.com/alediez2048/apex/internal/store"
	"github.com/alediez2048/apex/internal/vendor"
)

// Deps holds the injected dependencies for a pipeline run.
type Deps struct {
	DB            *sql.DB
	VendorStub    *vendor.Stub
	FundingEngine *funding.Engine
}

// Run executes the deposit pipeline for an existing transfer (status must be Requested).
// It transitions the transfer through the PRD-defined states, persists each transition
// and logs to deposit_events. Returns the final Result.
func Run(ctx context.Context, deps Deps, transferID, apiKey string) (*Result, error) {
	t, err := store.GetTransfer(deps.DB, transferID)
	if err != nil {
		return nil, err
	}
	if t.Status != domain.StateRequested {
		return nil, fmt.Errorf("pipeline: transfer %s not in Requested state (is %s)", t.ID, t.Status)
	}

	// --- Step 1: Vendor validation ---
	res, err := stepVendor(ctx, deps, t)
	if err != nil {
		return nil, err
	}
	if res.Action != Continue {
		return res, nil
	}

	// Reload transfer (vendor fields updated)
	t, err = store.GetTransfer(deps.DB, transferID)
	if err != nil {
		return nil, err
	}

	// --- Step 2: Funding rules ---
	res, err = stepFunding(ctx, deps, t, apiKey)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// stepVendor: Requested → Validating → (Analyzing | Rejected).
func stepVendor(ctx context.Context, deps Deps, t *domain.Transfer) (*Result, error) {
	// Requested → Validating
	if err := transition(deps.DB, t, domain.StateValidating, "system", "pipeline step 1: vendor validation"); err != nil {
		return nil, err
	}
	logStep(deps.DB, t.ID, 1, "vendor_validation", "Continue", "system")

	resp := deps.VendorStub.Validate(vendor.Request{
		AccountID:   t.InvestorAccountID,
		AmountCents: int64(t.Amount),
	}, "")

	if isTerminalOutcome(resp.Outcome) {
		// Validating → Rejected
		reason := resp.ErrorCode + ": " + resp.Message
		if err := transition(deps.DB, t, domain.StateRejected, "system", reason); err != nil {
			return nil, err
		}
		logEvent(deps.DB, t.ID, "DEPOSIT_REJECTED", "system", map[string]interface{}{
			"error_code": resp.ErrorCode,
			"message":    resp.Message,
			"outcome":    resp.Outcome,
		})
		logStep(deps.DB, t.ID, 1, "vendor_validation", "HaltRejected", "system")
		return &Result{Action: HaltRejected, Status: string(domain.StateRejected), Reason: reason}, nil
	}

	// Non-terminal: Validating → Analyzing; store vendor fields + risk score
	score := RiskScore(resp.Confidence)
	if err := store.UpdateTransferVendor(deps.DB, t.ID,
		resp.VendorTransactionID, resp.CheckNumber,
		resp.MICRRouting, resp.MICRAccount, score); err != nil {
		return nil, err
	}
	if err := transition(deps.DB, t, domain.StateAnalyzing, "system", "vendor: "+resp.Outcome); err != nil {
		return nil, err
	}

	if IsFlagged(score) {
		logStep(deps.DB, t.ID, 1, "vendor_validation", "HaltFlagged", "system")
		return &Result{Action: HaltFlagged, Status: string(domain.StateAnalyzing), Reason: "risk score " + fmt.Sprint(score) + " — flagged for review"}, nil
	}

	return &Result{Action: Continue}, nil
}

// stepFunding: Analyzing → (Approved → FundsPosted | Rejected).
func stepFunding(ctx context.Context, deps Deps, t *domain.Transfer, apiKey string) (*Result, error) {
	logStep(deps.DB, t.ID, 2, "funding_rules", "Continue", "system")

	fundCtx, err := deps.FundingEngine.ValidateDeposit(
		apiKey, int64(t.Amount), t.MICRRouting, t.MICRAccount, t.CheckNumber, t.ID,
	)
	if err != nil {
		reason := err.Error()
		if de, ok := err.(*domain.DomainError); ok {
			reason = de.Code() + ": " + de.Error()
		}
		if txErr := transition(deps.DB, t, domain.StateRejected, "system", reason); txErr != nil {
			return nil, txErr
		}
		logEvent(deps.DB, t.ID, "DEPOSIT_REJECTED", "system", map[string]interface{}{
			"reason": reason,
		})
		logStep(deps.DB, t.ID, 2, "funding_rules", "HaltRejected", "system")
		return &Result{Action: HaltRejected, Status: string(domain.StateRejected), Reason: reason}, nil
	}

	// Store contribution_type if resolved
	if fundCtx.ContributionType != "" {
		_ = store.UpdateTransferContribution(deps.DB, t.ID, fundCtx.ContributionType)
	}

	// Analyzing → Approved (actor=system for auto-approve)
	if err := transition(deps.DB, t, domain.StateApproved, "system", "auto-approved"); err != nil {
		return nil, err
	}
	logEvent(deps.DB, t.ID, "DEPOSIT_APPROVED", "system", map[string]interface{}{
		"actor": "system",
	})
	logStep(deps.DB, t.ID, 2, "funding_rules", "Continue", "system")

	// Step 3: Approved → FundsPosted (ledger post)
	logStep(deps.DB, t.ID, 3, "ledger_posting", "Continue", "system")
	if err := ledger.Post(ctx, deps.DB, t.ID, fundCtx.OmnibusID, fundCtx.Investor.AccountID, int64(t.Amount), "FREE"); err != nil {
		return nil, fmt.Errorf("pipeline: ledger post: %w", err)
	}
	if err := transition(deps.DB, t, domain.StateFundsPosted, "system", "ledger posted"); err != nil {
		return nil, err
	}
	logEvent(deps.DB, t.ID, "LEDGER_POSTED", "system", map[string]interface{}{
		"from_account": fundCtx.OmnibusID,
		"to_account":   fundCtx.Investor.AccountID,
		"amount_cents": int64(t.Amount),
	})
	logStep(deps.DB, t.ID, 3, "ledger_posting", "HaltApproved", "system")

	return &Result{Action: HaltApproved, Status: string(domain.StateFundsPosted), Reason: "funds posted"}, nil
}

// transition validates + persists a state change and logs STATE_TRANSITION event.
func transition(db *sql.DB, t *domain.Transfer, to domain.State, actor, reason string) error {
	if err := domain.Transition(t.Status, to); err != nil {
		return fmt.Errorf("pipeline: %s → %s: %w", t.Status, to, err)
	}
	from := t.Status
	t.Status = to
	if err := store.UpdateTransferStatus(db, t.ID, to); err != nil {
		return err
	}
	payload, _ := json.Marshal(domain.StateTransitionPayload{
		FromState: string(from),
		ToState:   string(to),
		Actor:     actor,
		Reason:    reason,
	})
	return store.InsertEvent(db, t.ID, domain.EventTypeStateTransition, actor, string(payload))
}

func isTerminalOutcome(outcome string) bool {
	switch outcome {
	case vendor.OutcomeIQAFailBlur, vendor.OutcomeIQAFailGlare, vendor.OutcomeDuplicate:
		return true
	}
	return false
}

func logEvent(db *sql.DB, transferID, eventType, actor string, payload map[string]interface{}) {
	data, _ := json.Marshal(payload)
	if err := store.InsertEvent(db, transferID, eventType, actor, string(data)); err != nil {
		slog.Error("pipeline: log event failed", "error", err, "transfer_id", transferID, "event", eventType)
	}
}

func logStep(db *sql.DB, transferID string, stepIndex int, stepName, action, actor string) {
	logEvent(db, transferID, "PIPELINE_STEP", actor, map[string]interface{}{
		"step_index": stepIndex,
		"step_name":  stepName,
		"action":     action,
	})
}
