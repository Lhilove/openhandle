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
