package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alediez2048/apex/internal/domain"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("store: not found")

// CreateTransfer inserts a new transfer with status Requested.
func CreateTransfer(db *sql.DB, t *domain.Transfer) error {
	const q = `INSERT INTO transfers
		(id, investor_account_id, correspondent_id, amount, status, check_number, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec(q, t.ID, t.InvestorAccountID, t.CorrespondentID,
		int64(t.Amount), string(t.Status), t.CheckNumber, now, now)
	if err != nil {
		return fmt.Errorf("store: create transfer: %w", err)
	}
	return nil
}

// UpdateTransferStatus sets status and updated_at.
func UpdateTransferStatus(db *sql.DB, id string, status domain.State) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE transfers SET status = ?, updated_at = ? WHERE id = ?", string(status), now, id)
	if err != nil {
		return fmt.Errorf("store: update transfer status: %w", err)
	}
	return nil
}

// UpdateTransferVendor sets vendor-related fields after step 1.
func UpdateTransferVendor(db *sql.DB, id, vendorTxnID, checkNumber, micrRouting, micrAccount string, riskScore int) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`UPDATE transfers SET
		vendor_transaction_id = ?, check_number = ?, micr_routing = ?, micr_account = ?,
		risk_score = ?, updated_at = ?
		WHERE id = ?`,
		vendorTxnID, checkNumber, micrRouting, micrAccount, riskScore, now, id)
	if err != nil {
		return fmt.Errorf("store: update transfer vendor: %w", err)
	}
	return nil
}

// UpdateTransferContribution sets contribution_type after funding resolves it.
func UpdateTransferContribution(db *sql.DB, id, contributionType string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE transfers SET contribution_type = ?, updated_at = ? WHERE id = ?",
		contributionType, now, id)
	if err != nil {
		return fmt.Errorf("store: update contribution: %w", err)
	}
	return nil
}

// scanner is the common interface between *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTransfer(s scanner) (*domain.Transfer, error) {
	t := &domain.Transfer{}
	var amountCents int64
	var status string
	var riskScore sql.NullInt64
	var createdAt, updatedAt string
	err := s.Scan(
		&t.ID, &t.InvestorAccountID, &t.CorrespondentID,
		&amountCents, &status,
		&t.VendorTransactionID, &t.CheckNumber,
		&t.MICRRouting, &t.MICRAccount, &t.MICRData,
		&riskScore, &t.ContributionType, &t.SettlementBatchID,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Amount = domain.Amount(amountCents)
	t.Status = domain.State(status)
	if riskScore.Valid {
		rs := int(riskScore.Int64)
		t.RiskScore = &rs
	}
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return t, nil
}

const transferColumns = `id, investor_account_id, correspondent_id, amount, status,
	COALESCE(vendor_transaction_id,''), check_number,
	COALESCE(micr_routing,''), COALESCE(micr_account,''), COALESCE(micr_data,''),
	risk_score, COALESCE(contribution_type,''), COALESCE(settlement_batch_id,''),
	created_at, updated_at`

// GetTransfer loads a transfer by ID. Returns ErrNotFound if the transfer does not exist.
func GetTransfer(db *sql.DB, id string) (*domain.Transfer, error) {
	q := `SELECT ` + transferColumns + ` FROM transfers WHERE id = ?`
	t, err := scanTransfer(db.QueryRow(q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get transfer %s: %w", id, err)
	}
	return t, nil
}

// TransferFilters holds optional query filters for ListTransfers.
type TransferFilters struct {
	Status    string
	AccountID string
	From      string // YYYY-MM-DD or RFC3339
	To        string
}

// ValidateDateFilters returns an error if From or To are non-empty but not valid date strings.
func (f *TransferFilters) ValidateDateFilters() error {
	if f.From != "" && !isValidDate(f.From) {
		return fmt.Errorf("invalid 'from' date format: %q (expected YYYY-MM-DD)", f.From)
	}
	if f.To != "" && !isValidDate(f.To) {
		return fmt.Errorf("invalid 'to' date format: %q (expected YYYY-MM-DD)", f.To)
	}
	return nil
}

func isValidDate(s string) bool {
	layouts := []string{"2006-01-02", "2006-01-02T15:04:05Z", time.RFC3339}
	for _, l := range layouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}

// ListTransfers returns transfers matching the optional filters, newest first.
func ListTransfers(db *sql.DB, f TransferFilters) ([]*domain.Transfer, error) {
	q := `SELECT ` + transferColumns + ` FROM transfers WHERE 1=1`
	var args []interface{}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.AccountID != "" {
		q += " AND investor_account_id = ?"
		args = append(args, f.AccountID)
	}
	if f.From != "" {
		q += " AND created_at >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND created_at <= ?"
		args = append(args, f.To)
	}
	q += " ORDER BY created_at DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list transfers: %w", err)
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
