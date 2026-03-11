package store

import (
	"database/sql"
	"fmt"
)

// LedgerEntry mirrors a row from the ledger_entries table.
type LedgerEntry struct {
	ID         string `json:"id"`
	TransferID string `json:"transfer_id"`
	AccountID  string `json:"account_id"`
	EntryType  string `json:"entry_type"`
	Amount     int64  `json:"amount_cents"`
	Memo       string `json:"memo"`
	PostedAt   string `json:"posted_at"`
}

// ListLedgerEntriesByAccount returns all ledger entries for a given account, newest first.
func ListLedgerEntriesByAccount(db *sql.DB, accountID string) ([]LedgerEntry, error) {
	const q = `SELECT id, transfer_id, account_id, entry_type, amount, COALESCE(memo,''), posted_at
		FROM ledger_entries WHERE account_id = ? ORDER BY posted_at DESC`
	rows, err := db.Query(q, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list ledger entries: %w", err)
	}
	defer rows.Close()
	var out []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.TransferID, &e.AccountID, &e.EntryType, &e.Amount, &e.Memo, &e.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
