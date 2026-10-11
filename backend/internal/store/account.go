package store

import (
	"context"

	"github.com/google/uuid"
)

// DeleteAccount deactivates and scrubs a user's account. The user row is
// retained in a tombstoned state so audit history and transaction records keep
// referential integrity, while personal data is removed:
//   - password hash cleared, email anonymised
//   - handle released (freed for reuse)
//   - wallets, identities, sessions and challenges removed
//
// On-chain transactions are public and permanent; rows are retained with the
// creator reference nulled by the FK's ON DELETE SET NULL only when the user
// row is removed. Here the row is kept but the account is disabled.
func (s *Store) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE users SET status='deleted',
			email = 'deleted+' || id::text || '@invalid.local',
			password_hash = '',
			updated_at = now()
		 WHERE id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE handles SET released_at=now() WHERE user_id=$1 AND released_at IS NULL`, userID); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM wallets WHERE user_id=$1`,
		`DELETE FROM identity_connections WHERE user_id=$1`,
		`DELETE FROM sessions WHERE user_id=$1`,
		`DELETE FROM wallet_challenges WHERE user_id=$1`,
	} {
		if _, err := tx.Exec(ctx, q, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
