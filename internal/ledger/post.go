package ledger

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// Post creates a balanced DEBIT + CREDIT pair in a single transaction.
// fromAccountID is debited; toAccountID is credited. Both rows share the same transfer_id and memo.
// The transaction uses BEGIN IMMEDIATE to acquire the write lock up front (SQLite single-writer).
func Post(ctx context.Context, db *sql.DB, transferID, fromAccountID, toAccountID string, amountCents int64, memo string) error {
	if amountCents <= 0 {
		return fmt.Errorf("ledger: amount must be positive, got %d", amountCents)
	}
	return postTx(ctx, db, transferID, fromAccountID, toAccountID, amountCents, memo)
}

func postTx(ctx context.Context, db *sql.DB, transferID, fromAccountID, toAccountID string, amountCents int64, memo string) error {
	// BEGIN IMMEDIATE — acquires reserved lock before any writes, preventing writer starvation.
	// We manage the transaction manually so we can specify IMMEDIATE explicitly.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("ledger: conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("ledger: begin immediate: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	debitID, err := newID()
	if err != nil {
		return err
	}
	creditID, err := newID()
	if err != nil {
		return err
	}

	const ins = `INSERT INTO ledger_entries (id, transfer_id, account_id, entry_type, amount, memo) VALUES (?,?,?,?,?,?)`

	if _, err := conn.ExecContext(ctx, ins, debitID, transferID, fromAccountID, "DEBIT", amountCents, memo); err != nil {
		return fmt.Errorf("ledger: insert debit: %w", err)
	}
	if _, err := conn.ExecContext(ctx, ins, creditID, transferID, toAccountID, "CREDIT", amountCents, memo); err != nil {
		return fmt.Errorf("ledger: insert credit: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("ledger: commit: %w", err)
	}
	committed = true
	return nil
}

// PostPairTx inserts a balanced DEBIT + CREDIT pair using an existing transaction.
// Caller owns the transaction (begin/commit/rollback). fromAccountID is debited; toAccountID is credited.
func PostPairTx(tx *sql.Tx, transferID, fromAccountID, toAccountID string, amountCents int64, memo string) error {
	if amountCents <= 0 {
		return fmt.Errorf("ledger: amount must be positive, got %d", amountCents)
	}
	debitID, err := newID()
	if err != nil {
		return err
	}
	creditID, err := newID()
	if err != nil {
		return err
	}
	const ins = `INSERT INTO ledger_entries (id, transfer_id, account_id, entry_type, amount, memo) VALUES (?,?,?,?,?,?)`
	if _, err := tx.Exec(ins, debitID, transferID, fromAccountID, "DEBIT", amountCents, memo); err != nil {
		return fmt.Errorf("ledger: insert debit: %w", err)
	}
	if _, err := tx.Exec(ins, creditID, transferID, toAccountID, "CREDIT", amountCents, memo); err != nil {
		return fmt.Errorf("ledger: insert credit: %w", err)
	}
	return nil
}

// newID returns a "le-" prefixed 32-char hex string from 16 bytes of crypto/rand.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("ledger: rand id: %w", err)
	}
	return "le-" + hex.EncodeToString(b[:]), nil
}
