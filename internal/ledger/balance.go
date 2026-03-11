package ledger

import (
	"context"
	"database/sql"
	"fmt"
)

// Balance returns the net balance for accountID: SUM(credits) − SUM(debits), in cents.
// Returns 0 if no entries exist for the account.
func Balance(ctx context.Context, db *sql.DB, accountID string) (int64, error) {
	const q = `SELECT COALESCE(SUM(CASE WHEN entry_type = 'CREDIT' THEN amount ELSE -amount END), 0)
		FROM ledger_entries WHERE account_id = ?`
	var bal int64
	if err := db.QueryRowContext(ctx, q, accountID).Scan(&bal); err != nil {
		return 0, fmt.Errorf("ledger: balance query: %w", err)
	}
	return bal, nil
}
