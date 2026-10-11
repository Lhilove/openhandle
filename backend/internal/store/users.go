package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"openhandle/internal/models"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a uniqueness constraint is violated.
var ErrConflict = errors.New("conflict")

func scanUser(row pgx.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a new user. A duplicate email returns ErrConflict.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error) {
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1,$2)
		 RETURNING id, email, password_hash, status, created_at, updated_at`,
		email, passwordHash)
	u, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return u, nil
}

// GetUserByEmail fetches a user by case-insensitive email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT id, email, password_hash, status, created_at, updated_at
		 FROM users WHERE lower(email)=lower($1)`, email)
	return scanUser(row)
}

// GetUserByID fetches a user by id.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT id, email, password_hash, status, created_at, updated_at
		 FROM users WHERE id=$1`, id)
	return scanUser(row)
}

// CreateSession stores a session keyed by the hash of the raw cookie token.
func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash, csrfToken, userAgent, ip string, expiresAt time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO sessions (user_id, token_hash, csrf_token, user_agent, ip, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		userID, tokenHash, csrfToken, userAgent, ip, expiresAt).Scan(&id)
	return id, err
}

// GetSessionByTokenHash loads a live (unexpired) session.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	var sess models.Session
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_id, csrf_token, expires_at
		 FROM sessions WHERE token_hash=$1 AND expires_at > now()`, tokenHash).
		Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// DeleteSession removes a single session (logout).
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}

// DeleteSessionsForUser removes all sessions for a user (logout everywhere).
func (s *Store) DeleteSessionsForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}

// DeleteExpiredSessions prunes expired sessions.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}
