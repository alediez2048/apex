package settlement

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/alediez2048/apex/internal/store"
)

// GenerateBatch lists unbatched FundsPosted in a transaction, builds X9 file, inserts batch, assigns settlement_batch_id to each transfer, and commits.
// Uses serializable isolation (BEGIN IMMEDIATE–style single-writer consistency with the ledger). now is the effective "current" time (from real clock or ?as_of).
// Returns batchID, settlementDate, fileJSON. If no transfers to batch, returns empty batchID and fileJSON with nil error (caller treats as "no batch created").
func GenerateBatch(db *sql.DB, now time.Time) (batchID, settlementDate, fileJSON string, err error) {
	ctx := context.Background()
	opts := &sql.TxOptions{Isolation: sql.LevelSerializable}
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return "", "", "", fmt.Errorf("settlement: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	list, err := store.ListUnbatchedFundsPostedInTx(tx)
	if err != nil {
		return "", "", "", err
	}
	if len(list) == 0 {
		return "", SettlementDate(now), "", nil
	}

	settlementDate = SettlementDate(now)
	x9, err := BuildFile(list, settlementDate, now)
	if err != nil {
		return "", "", "", err
	}
	fileJSON, err = x9.ToJSON()
	if err != nil {
		return "", "", "", err
	}

	batchID = "batch-" + settlementDate + "-" + shortUUID()
	if err := store.InsertSettlementBatch(tx, batchID, settlementDate, fileJSON); err != nil {
		return "", "", "", err
	}
	for _, t := range list {
		if err := store.UpdateTransferSettlementBatch(tx, t.ID, batchID); err != nil {
			return "", "", "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", fmt.Errorf("settlement: commit: %w", err)
	}
	return batchID, settlementDate, fileJSON, nil
}

func shortUUID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
