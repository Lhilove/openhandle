package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides data access. All queries use parameterized SQL.
type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// Audit writes an audit event. metadata must never contain secrets.
func (s *Store) Audit(ctx context.Context, actor *uuid.UUID, eventType, targetType, targetID string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		raw = []byte("{}")
	}
	_, _ = s.Pool.Exec(ctx,
		`INSERT INTO audit_events (actor_user_id, event_type, target_type, target_id, metadata)
		 VALUES ($1,$2,$3,$4,$5)`,
		actor, eventType, targetType, targetID, raw)
}
