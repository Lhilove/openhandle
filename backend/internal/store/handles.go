package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"openhandle/internal/models"
)

const handleCols = `id, user_id, handle, display_name, bio, is_public, created_at, updated_at, released_at`

func scanHandle(row pgx.Row) (*models.Handle, error) {
	var h models.Handle
	err := row.Scan(&h.ID, &h.UserID, &h.Handle, &h.DisplayName, &h.Bio, &h.IsPublic, &h.CreatedAt, &h.UpdatedAt, &h.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// HandleExists reports whether an active handle is taken.
func (s *Store) HandleExists(ctx context.Context, handle string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM handles WHERE handle=$1 AND released_at IS NULL)`, handle).Scan(&exists)
	return exists, err
}

// GetHandle returns an active handle by its normalized value.
func (s *Store) GetHandle(ctx context.Context, handle string) (*models.Handle, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+handleCols+` FROM handles WHERE handle=$1 AND released_at IS NULL`, handle)
	return scanHandle(row)
}

// GetActiveHandleForUser returns a user's current active handle.
func (s *Store) GetActiveHandleForUser(ctx context.Context, userID uuid.UUID) (*models.Handle, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+handleCols+` FROM handles WHERE user_id=$1 AND released_at IS NULL`, userID)
	return scanHandle(row)
}

// ClaimHandle atomically creates a handle for a user. Uniqueness and the
// one-handle-per-user rule are enforced by database indexes, so concurrent
// claims of the same handle resolve to exactly one winner.
func (s *Store) ClaimHandle(ctx context.Context, userID uuid.UUID, handle string) (*models.Handle, error) {
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO handles (user_id, handle) VALUES ($1,$2)
		 RETURNING `+handleCols, userID, handle)
	h, err := scanHandle(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return h, nil
}

// UpdateHandleProfile updates mutable profile fields for a user's handle.
func (s *Store) UpdateHandleProfile(ctx context.Context, userID uuid.UUID, displayName, bio *string, isPublic *bool) (*models.Handle, error) {
	row := s.Pool.QueryRow(ctx,
		`UPDATE handles SET
			display_name = COALESCE($2, display_name),
			bio = COALESCE($3, bio),
			is_public = COALESCE($4, is_public),
			updated_at = now()
		 WHERE user_id=$1 AND released_at IS NULL
		 RETURNING `+handleCols,
		userID, displayName, bio, isPublic)
	h, err := scanHandle(row)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// ReleaseHandleForUser marks a user's active handle as released, freeing it.
func (s *Store) ReleaseHandleForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE handles SET released_at=now() WHERE user_id=$1 AND released_at IS NULL`, userID)
	return err
}
