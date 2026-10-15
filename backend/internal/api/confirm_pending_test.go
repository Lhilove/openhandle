package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"openhandle/internal/api"
	"openhandle/internal/config"
	"openhandle/internal/rpc"
)

// fakeTx is a transaction as the fake chain reports it.
type fakeTx struct {
	slot        int64
	err         bool
	destination string
	lamports    int64
}

// fakeChain is a minimal Solana JSON-RPC endpoint whose answers the test
// controls, so the confirmation worker can be exercised without a network.
type fakeChain struct {
	mu   sync.Mutex
	seen map[string]fakeTx
	// failTxLookup makes getTransaction fail while the status index keeps
	// answering -- what a provider does when it rate-limits one method and not
	// another. Nothing can be concluded about a transaction from a lookup that
	// never completed.
	failTxLookup bool
}

func (f *fakeChain) land(signature, destination string, lamports, slot int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen[signature] = fakeTx{slot: slot, destination: destination, lamports: lamports}
}

func writeRPC(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
}

func (f *fakeChain) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		f.mu.Lock()
		defer f.mu.Unlock()

		switch req.Method {
		case "getSignatureStatuses":
			var sigs []string
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params[0], &sigs)
			}
			values := make([]any, len(sigs))
			for i, sig := range sigs {
				tx, ok := f.seen[sig]
				if !ok {
					// The signature is unknown to this node: JSON null.
					continue
				}
				var errVal any
				if tx.err {
					errVal = map[string]any{"InstructionError": []any{0, map[string]any{"Custom": 1}}}
				}
				values[i] = map[string]any{
					"slot":               tx.slot,
					"confirmations":      nil,
					"confirmationStatus": "finalized",
					"err":                errVal,
				}
			}
			writeRPC(w, map[string]any{"context": map[string]any{"slot": 1}, "value": values})

		case "getTransaction":
			if f.failTxLookup {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			var sig string
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params[0], &sig)
			}
			tx, ok := f.seen[sig]
			if !ok {
				writeRPC(w, nil)
				return
			}
			var metaErr any
			if tx.err {
				metaErr = map[string]any{"InstructionError": []any{0, map[string]any{"Custom": 1}}}
			}
			writeRPC(w, map[string]any{
				"slot": tx.slot,
				"meta": map[string]any{"err": metaErr},
				"transaction": map[string]any{
					"message": map[string]any{
						"accountKeys": []any{map[string]any{"pubkey": tx.destination}},
						"instructions": []any{map[string]any{
							"program": "system",
							"parsed": map[string]any{
								"type": "transfer",
								"info": map[string]any{
									"source":      "sender",
									"destination": tx.destination,
									"lamports":    tx.lamports,
								},
							},
						}},
					},
				},
			})

		default:
			writeRPC(w, map[string]any{})
		}
	}
}

// newChainServer wires a Server against the fake chain, so ConfirmPending can be
// driven directly.
func newChainServer(t *testing.T, e *env, chain *fakeChain) *api.Server {
	t.Helper()
	ts := httptest.NewServer(chain.handler())
	t.Cleanup(ts.Close)
	cfg := &config.Config{
		Env:           "test",
		SessionSecret: "test-secret-test-secret-test-secret-0000",
		SolanaCluster: "devnet",
		SolanaRPCURL:  ts.URL,
		LogLevel:      "error",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewServer(cfg, e.st, logger, rpc.New(ts.URL, "devnet"))
}

// newFailingRPCServer wires a Server against an endpoint that answers every
// call with an authorization failure -- what a provider does when its API key
// is missing, expired or rejected. Nothing about the chain can be learned from
// it, so nothing about the chain may be concluded from it.
func newFailingRPCServer(t *testing.T, e *env) *api.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Unauthorized"))
	}))
	t.Cleanup(ts.Close)
	cfg := &config.Config{
		Env:           "test",
		SessionSecret: "test-secret-test-secret-test-secret-0000",
		SolanaCluster: "devnet",
		SolanaRPCURL:  ts.URL,
		LogLevel:      "error",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewServer(cfg, e.st, logger, rpc.New(ts.URL, "devnet"))
}

func seedTransaction(t *testing.T, e *env, signature, destination, age string) {
	t.Helper()
	_, err := e.st.Pool.Exec(t.Context(),
		`INSERT INTO transactions
		   (network, signature, recipient_address, amount_base_units, asset_identifier, status, submitted_at)
		 VALUES ('devnet', $1, $2, 10000000, 'SOL', 'submitted', now() - $3::interval)`,
		signature, destination, age)
	if err != nil {
		t.Fatalf("seed transaction %s: %v", signature, err)
	}
}

func loadTxStatus(t *testing.T, e *env, signature string) (string, *int64) {
	t.Helper()
	var status string
	var slot *int64
	err := e.st.Pool.QueryRow(t.Context(),
		`SELECT status, slot FROM transactions WHERE signature = $1`, signature).Scan(&status, &slot)
	if err != nil {
		t.Fatalf("load transaction %s: %v", signature, err)
	}
	return status, slot
}

