package api_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

// historyBody is the shape of the history endpoint this file asserts on.
type historyBody struct {
	Transactions []struct {
		Signature          string `json:"signature"`
		Status             string `json:"status"`
		Direction          string `json:"direction"`
		CounterpartyHandle string `json:"counterparty_handle"`
		CounterpartyAddr   string `json:"counterparty_address"`
	} `json:"transactions"`
	Total int    `json:"total"`
	Scope string `json:"scope"`
}

// A payment has two sides. The sender's client is what records it, so before
// this the recipient had no record of a tip at all -- the delivery was invisible
// to the person who was paid, and its status only ever existed for the sender.
func TestPaymentHistoryShowsBothSidesOfAPayment(t *testing.T) {
	e := newEnv(t)

	// The recipient: a public handle and a wallet to be paid into.
	recipient := e.newClient()
	recipient.register("receive@example.com", "supersecret123")
	recipient.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "tipped"}, true)
	rpub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipientAddr := base58.Encode(rpub)
	if _, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "receive@example.com"), "solana", recipientAddr, nil); err != nil {
		t.Fatalf("add recipient wallet: %v", err)
	}

	// The sender, with a handle of their own so the recipient can see who paid.
	sender := e.newClient()
	sender.register("sending@example.com", "supersecret123")
	sender.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "tipper"}, true)
	spub, _, _ := ed25519.GenerateKey(rand.Reader)
	senderAddr := base58.Encode(spub)
	if _, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "sending@example.com"), "solana", senderAddr, nil); err != nil {
		t.Fatalf("add sender wallet: %v", err)
	}

	// An unrelated account, which must see none of this.
	other := e.newClient()
	other.register("other@example.com", "supersecret123")

	sig := validSignature("bothsides")
	rec := sender.do(http.MethodPost, "/api/v1/transactions", map[string]string{
		"signature":         sig,
		"recipient_handle":  "tipped",
		"amount_base_units": "10000000",
		"asset":             "SOL",
		"sender_address":    senderAddr,
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit payment: status %d body %s", rec.Code, rec.Body.String())
	}

	// The chain confirms it. Both sides read the same record, so neither can
	// report a different status than the other.
	chain := &fakeChain{seen: map[string]fakeTx{}}
	chain.land(sig, recipientAddr, 10000000, 777)
	newChainServer(t, e, chain).ConfirmPending(t.Context())

	sent := decode[historyBody](t, sender.do(http.MethodGet, "/api/v1/me/transactions", nil, false))
	if sent.Total != 1 || len(sent.Transactions) != 1 {
		t.Fatalf("sender history: expected 1 payment, got %d", sent.Total)
	}
	if got := sent.Transactions[0]; got.Direction != "sent" || got.Status != "confirmed" {
		t.Fatalf("sender sees the payment as %q/%q, want sent/confirmed", got.Direction, got.Status)
	}
	if sent.Transactions[0].CounterpartyHandle != "tipped" {
		t.Fatalf("sender should see who they paid, got handle %q", sent.Transactions[0].CounterpartyHandle)
	}

	received := decode[historyBody](t, recipient.do(http.MethodGet, "/api/v1/me/transactions", nil, false))
	if received.Total != 1 || len(received.Transactions) != 1 {
		t.Fatalf("recipient history: expected the payment they were sent, got %d", received.Total)
	}
	got := received.Transactions[0]
	if got.Direction != "received" {
		t.Fatalf("recipient sees the payment as %q, want received", got.Direction)
	}
	// Same transaction, same status: that is the whole point of one record.
	if got.Status != sent.Transactions[0].Status {
		t.Fatalf("status disagrees between sender (%q) and recipient (%q)", sent.Transactions[0].Status, got.Status)
	}
	if got.CounterpartyHandle != "tipper" {
		t.Fatalf("recipient should see who paid them, got handle %q", got.CounterpartyHandle)
	}
	if got.CounterpartyAddr != senderAddr {
		t.Fatalf("recipient counterparty address %q, want the sender's %q", got.CounterpartyAddr, senderAddr)
	}

	// The recipient may read the single payment they received; a stranger may not.
	if r := recipient.do(http.MethodGet, "/api/v1/transactions/"+sig, nil, false); r.Code != http.StatusOK {
		t.Fatalf("recipient read of their own payment: status %d body %s", r.Code, r.Body.String())
	}
	if r := other.do(http.MethodGet, "/api/v1/transactions/"+sig, nil, false); r.Code != http.StatusNotFound {
		t.Fatalf("a stranger must not read the payment, got %d", r.Code)
	}
	if r := other.do(http.MethodGet, "/api/v1/me/transactions", nil, false); r.Code != http.StatusOK {
		t.Fatalf("other history: status %d", r.Code)
	} else if body := decode[historyBody](t, r); body.Total != 0 {
		t.Fatalf("an unrelated account must see no payments, got %d", body.Total)
	}
}

