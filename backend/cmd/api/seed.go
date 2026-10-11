package main

import (
	"context"
	"errors"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"openhandle/internal/store"
)

// seedDemo creates a clearly-labelled development demo account. It is only
// invoked when SEED_DEMO=true and the app is not in production. It creates no
// wallets, identities, transactions or balances -- so nothing in the UI is
// fabricated.
func seedDemo(ctx context.Context, st *store.Store, log *slog.Logger) error {
	const email = "demo@example.com"
	const password = "demo-password-123"
	const handle = "demo"

	if _, err := st.GetUserByEmail(ctx, email); err == nil {
		log.Info("demo seed already present")
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user, err := st.CreateUser(ctx, email, string(hash))
	if err != nil {
		return err
	}
	if _, err := st.ClaimHandle(ctx, user.ID, handle); err != nil {
		return err
	}
	log.Warn("DEMO seed created (development only)",
		"email", email, "password", password, "handle", handle)
	return nil
}
