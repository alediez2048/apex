package store

import (
	"database/sql"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps *sql.DB for the application. Use BeginTx with sql.TxOptions{Isolation: sql.LevelSerializable}
// and "BEGIN IMMEDIATE" for write transactions (handled by the driver when using sql.LevelSerializable).
var defaultDriver = "sqlite3"

// Open opens a SQLite database at dbPath. Caller must call Close when done.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open(defaultDriver, dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite single-writer; one conn avoids lock issues
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	slog.Info("database opened", "path", dbPath)
	return db, nil
}
