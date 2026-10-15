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

// ListUserTransactions returns a user's payments -- the ones they submitted and
// the ones paid to them -- newest first, as one timeline.
//
// A payment has two sides: the sender's client records it, and the recipient's
// account is the side the funds actually reached. Scoping history to
// created_by_user_id alone left a recipient with no record of a tip at all, so
// the delivery was invisible to the person who was paid.
func (s *Store) ListUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Payment, int, error) {
	var total int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM transactions t WHERE `+userPaymentVisibility, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT `+paymentCols+` FROM transactions t
		 LEFT JOIN handles sh ON sh.id = t.sender_handle_id AND sh.released_at IS NULL
		 LEFT JOIN handles rh ON rh.id = t.recipient_handle_id AND rh.released_at IS NULL
		 WHERE `+userPaymentVisibility+`
		 ORDER BY t.submitted_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Payment
	for rows.Next() {
		p, err := scanPayment(rows, userID)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, rows.Err()
}

// TransactionVisibleTo reports whether a user may read a single transaction:
// they sent it, or they received it. A recipient needs to read the record the
// sender created in order to see the payment arrive, and to see its status
// settle.
func (s *Store) TransactionVisibleTo(ctx context.Context, userID uuid.UUID, network, signature string) (bool, error) {
	var visible bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM transactions t
		  WHERE t.network=$2 AND t.signature=$3 AND `+userPaymentVisibility+`)`,
		userID, network, signature).Scan(&visible)
	return visible, err
}

// userPaymentVisibility is the predicate that decides which payments a user may
// see: the ones they sent, and the ones paid to them. It is shared by the
// history listing, its count and the single-transaction check so those three
// can never drift apart and leak or hide a record.
//
// $1 is always the viewing user's id. A payment reaches a user either because
// their handle was the recipient or because the funds landed in one of their
// wallets (a wallet linked after the payment was sent, say).
const userPaymentVisibility = `(
	t.created_by_user_id = $1
	OR t.recipient_handle_id IN (SELECT id FROM handles WHERE user_id = $1)
	OR EXISTS (SELECT 1 FROM wallets w WHERE w.user_id = $1 AND w.address = t.recipient_address)
)`

const paymentCols = `t.id, t.network, t.signature, t.sender_address, t.recipient_address,
	t.amount_base_units::text, t.asset_identifier, t.status, t.slot, t.submitted_at,
	t.confirmed_at, t.last_checked_at, t.created_by_user_id, t.sender_handle_id,
	t.recipient_handle_id, t.created_at, sh.handle, sh.is_public, rh.handle, rh.is_public`

// scanPayment reads one row of paymentCols and resolves it from the given
// viewer's side: the direction, the other party's address, and their handle when
// it is public.
func scanPayment(row pgx.Row, viewer uuid.UUID) (*models.Payment, error) {
	var (
		p                             models.Payment
		senderHandle, recipientHandle *string
		senderPublic, recipientPublic *bool
	)
	err := row.Scan(&p.ID, &p.Network, &p.Signature, &p.SenderAddress, &p.RecipientAddress,
		&p.AmountBaseUnits, &p.AssetIdentifier, &p.Status, &p.Slot, &p.SubmittedAt,
		&p.ConfirmedAt, &p.LastCheckedAt, &p.CreatedByUserID, &p.SenderHandleID,
		&p.RecipientHandleID, &p.CreatedAt, &senderHandle, &senderPublic,
		&recipientHandle, &recipientPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if p.CreatedByUserID != nil && *p.CreatedByUserID == viewer {
		p.Direction = models.DirectionSent
		p.CounterpartyAddress = p.RecipientAddress
		p.CounterpartyHandle = publicHandle(recipientHandle, recipientPublic)
	} else {
		p.Direction = models.DirectionReceived
		if p.SenderAddress != nil {
			p.CounterpartyAddress = *p.SenderAddress
		}
		p.CounterpartyHandle = publicHandle(senderHandle, senderPublic)
	}
	return &p, nil
}

// publicHandle exposes a counterparty's handle only while it is public and
// active. A handle that its owner has since hidden stays hidden, even from the
// person who paid them.
func publicHandle(handle *string, isPublic *bool) *string {
	if handle == nil || isPublic == nil || !*isPublic {
		return nil
	}
	return handle
}

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
//
// Besides freshly submitted rows it re-checks ones already marked expired, so a
// transaction that the node had not seen but that landed afterwards is still
// corrected. Those are polled slowly (every expiryRecheckInterval) because a
// signature that never reached the network never will.
//
// The window is a week rather than a day: an expired record can be wrong because
// the status endpoint was unreachable while the transfer went through, and such
// a record deserves to be repaired when the endpoint recovers.
func (s *Store) ListPendingTransactions(ctx context.Context, limit int) ([]models.Transaction, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+txCols+` FROM transactions
		 WHERE status='submitted'
		    OR (status='expired'
		        AND submitted_at > now() - interval '7 days'
		        AND (last_checked_at IS NULL OR last_checked_at < now() - interval '10 minutes'))
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
