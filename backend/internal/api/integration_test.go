package api_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mr-tron/base58"

	"openhandle/internal/api"
	"openhandle/internal/config"
	"openhandle/internal/db"
	"openhandle/internal/rpc"
	"openhandle/internal/store"
)

// These tests require PostgreSQL. Set TEST_DATABASE_URL (and optionally
// APP_ENV=test) to run them; otherwise they are skipped explicitly rather than
// silently passing.
func testDBURL(t *testing.T) string {
	t.Helper()
	u := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if u == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}
	return u
}

type env struct {
	t      *testing.T
	router http.Handler
	st     *store.Store
}

func newEnv(t *testing.T) *env { return newEnvWith(t, nil) }

// newEnvWith builds the same environment as newEnv, after letting the caller
// adjust configuration (cookie policy, client-IP mode, ...).
func newEnvWith(t *testing.T, mutate func(*config.Config)) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Env:                "test",
		AppBaseURL:         "http://localhost:5173",
		APIBaseURL:         "http://localhost:8090",
		SessionSecret:      "test-secret-test-secret-test-secret-0000",
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		SolanaCluster:      "devnet",
		SolanaRPCURL:       "http://127.0.0.1:1/", // unreachable: RPC calls fail fast
		LogLevel:           "error",
		CookieSameSite:     "lax",
		ClientIPMode:       "remote",
	}
	if mutate != nil {
		mutate(cfg)
	}

	ctx := t.Context()
	pool, err := db.Connect(ctx, testDBURL(t))
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, err = pool.Exec(ctx, `TRUNCATE transactions, wallet_challenges, identity_connections, wallets, handles, sessions, users, audit_events RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	st := store.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rpcClient := rpc.New(cfg.SolanaRPCURL, cfg.SolanaCluster)
	return &env{t: t, router: api.Router(cfg, st, logger, rpcClient), st: st}
}

type client struct {
	t      *testing.T
	h      http.Handler
	cookie string
	csrf   string
}

func (e *env) newClient() *client { return &client{t: e.t, h: e.router} }

func (c *client) do(method, path string, body any, csrf bool) *httptest.ResponseRecorder {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	if csrf {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func (c *client) register(email, password string) {
	c.t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{"email": email, "password": password}, false)
	if rec.Code != http.StatusCreated {
		c.t.Fatalf("register: status %d body %s", rec.Code, rec.Body.String())
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "oh_session" {
			c.cookie = ck.Name + "=" + ck.Value
		}
	}
	var out struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	c.csrf = out.CSRFToken
}

// validSignature builds a distinct signature-shaped string: base58 over 64
// bytes, which is the only thing a Solana RPC accepts.
func validSignature(seed string) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = seed[i%len(seed)]
	}
	return base58.Encode(b)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestAuthAndMe(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("a@example.com", "supersecret123")

	rec := c.do(http.MethodGet, "/api/v1/me", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status %d", rec.Code)
	}
	me := decode[map[string]any](t, rec)
	if me["email"] != "a@example.com" {
		t.Fatalf("unexpected me payload: %v", me)
	}
	if _, leaked := me["password_hash"]; leaked {
		t.Fatal("me must not leak password hash")
	}

	// Unauthenticated access is rejected.
	anon := e.newClient()
	if r := anon.do(http.MethodGet, "/api/v1/me", nil, false); r.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous /me, got %d", r.Code)
	}
}

func TestDuplicateEmailRejected(t *testing.T) {
	e := newEnv(t)
	c1 := e.newClient()
	c1.register("dup@example.com", "supersecret123")

	c2 := e.newClient()
	rec := c2.do(http.MethodPost, "/api/v1/auth/register", map[string]string{"email": "DUP@example.com", "password": "supersecret123"}, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate email, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleClaimReservedAndDuplicate(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("h@example.com", "supersecret123")

	// Reserved handle is rejected by validation.
	rec := c.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "admin"}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for reserved handle, got %d", rec.Code)
	}

	// Valid claim.
	rec = c.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "Alice_1"}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for claim, got %d: %s", rec.Code, rec.Body.String())
	}

	// Immutable: second claim conflicts.
	rec = c.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "other"}, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for second handle, got %d", rec.Code)
	}
}

func TestHandleConcurrentClaimSingleWinner(t *testing.T) {
	e := newEnv(t)
	// Create several users, then have them all race for the same handle.
	// Kept within the auth rate-limit burst so registration itself is not throttled.
	const n = 5
	clients := make([]*client, n)
	for i := 0; i < n; i++ {
		c := e.newClient()
		c.register(fmt.Sprintf("race%d@example.com", i), "supersecret123")
		clients[i] = c
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(c *client) {
			defer wg.Done()
			rec := c.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "wanted"}, true)
			if rec.Code == http.StatusCreated {
				mu.Lock()
				created++
				mu.Unlock()
			}
		}(clients[i])
	}
	wg.Wait()

	if created != 1 {
		t.Fatalf("expected exactly one winner, got %d", created)
	}
}

func TestPublicResolutionPrivacy(t *testing.T) {
	e := newEnv(t)

	// User A claims a public handle but has no wallet.
	a := e.newClient()
	a.register("pub@example.com", "supersecret123")
	a.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "pubuser"}, true)

	rec := a.do(http.MethodGet, "/api/v1/handles/pubuser", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 resolve, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"\"id\"", "user_id", "email", "password"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public resolution leaked %q: %s", forbidden, body)
		}
	}

	// Make the profile private and confirm it is no longer resolvable.
	priv := false
	a.do(http.MethodPatch, "/api/v1/me/profile", map[string]any{"is_public": priv}, true)
	rec = a.do(http.MethodGet, "/api/v1/handles/pubuser", nil, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for private handle, got %d", rec.Code)
	}
}

func TestCrossUserWalletAccess(t *testing.T) {
	e := newEnv(t)
	a := e.newClient()
	a.register("wa@example.com", "supersecret123")
	b := e.newClient()
	b.register("wb@example.com", "supersecret123")

	// Give A a wallet directly through the store (verified).
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	added, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "wa@example.com"), "solana", base58.Encode(pub), nil)
	if err != nil {
		t.Fatalf("add wallet: %v", err)
	}

	// B must not be able to delete A's wallet.
	rec := b.do(http.MethodDelete, "/api/v1/me/wallets/"+added.ID.String(), nil, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user delete, got %d", rec.Code)
	}
}

func mustUserID(t *testing.T, e *env, email string) uuid.UUID {
	t.Helper()
	u, err := e.st.GetUserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	return u.ID
}

func TestWalletChallengeReplayAndTamper(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("w@example.com", "supersecret123")

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := base58.Encode(pub)

	rec := c.do(http.MethodPost, "/api/v1/wallet-challenges", map[string]string{"address": addr}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("challenge status %d: %s", rec.Code, rec.Body.String())
	}
	ch := decode[struct {
		Message string `json:"message"`
		Nonce   string `json:"nonce"`
	}](t, rec)

	sig := base58.Encode(ed25519.Sign(priv, []byte(ch.Message)))
	verifyBody := map[string]string{"address": addr, "nonce": ch.Nonce, "signature": sig}

	rec = c.do(http.MethodPost, "/api/v1/wallets/verify", verifyBody, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status %d: %s", rec.Code, rec.Body.String())
	}

	// Replay must fail (single-use challenge).
	rec = c.do(http.MethodPost, "/api/v1/wallets/verify", verifyBody, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected replay to fail with 400, got %d", rec.Code)
	}
}

func TestWalletChallengeBadSignature(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("w2@example.com", "supersecret123")

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	addr := base58.Encode(pub)

	rec := c.do(http.MethodPost, "/api/v1/wallet-challenges", map[string]string{"address": addr}, true)
	ch := decode[struct {
		Message string `json:"message"`
		Nonce   string `json:"nonce"`
	}](t, rec)

	badSig := base58.Encode(ed25519.Sign(otherPriv, []byte(ch.Message)))
	rec = c.do(http.MethodPost, "/api/v1/wallets/verify", map[string]string{
		"address": addr, "nonce": ch.Nonce, "signature": badSig,
	}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad signature, got %d", rec.Code)
	}
}

func TestTransactionIdempotency(t *testing.T) {
	e := newEnv(t)

	// Recipient with a public handle and a wallet.
	receiver := e.newClient()
	receiver.register("rx@example.com", "supersecret123")
	receiver.do(http.MethodPost, "/api/v1/handles", map[string]string{"handle": "receiver"}, true)
	rpub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := e.st.AddWallet(t.Context(), mustUserID(t, e, "rx@example.com"), "solana", base58.Encode(rpub), nil); err != nil {
		t.Fatalf("add receiver wallet: %v", err)
	}

	sender := e.newClient()
	sender.register("tx@example.com", "supersecret123")

	body := map[string]string{
		"signature":         validSignature("idempotency"),
		"recipient_handle":  "receiver",
		"amount_base_units": "1000000",
		"asset":             "SOL",
	}
	rec1 := sender.do(http.MethodPost, "/api/v1/transactions", body, true)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first submit status %d: %s", rec1.Code, rec1.Body.String())
	}
	tx1 := decode[map[string]any](t, rec1)

	rec2 := sender.do(http.MethodPost, "/api/v1/transactions", body, true)
	if rec2.Code != http.StatusOK {
		t.Fatalf("duplicate submit should be 200, got %d", rec2.Code)
	}
	tx2 := decode[map[string]any](t, rec2)
	if tx1["id"] != tx2["id"] {
		t.Fatalf("idempotency violated: ids %v vs %v", tx1["id"], tx2["id"])
	}

	// History should contain exactly one record.
	rec := sender.do(http.MethodGet, "/api/v1/me/transactions", nil, false)
	hist := decode[struct {
		Total int `json:"total"`
	}](t, rec)
	if hist.Total != 1 {
		t.Fatalf("expected 1 transaction, got %d", hist.Total)
	}
}

func TestTransactionRejectsUnknownHandle(t *testing.T) {
	e := newEnv(t)
	s := e.newClient()
	s.register("tx2@example.com", "supersecret123")

	rec := s.do(http.MethodPost, "/api/v1/transactions", map[string]string{
		"signature":         validSignature("unknownhandle"),
		"recipient_handle":  "doesnotexist",
		"amount_base_units": "1000",
	}, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown recipient, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A malformed signature must never be stored: the status RPC rejects a whole
// batch that contains one bad entry, so it would stall every pending payment.
func TestTransactionRejectsMalformedSignature(t *testing.T) {
	e := newEnv(t)
	s := e.newClient()
	s.register("tx3@example.com", "supersecret123")

	for _, sig := range []string{
		"5VERYfakeSignatureWithIllegalCharacters1111111111111111", // 'I', 'O', 'l', '0' are not base58
		validSignature("short")[:40],                              // right alphabet, wrong size
		"",
	} {
		rec := s.do(http.MethodPost, "/api/v1/transactions", map[string]string{
			"signature":         sig,
			"recipient_handle":  "anyone",
			"amount_base_units": "1000",
		}, true)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("signature %q: expected 400, got %d: %s", sig, rec.Code, rec.Body.String())
		}
	}
}

func TestRateLimitLogin(t *testing.T) {
	e := newEnv(t)
	got := 0
	for i := 0; i < 30; i++ {
		c := e.newClient()
		rec := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": "nobody@example.com", "password": "wrongpassword"}, false)
		if rec.Code == http.StatusTooManyRequests {
			got++
		}
	}
	if got == 0 {
		t.Fatal("expected login rate limiting to trigger")
	}
}
