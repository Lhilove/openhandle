package models

import (
	"time"

	"github.com/google/uuid"
)

// User is an account. The internal id is never exposed through public APIs.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Handle is a claimed public handle. Handles are immutable in the MVP; a row is
// marked released (released_at set) when a handle is freed, e.g. on account
// deletion.
type Handle struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Handle      string
	DisplayName *string
	Bio         *string
	IsPublic    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ReleasedAt  *time.Time
}

// Wallet is a connected self-custody wallet. Only public addresses are stored.
type Wallet struct {
	ID                  uuid.UUID
	UserID              uuid.UUID
	Network             string
	Address             string
	IsPrimary           bool
	OwnershipVerifiedAt *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Transaction records a payment that was initiated through OpenHandle.
type Transaction struct {
	ID                uuid.UUID
	Network           string
	Signature         string
	SenderAddress     *string
	RecipientAddress  string
	AmountBaseUnits   string // decimal string, never a float
	AssetIdentifier   string
	Status            string
	Slot              *int64
	SubmittedAt       time.Time
	ConfirmedAt       *time.Time
	LastCheckedAt     *time.Time
	CreatedByUserID   *uuid.UUID
	SenderHandleID    *uuid.UUID
	RecipientHandleID *uuid.UUID
	CreatedAt         time.Time
}

// Direction describes which side of a payment the viewer is on.
const (
	DirectionSent     = "sent"
	DirectionReceived = "received"
)

// Payment is a transaction seen from one user's side, with the counterparty
// resolved for display. A payment has two sides and each side sees the same
// record, so a sender and a recipient can never disagree about its status.
type Payment struct {
	Transaction
	Direction string
	// CounterpartyHandle is the other side's handle, present only when that
	// handle is public and still active. The address is always present.
	CounterpartyHandle  *string
	CounterpartyAddress string
}

// IdentityConnection is a verified link to an external account. Only stored
// after a real provider verification; tokens are not retained.
type IdentityConnection struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Provider         string
	ProviderSubject  string
	ProviderUsername string
	VerifiedAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Session is a server-side session. Only a hash of the token is stored.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
}
