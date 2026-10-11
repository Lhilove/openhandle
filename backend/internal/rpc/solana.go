// Package rpc provides a minimal Solana JSON-RPC client behind an interface so
// it can be replaced or faked. It is used for authoritative transaction status
// and for verifying that a submitted transfer matches what was recorded.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Confirmation statuses as reported by the RPC.
const (
	StatusProcessed = "processed"
	StatusConfirmed = "confirmed"
	StatusFinalized = "finalized"
)

// Status is the chain status of a signature.
type Status struct {
	Found              bool
	Slot               int64
	Confirmations      *int64
	ConfirmationStatus string
	Err                bool
}

// Transfer describes a parsed native SOL transfer found in a transaction.
type Transfer struct {
	Source      string
	Destination string
	Lamports    int64
}

// ParsedTransaction is a normalised view of a fetched transaction.
type ParsedTransaction struct {
	Slot           int64
	Err            bool
	Transfers      []Transfer
	AccountKeys    []string
	MetaFeePresent bool
}

// Client talks to a Solana JSON-RPC endpoint.
type Client struct {
	url     string
	cluster string
	http    *http.Client
}

// New builds a client for the given RPC URL and cluster name.
func New(url, cluster string) *Client {
	return &Client{
		url:     url,
		cluster: cluster,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Cluster returns the configured cluster label.
func (c *Client) Cluster() string { return c.cluster }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc %s: http %d", method, resp.StatusCode)
	}
	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("rpc %s: decode: %w", method, err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("rpc %s: %s (code %d)", method, parsed.Error.Message, parsed.Error.Code)
	}
	return parsed.Result, nil
}

// Healthy performs a getHealth check.
func (c *Client) Healthy(ctx context.Context) bool {
	_, err := c.call(ctx, "getHealth", []any{})
	return err == nil
}

// GetSignatureStatuses returns the chain status of up to a batch of signatures.
func (c *Client) GetSignatureStatuses(ctx context.Context, signatures []string) ([]Status, error) {
	if len(signatures) == 0 {
		return nil, nil
	}
	raw, err := c.call(ctx, "getSignatureStatuses", []any{signatures, map[string]any{"searchTransactionHistory": true}})
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Value []*struct {
			Slot               int64  `json:"slot"`
			Confirmations      *int64 `json:"confirmations"`
			ConfirmationStatus string `json:"confirmationStatus"`
			Err                any    `json:"err"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	out := make([]Status, len(signatures))
	for i := range signatures {
		if i >= len(wrap.Value) || wrap.Value[i] == nil {
			out[i] = Status{Found: false}
			continue
		}
		v := wrap.Value[i]
		out[i] = Status{
			Found:              true,
			Slot:               v.Slot,
			Confirmations:      v.Confirmations,
			ConfirmationStatus: v.ConfirmationStatus,
			Err:                v.Err != nil,
		}
	}
	return out, nil
}

// GetTransaction fetches a transaction with jsonParsed encoding and extracts
// native SOL transfers. Returns (nil, nil) when the transaction is not found.
func (c *Client) GetTransaction(ctx context.Context, signature string) (*ParsedTransaction, error) {
	raw, err := c.call(ctx, "getTransaction", []any{signature, map[string]any{
		"encoding":                       "jsonParsed",
		"maxSupportedTransactionVersion": 0,
		"commitment":                     "confirmed",
	}})
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var tx struct {
		Slot int64 `json:"slot"`
		Meta *struct {
			Err any `json:"err"`
		} `json:"meta"`
		Transaction struct {
			Message struct {
				AccountKeys []struct {
					Pubkey string `json:"pubkey"`
				} `json:"accountKeys"`
				Instructions []struct {
					Program string `json:"program"`
					Parsed  struct {
						Type string          `json:"type"`
						Info json.RawMessage `json:"info"`
					} `json:"parsed"`
				} `json:"instructions"`
			} `json:"message"`
		} `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &tx); err != nil {
		return nil, err
	}

	out := &ParsedTransaction{Slot: tx.Slot}
	if tx.Meta != nil {
		out.Err = tx.Meta.Err != nil
		out.MetaFeePresent = true
	}
	for _, k := range tx.Transaction.Message.AccountKeys {
		out.AccountKeys = append(out.AccountKeys, k.Pubkey)
	}
	for _, ix := range tx.Transaction.Message.Instructions {
		if ix.Program != "system" || ix.Parsed.Type != "transfer" || len(ix.Parsed.Info) == 0 {
			continue
		}
		var info struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
			Lamports    int64  `json:"lamports"`
		}
		if err := json.Unmarshal(ix.Parsed.Info, &info); err != nil {
			continue
		}
		out.Transfers = append(out.Transfers, Transfer{
			Source:      info.Source,
			Destination: info.Destination,
			Lamports:    info.Lamports,
		})
	}
	return out, nil
}
