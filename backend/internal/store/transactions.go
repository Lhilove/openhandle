package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"openhandle/internal/models"
)

const txCols = `id, network, signature, sender_address, recipient_address,
	amount_base_units::text, asset_identifier, status, slot, submitted_at,
	confirmed_at, last_checked_at, created_by_user_id, sender_handle_id,
	recipient_handle_id, created_at`

func scanTx(row pgx.Row) (*models.Transaction, error) {
	var t models.Transaction
	err := row.Scan(&t.ID, &t.Network, &t.Signature, &t.SenderAddress, &t.RecipientAddress,
		&t.AmountBaseUnits, &t.AssetIdentifier, &t.Status, &t.Slot, &t.SubmittedAt,
		&t.ConfirmedAt, &t.LastCheckedAt, &t.CreatedByUserID, &t.SenderHandleID,
		&t.RecipientHandleID, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// IngestTransaction records a submitted transaction idempotently. Re-submitting
// the same (network, signature) returns the existing row and created=false, so
// internal records are never double-counted.
func (s *Store) IngestTransaction(ctx context.Context, t models.Transaction) (*models.Transaction, bool, error) {
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO transactions
		   (network, signature, sender_address, recipient_address, amount_base_units,
		    asset_identifier, status, created_by_user_id, sender_handle_id, recipient_handle_id)
		 VALUES ($1,$2,$3,$4,$5::numeric,$6,$7,$8,$9,$10)
		 ON CONFLICT (network, signature) DO NOTHING
		 RETURNING `+txCols,
		t.Network, t.Signature, t.SenderAddress, t.RecipientAddress, t.AmountBaseUnits,
		t.AssetIdentifier, t.Status, t.CreatedByUserID, t.SenderHandleID, t.RecipientHandleID)

	created, err := scanTx(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	// Conflict: return the existing row.
	existing, err := s.GetTransactionBySignature(ctx, t.Network, t.Signature)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

// GetTransactionBySignature loads a transaction by network + signature.
func (s *Store) GetTransactionBySignature(ctx context.Context, network, signature string) (*models.Transaction, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+txCols+` FROM transactions WHERE network=$1 AND signature=$2`, network, signature)
	return scanTx(row)
}

// ListUserTransactions returns transactions initiated by a user, newest first.
func (s *Store) ListUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Transaction, int, error) {
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE created_by_user_id=$1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT `+txCols+` FROM transactions WHERE created_by_user_id=$1
		 ORDER BY submitted_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

// UpdateTransactionStatus transitions a transaction's status conservatively.
// A confirmed transaction is never moved to failed, and a failed transaction is
// never re-confirmed, to tolerate transient RPC inconsistency.
func (s *Store) UpdateTransactionStatus(ctx context.Context, id uuid.UUID, status string, slot *int64) error {
	var confirmedAt any
	if status == "confirmed" {
		confirmedAt = time.Now()
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE transactions SET
			status = CASE
				WHEN status='confirmed' AND $2 <> 'confirmed' THEN status
				ELSE $2
			END,
			slot = COALESCE($3, slot),
			confirmed_at = COALESCE(confirmed_at, $4),
			last_checked_at = now()
		 WHERE id=$1`,
		id, status, slot, confirmedAt)
	return err
}

// ListPendingTransactions returns transactions still awaiting confirmation.
func (s *Store) ListPendingTransactions(ctx context.Context, limit int) ([]models.Transaction, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+txCols+` FROM transactions WHERE status='submitted'
		 ORDER BY submitted_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}
