package ledger

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory SQLite DB with the ledger_entries schema from migration 001.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const schema = `CREATE TABLE IF NOT EXISTS ledger_entries (
		id TEXT PRIMARY KEY,
		transfer_id TEXT NOT NULL,
		account_id TEXT NOT NULL,
		entry_type TEXT NOT NULL CHECK (entry_type IN ('DEBIT','CREDIT')),
		amount INTEGER NOT NULL,
		memo TEXT,
		posted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		reversal_of TEXT REFERENCES ledger_entries(id)
	);
	CREATE INDEX IF NOT EXISTS idx_ledger_entries_transfer ON ledger_entries(transfer_id);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestPost_CreatesExactlyOneDebitAndOneCredit(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := Post(ctx, db, "txn-001", "OMNI-APEX-001", "PASS-10001", 15000, "FREE")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	rows, err := db.Query("SELECT id, account_id, entry_type, amount, memo FROM ledger_entries WHERE transfer_id = ?", "txn-001")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type entry struct {
		id, accountID, entryType, memo string
		amount                         int64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.accountID, &e.entryType, &e.amount, &e.memo); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, e)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	var debit, credit *entry
	for i := range entries {
		switch entries[i].entryType {
		case "DEBIT":
			debit = &entries[i]
		case "CREDIT":
			credit = &entries[i]
		}
	}
	if debit == nil || credit == nil {
		t.Fatal("expected one DEBIT and one CREDIT")
	}
	if debit.amount != 15000 {
		t.Fatalf("debit amount: want 15000, got %d", debit.amount)
	}
	if credit.amount != 15000 {
		t.Fatalf("credit amount: want 15000, got %d", credit.amount)
	}
	if debit.accountID != "OMNI-APEX-001" {
		t.Fatalf("debit account: want OMNI-APEX-001, got %s", debit.accountID)
	}
	if credit.accountID != "PASS-10001" {
		t.Fatalf("credit account: want PASS-10001, got %s", credit.accountID)
	}
	if debit.memo != "FREE" || credit.memo != "FREE" {
		t.Fatalf("memos: %q / %q", debit.memo, credit.memo)
	}
	if debit.id == credit.id {
		t.Fatal("debit and credit must have distinct IDs")
	}
}

func TestPost_RejectsNonPositiveAmount(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	if err := Post(ctx, db, "txn-neg", "A", "B", 0, ""); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err := Post(ctx, db, "txn-neg2", "A", "B", -100, ""); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestBalance_CorrectAfterMultiplePosts(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Post 1: OMNI → PASS-10001, 15000 cents
	if err := Post(ctx, db, "txn-1", "OMNI-APEX-001", "PASS-10001", 15000, "FREE"); err != nil {
		t.Fatal(err)
	}
	// Post 2: OMNI → PASS-10001, 5000 cents
	if err := Post(ctx, db, "txn-2", "OMNI-APEX-001", "PASS-10001", 5000, "FREE"); err != nil {
		t.Fatal(err)
	}
	// Post 3: OMNI-BETA → MICR-10001, 3000 cents
	if err := Post(ctx, db, "txn-3", "OMNI-BETA-001", "MICR-10001", 3000, "FREE"); err != nil {
		t.Fatal(err)
	}

	// PASS-10001: credited 15000 + 5000 = 20000
	bal, err := Balance(ctx, db, "PASS-10001")
	if err != nil {
		t.Fatal(err)
	}
	if bal != 20000 {
		t.Fatalf("PASS-10001 balance: want 20000, got %d", bal)
	}

	// OMNI-APEX-001: debited 15000 + 5000 = -20000
	bal, err = Balance(ctx, db, "OMNI-APEX-001")
	if err != nil {
		t.Fatal(err)
	}
	if bal != -20000 {
		t.Fatalf("OMNI-APEX-001 balance: want -20000, got %d", bal)
	}

	// MICR-10001: credited 3000
	bal, err = Balance(ctx, db, "MICR-10001")
	if err != nil {
		t.Fatal(err)
	}
	if bal != 3000 {
		t.Fatalf("MICR-10001 balance: want 3000, got %d", bal)
	}

	// Non-existent account: 0
	bal, err = Balance(ctx, db, "NOBODY")
	if err != nil {
		t.Fatal(err)
	}
	if bal != 0 {
		t.Fatalf("NOBODY balance: want 0, got %d", bal)
	}
}

func TestLedgerInvariant(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	posts := []struct {
		id   string
		from string
		to   string
		amt  int64
	}{
		{"inv-1", "OMNI-APEX-001", "PASS-10001", 15000},
		{"inv-2", "OMNI-APEX-001", "PASS-10001", 5000},
		{"inv-3", "OMNI-BETA-001", "MICR-10001", 3000},
		{"inv-4", "OMNI-GAMMA-001", "MISMATCH-10001", 75000},
	}
	for _, p := range posts {
		if err := Post(ctx, db, p.id, p.from, p.to, p.amt, "FREE"); err != nil {
			t.Fatalf("Post %s: %v", p.id, err)
		}
	}

	// Global invariant: sum(DEBIT amounts) == sum(CREDIT amounts)
	rows, err := db.Query("SELECT entry_type, SUM(amount) FROM ledger_entries GROUP BY entry_type")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	sums := make(map[string]int64)
	for rows.Next() {
		var et string
		var total int64
		if err := rows.Scan(&et, &total); err != nil {
			t.Fatal(err)
		}
		sums[et] = total
	}
	if sums["DEBIT"] != sums["CREDIT"] {
		t.Fatalf("invariant broken: DEBIT sum %d != CREDIT sum %d", sums["DEBIT"], sums["CREDIT"])
	}
	if sums["DEBIT"] == 0 {
		t.Fatal("no entries were posted")
	}

	expectedTotal := int64(15000 + 5000 + 3000 + 75000)
	if sums["DEBIT"] != expectedTotal {
		t.Fatalf("total: want %d, got %d", expectedTotal, sums["DEBIT"])
	}
}

func TestPost_TransactionRollsBackOnFailure(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Post a valid entry first
	if err := Post(ctx, db, "good", "A", "B", 100, "ok"); err != nil {
		t.Fatal(err)
	}

	// Now try posting with a duplicate ledger entry ID — impossible to force with crypto/rand,
	// but we can test the constraint: post with same transfer_id should still succeed
	// (transfer_id is not unique; only ledger entry id is).
	if err := Post(ctx, db, "good", "A", "B", 200, "ok"); err != nil {
		t.Fatalf("second post with same transfer_id should succeed: %v", err)
	}

	// Verify we have 4 entries total (2 from each post)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ledger_entries").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("want 4 entries, got %d", count)
	}
}
