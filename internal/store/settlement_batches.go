package store

import (
	"database/sql"
	"fmt"

	"github.com/alediez2048/apex/internal/domain"
)

// InsertSettlementBatch inserts a row into settlement_batches. Must be called within a transaction.
func InsertSettlementBatch(tx *sql.Tx, id, settlementDate, fileJSON string) error {
	_, err := tx.Exec(
		"INSERT INTO settlement_batches (id, settlement_date, file_json) VALUES (?, ?, ?)",
		id, settlementDate, fileJSON,
	)
	if err != nil {
		return fmt.Errorf("store: insert settlement batch: %w", err)
	}
	return nil
}

// SettlementBatch holds settlement_batches row data.
type SettlementBatch struct {
	ID             string
	SettlementDate  string
	FileJSON        string
	CreatedAt       string
}

// GetBatch returns a batch by ID. Returns ErrNotFound if not found.
func GetBatch(db *sql.DB, id string) (*SettlementBatch, error) {
	var b SettlementBatch
	err := db.QueryRow(
		"SELECT id, settlement_date, file_json, created_at FROM settlement_batches WHERE id = ?",
		id,
	).Scan(&b.ID, &b.SettlementDate, &b.FileJSON, &b.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get batch: %w", err)
	}
	return &b, nil
}

// ListTransfersBySettlementBatch returns all transfers that belong to the given batch.
func ListTransfersBySettlementBatch(db *sql.DB, batchID string) ([]*domain.Transfer, error) {
	q := `SELECT ` + transferColumns + ` FROM transfers WHERE settlement_batch_id = ? ORDER BY created_at ASC`
	rows, err := db.Query(q, batchID)
	if err != nil {
		return nil, fmt.Errorf("store: list transfers by batch: %w", err)
	}
	defer rows.Close()
	var out []*domain.Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
