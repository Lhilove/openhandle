package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"openhandle/internal/models"
)

// ListIdentities returns a user's verified identity connections.
func (s *Store) ListIdentities(ctx context.Context, userID uuid.UUID) ([]models.IdentityConnection, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, user_id, provider, provider_subject, provider_username, verified_at, created_at, updated_at
		 FROM identity_connections WHERE user_id=$1 ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.IdentityConnection
	for rows.Next() {
		var ic models.IdentityConnection
		if err := rows.Scan(&ic.ID, &ic.UserID, &ic.Provider, &ic.ProviderSubject, &ic.ProviderUsername, &ic.VerifiedAt, &ic.CreatedAt, &ic.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ic)
	}
	return out, rows.Err()
}

// UpsertIdentity records a verified identity connection.
func (s *Store) UpsertIdentity(ctx context.Context, userID uuid.UUID, provider, subject, username string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO identity_connections (user_id, provider, provider_subject, provider_username)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (user_id, provider) DO UPDATE
		   SET provider_subject=EXCLUDED.provider_subject,
		       provider_username=EXCLUDED.provider_username,
		       verified_at=now(), updated_at=now()`,
		userID, provider, subject, username)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// DeleteIdentity removes a user's connection to a provider.
func (s *Store) DeleteIdentity(ctx context.Context, userID uuid.UUID, provider string) error {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM identity_connections WHERE user_id=$1 AND provider=$2`, userID, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetIdentityByProvider returns a user's connection to one provider.
func (s *Store) GetIdentityByProvider(ctx context.Context, userID uuid.UUID, provider string) (*models.IdentityConnection, error) {
	var ic models.IdentityConnection
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_id, provider, provider_subject, provider_username, verified_at, created_at, updated_at
		 FROM identity_connections WHERE user_id=$1 AND provider=$2`, userID, provider).
		Scan(&ic.ID, &ic.UserID, &ic.Provider, &ic.ProviderSubject, &ic.ProviderUsername, &ic.VerifiedAt, &ic.CreatedAt, &ic.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ic, nil
}