// A payment recorded before the recipient's handle was linked -- or paid to an
// address they later connected -- still belongs in their history, so a delivery
// is never lost just because the link is missing.
func TestPaymentHistoryAttributesIncomingByWalletAddress(t *testing.T) {
	e := newEnv(t)

	recipient := e.newClient()
	recipient.register("walletlink@example.com", "supersecret123")
	rpub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipientAddr := base58.Encode(rpub)
	if _, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "walletlink@example.com"), "solana", recipientAddr, nil); err != nil {
		t.Fatalf("add wallet: %v", err)
	}

	// No handle link and no creator: the only thing tying this row to the
	// recipient is the address the funds landed in.
	const sig = "5walletLinkedIncomingPaymentForHistoryTest1111111"
	if _, err := e.st.Pool.Exec(t.Context(),
		`INSERT INTO transactions (network, signature, sender_address, recipient_address,
		   amount_base_units, asset_identifier, status)
		 VALUES ('devnet', $1, 'FdbSXirVHx7GtuAMno4qJx2q9zq7QBHvyhjqpcDyYu4k', $2, 5000, 'SOL', 'confirmed')`,
		sig, recipientAddr); err != nil {
		t.Fatalf("seed incoming payment: %v", err)
	}

	body := decode[historyBody](t, recipient.do(http.MethodGet, "/api/v1/me/transactions", nil, false))
	if body.Total != 1 {
		t.Fatalf("expected the incoming payment to appear, got %d", body.Total)
	}
	if body.Transactions[0].Direction != "received" {
		t.Fatalf("direction %q, want received", body.Transactions[0].Direction)
	}
	if body.Transactions[0].Status != "confirmed" {
		t.Fatalf("status %q, want confirmed", body.Transactions[0].Status)
	}
}

// A counterparty handle its owner has hidden must not leak back through payment
// history: the address stays, the handle does not.
func TestPaymentHistoryHidesPrivateCounterpartyHandle(t *testing.T) {
	e := newEnv(t)

	recipient := e.newClient()
	recipient.register("private@example.com", "supersecret123")
	recipient.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "hiddenone"}, true)
	rpub, _, _ := ed25519.GenerateKey(rand.Reader)
	recipientAddr := base58.Encode(rpub)
	if _, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "private@example.com"), "solana", recipientAddr, nil); err != nil {
		t.Fatalf("add wallet: %v", err)
	}

	sender := e.newClient()
	sender.register("payingsender@example.com", "supersecret123")
	sender.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "openpayer"}, true)

	sig := validSignature("privatehandle")
	rec := sender.do(http.MethodPost, "/api/v1/transactions", map[string]string{
		"signature":         sig,
		"recipient_handle":  "hiddenone",
		"amount_base_units": "2500000",
		"asset":             "SOL",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: status %d body %s", rec.Code, rec.Body.String())
	}
	// The recipient hides their profile afterwards.
	recipient.do(http.MethodPatch, "/api/v1/me/profile", map[string]any{"is_public": false}, true)

	body := decode[historyBody](t, recipient.do(http.MethodGet, "/api/v1/me/transactions", nil, false))
	if body.Total != 1 {
		t.Fatalf("hiding a profile must not erase the payment received, got %d", body.Total)
	}
	if body.Transactions[0].CounterpartyHandle != "openpayer" {
		t.Fatalf("expected the sender's handle, got %q", body.Transactions[0].CounterpartyHandle)
	}

	// The sender's own history must not expose a handle that is now private.
	sentBody := decode[historyBody](t, sender.do(http.MethodGet, "/api/v1/me/transactions", nil, false))
	if sentBody.Total != 1 {
		t.Fatalf("sender history: expected 1, got %d", sentBody.Total)
	}
	if h := sentBody.Transactions[0].CounterpartyHandle; h != "" {
		t.Fatalf("a private counterparty handle must not be exposed, got %q", h)
	}
	if sentBody.Transactions[0].CounterpartyAddr != recipientAddr {
		t.Fatalf("expected the recipient address for display, got %q", sentBody.Transactions[0].CounterpartyAddr)
	}
}

// The scope line is user-visible copy: it must say history covers both sides.
func TestPaymentHistoryScopeMentionsBothSides(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("scope@example.com", "supersecret123")

	rec := c.do(http.MethodGet, "/api/v1/me/transactions", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status %d", rec.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	scope, _ := raw["scope"].(string)
	lowered := strings.ToLower(scope)
	for _, want := range []string{"sent", "received"} {
		if !strings.Contains(lowered, want) {
			t.Fatalf("scope %q should mention %q", scope, want)
		}
	}
}