// A transaction the network never accepted must not be reported as pending
// forever: past the blockhash window it is over, and the sender's funds never
// moved. A young one must be left alone, because the network may simply be slow.
func TestConfirmPendingDeclaresDroppedTransactionNotSent(t *testing.T) {
	e := newEnv(t)
	chain := &fakeChain{seen: map[string]fakeTx{}}
	srv := newChainServer(t, e, chain)

	const dropped = "5droppedSignatureForConfirmPendingTest111111111111"
	const young = "5youngSignatureForConfirmPendingTest1111111111111"
	const dest = "BGN4PHEuETe1ZKKsMPumALst8o3ZFibMw8TCwxcKcpKV"
	seedTransaction(t, e, dropped, dest, "10 minutes")
	seedTransaction(t, e, young, dest, "5 seconds")

	srv.ConfirmPending(t.Context())

	if status, _ := loadTxStatus(t, e, dropped); status != "expired" {
		t.Fatalf("a transaction the chain has never seen past the blockhash window should be expired, got %q", status)
	}
	if status, _ := loadTxStatus(t, e, young); status != "submitted" {
		t.Fatalf("a just-submitted transaction must stay pending, got %q", status)
	}
}

// An endpoint that cannot answer says nothing about the transaction. A rejected
// credential once turned a payment that had already finalized into "Not sent",
// telling the sender their money had never left the wallet: the worst possible
// lie. Past the blockhash window the record must stay pending until the chain
// itself is actually asked, however long that takes.
func TestConfirmPendingKeepsRecordPendingWhenRPCCannotAnswer(t *testing.T) {
	e := newEnv(t)
	srv := newFailingRPCServer(t, e)

	const sig = "5unauthorizedSignatureForConfirmPendingTest1111111"
	const dest = "BGN4PHEuETe1ZKKsMPumALst8o3ZFibMw8TCwxcKcpKV"
	seedTransaction(t, e, sig, dest, "10 minutes")

	// Several ticks: the verdict must not drift towards "expired" the longer the
	// endpoint stays unreachable either.
	for i := 0; i < 3; i++ {
		srv.ConfirmPending(t.Context())
		if status, _ := loadTxStatus(t, e, sig); status != "submitted" {
			t.Fatalf("an unreachable endpoint must leave the record pending, got %q after %d checks", status, i+1)
		}
	}

	// The endpoint recovers and the transfer is there: it is confirmed, and the
	// record was never wrong in between.
	chain := &fakeChain{seen: map[string]fakeTx{}}
	chain.land(sig, dest, 10000000, 4242)
	recovered := newChainServer(t, e, chain)
	recovered.ConfirmPending(t.Context())

	if status, slot := loadTxStatus(t, e, sig); status != "confirmed" {
		t.Fatalf("after the endpoint recovered the transaction should be confirmed, got %q", status)
	} else if slot == nil || *slot != 4242 {
		t.Fatalf("expected the on-chain slot 4242, got %v", slot)
	}
}

// The status index can lag or omit a signature the ledger does contain, which is
// why the worker asks the ledger directly before declaring a transfer lost. If
// that lookup fails, the transfer is not lost -- it is unexamined. It must stay
// pending: the old code discarded the error and reported the payment as "Not
// sent" because a provider rate-limited one method.
func TestConfirmPendingKeepsRecordPendingWhenChainLookupFails(t *testing.T) {
	e := newEnv(t)
	chain := &fakeChain{seen: map[string]fakeTx{}, failTxLookup: true}
	srv := newChainServer(t, e, chain)

	const sig = "5ratelimitedLookupForConfirmPendingTest11111111111"
	const dest = "BGN4PHEuETe1ZKKsMPumALst8o3ZFibMw8TCwxcKcpKV"
	seedTransaction(t, e, sig, dest, "10 minutes")

	srv.ConfirmPending(t.Context())
	if status, _ := loadTxStatus(t, e, sig); status != "submitted" {
		t.Fatalf("a failed chain lookup must not become a verdict, got %q", status)
	}

	// The lookup recovers and the transfer is on the ledger: now, and only now,
	// is there something to report.
	chain.mu.Lock()
	chain.failTxLookup = false
	chain.seen[sig] = fakeTx{slot: 31337, destination: dest, lamports: 10000000}
	chain.mu.Unlock()

	srv.ConfirmPending(t.Context())
	if status, slot := loadTxStatus(t, e, sig); status != "confirmed" {
		t.Fatalf("the transfer should be confirmed once it can be looked up, got %q", status)
	} else if slot == nil || *slot != 31337 {
		t.Fatalf("expected slot 31337, got %v", slot)
	}
}

// The node may simply not have seen it: when the transaction does land, the
// record is corrected rather than left permanently wrong.
func TestConfirmPendingRecoversLateTransaction(t *testing.T) {
	e := newEnv(t)
	chain := &fakeChain{seen: map[string]fakeTx{}}
	srv := newChainServer(t, e, chain)

	const dropped = "5lateLandingSignatureForConfirmPendingTest111111111"
	const dest = "BGN4PHEuETe1ZKKsMPumALst8o3ZFibMw8TCwxcKcpKV"
	seedTransaction(t, e, dropped, dest, "10 minutes")

	srv.ConfirmPending(t.Context())
	if status, _ := loadTxStatus(t, e, dropped); status != "expired" {
		t.Fatalf("expected expired before the transaction lands, got %q", status)
	}

	chain.land(dropped, dest, 10000000, 987654)
	// The re-check of an expired record is deliberately slow (every 10 minutes).
	if _, err := e.st.Pool.Exec(t.Context(),
		`UPDATE transactions SET last_checked_at = now() - interval '11 minutes' WHERE signature = $1`,
		dropped); err != nil {
		t.Fatalf("backdate last_checked_at: %v", err)
	}

	srv.ConfirmPending(t.Context())

	status, slot := loadTxStatus(t, e, dropped)
	if status != "confirmed" {
		t.Fatalf("a transaction that landed late should be confirmed, got %q", status)
	}
	if slot == nil || *slot != 987654 {
		t.Fatalf("expected the on-chain slot 987654 to be recorded, got %v", slot)
	}
}
