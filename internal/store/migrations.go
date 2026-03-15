package store

import (
	"database/sql"
	"log/slog"
)

// migration holds a version and the SQL to run (one or more statements).
type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `CREATE TABLE IF NOT EXISTS transfers (
	id TEXT PRIMARY KEY,
	investor_account_id TEXT NOT NULL,
	correspondent_id TEXT NOT NULL,
	amount INTEGER NOT NULL,
	status TEXT NOT NULL,
	vendor_transaction_id TEXT,
	check_number TEXT NOT NULL,
	micr_routing TEXT,
	micr_account TEXT,
	micr_data TEXT,
	risk_score INTEGER,
	contribution_type TEXT,
	settlement_batch_id TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ledger_entries (
	id TEXT PRIMARY KEY,
	transfer_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	entry_type TEXT NOT NULL CHECK (entry_type IN ('DEBIT','CREDIT')),
	amount INTEGER NOT NULL,
	memo TEXT,
	posted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	reversal_of TEXT REFERENCES ledger_entries(id)
);

CREATE TABLE IF NOT EXISTS deposit_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	transfer_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	actor TEXT NOT NULL,
	payload TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transfers_duplicate ON transfers(micr_routing, micr_account, check_number);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_transfer ON ledger_entries(transfer_id);
CREATE INDEX IF NOT EXISTS idx_deposit_events_transfer ON deposit_events(transfer_id);
`,
	},
	{
		version: 2,
		sql: `CREATE TABLE IF NOT EXISTS settlement_batches (
	id TEXT PRIMARY KEY,
	settlement_date TEXT NOT NULL,
	file_json TEXT NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_transfers_settlement_batch ON transfers(settlement_batch_id);
`,
	},
}

// RunMigrations runs any pending migrations in order. Idempotent: already-applied versions are skipped.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)")
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var applied int
		err := db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", m.version).Scan(&applied)
		if err != nil {
			return err
		}
		if applied > 0 {
			slog.Debug("migration already applied", "version", m.version)
			continue
		}

		_, err = db.Exec(m.sql)
		if err != nil {
			return err
		}
		_, err = db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version)
		if err != nil {
			return err
		}
		slog.Info("migration applied", "version", m.version)
	}
	return nil
}
