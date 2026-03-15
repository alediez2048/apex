package store

import (
	"database/sql"
	"fmt"
)

// InsertEvent adds a row to deposit_events (audit trail / decision trace).
func InsertEvent(db *sql.DB, transferID, eventType, actor, payloadJSON string) error {
	_, err := db.Exec(
		"INSERT INTO deposit_events (transfer_id, event_type, actor, payload) VALUES (?,?,?,?)",
		transferID, eventType, actor, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("store: insert event: %w", err)
	}
	return nil
}

// InsertEventTx adds a row to deposit_events using an existing transaction.
func InsertEventTx(tx *sql.Tx, transferID, eventType, actor, payloadJSON string) error {
	_, err := tx.Exec(
		"INSERT INTO deposit_events (transfer_id, event_type, actor, payload) VALUES (?,?,?,?)",
		transferID, eventType, actor, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("store: insert event (tx): %w", err)
	}
	return nil
}

// ListEvents returns all events for a transfer, oldest first.
func ListEvents(db *sql.DB, transferID string) ([]Event, error) {
	rows, err := db.Query(
		"SELECT id, transfer_id, event_type, actor, COALESCE(payload,''), created_at FROM deposit_events WHERE transfer_id = ? ORDER BY id",
		transferID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list events: %w", err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TransferID, &e.EventType, &e.Actor, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Event mirrors deposit_events row.
type Event struct {
	ID         int64
	TransferID string
	EventType  string
	Actor      string
	Payload    string
	CreatedAt  string
}

// GetDistinctErrorCodesFromEvents returns distinct error_code values from deposit_events payloads (JSON).
// Used for vendor scenario coverage: count how many of the 7 vendor error codes have been exercised.
func GetDistinctErrorCodesFromEvents(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT json_extract(payload, '$.error_code') FROM deposit_events
		WHERE payload IS NOT NULL AND json_extract(payload, '$.error_code') IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("store: distinct error codes: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s.Valid && s.String != "" {
			out = append(out, s.String)
		}
	}
	return out, nil
}
