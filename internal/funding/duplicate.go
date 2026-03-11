package funding

import (
	"database/sql"
	"time"
)

// DuplicateChecker reports whether a duplicate deposit exists in the window (PRD: composite key, 30-day rolling).
type DuplicateChecker interface {
	Exists(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time) (bool, error)
	// ExistsExcluding is like Exists but ignores the transfer with id excludeTransferID (use when checking the current deposit so it does not match itself).
	ExistsExcluding(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time, excludeTransferID string) (bool, error)
}

// MemoryDuplicateChecker for unit tests: Record then Exists returns true for same key after windowStart.
type MemoryDuplicateChecker struct {
	keys map[string]time.Time
}

func NewMemoryDuplicateChecker() *MemoryDuplicateChecker {
	return &MemoryDuplicateChecker{keys: make(map[string]time.Time)}
}

func dupKeyFmt(routing, account, check string, amount int64) string {
	return routing + "|" + account + "|" + check + "|" + itoa(amount)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [32]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Record stores a composite key at now.
func (m *MemoryDuplicateChecker) Record(micrRouting, micrAccount, checkNumber string, amountCents int64) {
	m.RecordAt(micrRouting, micrAccount, checkNumber, amountCents, time.Now())
}

// RecordAt stores a composite key at t (for tests / backdating).
func (m *MemoryDuplicateChecker) RecordAt(micrRouting, micrAccount, checkNumber string, amountCents int64, t time.Time) {
	m.keys[dupKeyFmt(micrRouting, micrAccount, checkNumber, amountCents)] = t
}

func (m *MemoryDuplicateChecker) Exists(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time) (bool, error) {
	return m.ExistsExcluding(micrRouting, micrAccount, checkNumber, amountCents, windowStart, "")
}

func (m *MemoryDuplicateChecker) ExistsExcluding(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time, excludeTransferID string) (bool, error) {
	_ = excludeTransferID // in-memory checker has no row identity; tests use Record() for prior duplicates
	k := dupKeyFmt(micrRouting, micrAccount, checkNumber, amountCents)
	t, ok := m.keys[k]
	if !ok || t.Before(windowStart) {
		return false, nil
	}
	return true, nil
}

// SQLDuplicateChecker queries transfers for matching composite key within window.
type SQLDuplicateChecker struct {
	DB *sql.DB
}

func (s *SQLDuplicateChecker) Exists(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time) (bool, error) {
	return s.ExistsExcluding(micrRouting, micrAccount, checkNumber, amountCents, windowStart, "")
}

func (s *SQLDuplicateChecker) ExistsExcluding(micrRouting, micrAccount, checkNumber string, amountCents int64, windowStart time.Time, excludeTransferID string) (bool, error) {
	since := windowStart.UTC().Format("2006-01-02 15:04:05")
	q := `SELECT 1 FROM transfers
		WHERE COALESCE(micr_routing,'') = ? AND COALESCE(micr_account,'') = ? AND check_number = ? AND amount = ?
		AND created_at >= ?`
	args := []interface{}{micrRouting, micrAccount, checkNumber, amountCents, since}
	if excludeTransferID != "" {
		q += ` AND id != ?`
		args = append(args, excludeTransferID)
	}
	q += ` LIMIT 1`
	var one int
	err := s.DB.QueryRow(q, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
