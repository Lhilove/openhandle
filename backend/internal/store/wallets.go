package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"openhandle/internal/models"
)

const walletCols = `id, user_id, network, address, is_primary, ownership_verified_at, created_at, updated_at`

func scanWallet(row pgx.Row) (*models.Wallet, error) {
	var w models.Wallet
	err := row.Scan(&w.ID, &w.UserID, &w.Network, &w.Address, &w.IsPrimary, &w.OwnershipVerifiedAt, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWallets returns all wallets for a user, primary first.
func (s *Store) ListWallets(ctx context.Context, userID uuid.UUID) ([]models.Wallet, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE user_id=$1 ORDER BY is_primary DESC, created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Wallet
	for rows.Next() {
		w, err := scanWallet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// GetWalletForUser fetches one wallet scoped to its owner (object-level authz).
func (s *Store) GetWalletForUser(ctx context.Context, userID, walletID uuid.UUID) (*models.Wallet, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE id=$1 AND user_id=$2`, walletID, userID)
	return scanWallet(row)
}

// AddWallet attaches a wallet. If it is the user's first, it becomes primary.
func (s *Store) AddWallet(ctx context.Context, userID uuid.UUID, network, address string, verifiedAt *time.Time) (*models.Wallet, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM wallets WHERE user_id=$1`, userID).Scan(&count); err != nil {
		return nil, err
	}
	isPrimary := count == 0

	row := tx.QueryRow(ctx,
		`INSERT INTO wallets (user_id, network, address, is_primary, ownership_verified_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (network, address) DO UPDATE
		   SET ownership_verified_at = COALESCE(EXCLUDED.ownership_verified_at, wallets.ownership_verified_at),
		       updated_at = now()
		 WHERE wallets.user_id = EXCLUDED.user_id
		 RETURNING `+walletCols,
		userID, network, address, isPrimary, verifiedAt)

	w, err := scanWallet(row)
	if err != nil {
		if isUniqueViolation(err) || errors.Is(err, ErrNotFound) {
			return nil, ErrConflict
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// DeleteWallet removes a wallet owned by the user. If it was primary, another
// wallet of the same user is promoted.
func (s *Store) DeleteWallet(ctx context.Context, userID, walletID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var wasPrimary bool
	err = tx.QueryRow(ctx, `DELETE FROM wallets WHERE id=$1 AND user_id=$2 RETURNING is_primary`, walletID, userID).Scan(&wasPrimary)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if wasPrimary {
		_, err = tx.Exec(ctx,
			`UPDATE wallets SET is_primary=true, updated_at=now()
			 WHERE id = (SELECT id FROM wallets WHERE user_id=$1 ORDER BY created_at ASC LIMIT 1)`, userID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CreateChallenge stores a single-use wallet ownership challenge.
func (s *Store) CreateChallenge(ctx context.Context, userID uuid.UUID, address, nonceHash, domain, message string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO wallet_challenges (user_id, wallet_address, nonce_hash, domain, message, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		userID, address, nonceHash, domain, message, expiresAt)
	return err
}

// ConsumeChallenge atomically verifies and consumes a challenge, returning its
// message. A challenge can be consumed exactly once; reuse or expiry fails.
func (s *Store) ConsumeChallenge(ctx context.Context, userID uuid.UUID, address, nonce, domain string) (string, error) {
	var message string
	err := s.Pool.QueryRow(ctx,
		`UPDATE wallet_challenges SET consumed_at=now()
		 WHERE user_id=$1 AND wallet_address=$2 AND nonce_hash=$3 AND domain=$4
		   AND consumed_at IS NULL AND expires_at > now()
		 RETURNING message`,
		userID, address, nonce, domain).Scan(&message)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return message, nil
}

// DeleteExpiredChallenges prunes expired challenges.
func (s *Store) DeleteExpiredChallenges(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM wallet_challenges WHERE expires_at < now() - interval '1 day'`)
	return err
}
