package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openhandle/internal/httpx"
	"openhandle/internal/models"
	"openhandle/internal/rpc"
	"openhandle/internal/store"
	"openhandle/internal/validate"
)

type txDTO struct {
	ID               string     `json:"id"`
	Network          string     `json:"network"`
	Signature        string     `json:"signature"`
	SenderAddress    *string    `json:"sender_address"`
	RecipientAddress string     `json:"recipient_address"`
	AmountBaseUnits  string     `json:"amount_base_units"`
	AssetIdentifier  string     `json:"asset_identifier"`
	Status           string     `json:"status"`
	Slot             *int64     `json:"slot"`
	SubmittedAt      time.Time  `json:"submitted_at"`
	ConfirmedAt      *time.Time `json:"confirmed_at"`
}

func toTxDTO(t models.Transaction) txDTO {
	return txDTO{
		ID:               t.ID.String(),
		Network:          t.Network,
		Signature:        t.Signature,
		SenderAddress:    t.SenderAddress,
		RecipientAddress: t.RecipientAddress,
		AmountBaseUnits:  t.AmountBaseUnits,
		AssetIdentifier:  t.AssetIdentifier,
		Status:           t.Status,
		Slot:             t.Slot,
		SubmittedAt:      t.SubmittedAt,
		ConfirmedAt:      t.ConfirmedAt,
	}
}

// resolveRecipientWallet finds the public payment address for a handle.
func (s *Server) resolveRecipientWallet(ctx context.Context, handle string) (*models.Handle, string, error) {
	h, err := s.Store.GetHandle(ctx, handle)
	if err != nil {
		return nil, "", err
	}
	if !h.IsPublic {
		return nil, "", store.ErrNotFound
	}
	ws, err := s.Store.ListWallets(ctx, h.UserID)
	if err != nil {
		return nil, "", err
	}
	// Prefer the primary wallet; fall back to the earliest wallet.
	for _, w := range ws {
		if w.IsPrimary {
			return h, w.Address, nil
		}
	}
	if len(ws) > 0 {
		return h, ws[0].Address, nil
	}
	return h, "", store.ErrNotFound
}

// CreateTransaction records a payment the user just signed and submitted. The
// transaction is verified against the chain when the RPC can already see it;
// otherwise it is stored as pending and confirmed by the background worker.
func (s *Server) CreateTransaction(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		Network         string `json:"network"`
		Signature       string `json:"signature"`
		RecipientHandle string `json:"recipient_handle"`
		AmountBaseUnits string `json:"amount_base_units"`
		Asset           string `json:"asset"`
		SenderAddress   string `json:"sender_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}

	network := req.Network
	if network == "" {
		network = s.Cfg.SolanaCluster
	}
	if network != s.Cfg.SolanaCluster {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid, "This server is configured for a different network.")
		return
	}
	asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	if asset == "" {
		asset = "SOL"
	}
	if asset != "SOL" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid, "Only native SOL transfers are supported in this release.")
		return
	}
	sig := strings.TrimSpace(req.Signature)
	// The RPC rejects a whole status batch that contains one malformed signature,
	// so a single junk value could stall confirmation for every pending payment.
	// Only a 64-byte ed25519 signature is accepted.
	if !validate.IsValidSignature(sig) {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid, "That is not a valid Solana transaction signature.")
		return
	}
	amount, err := strconv.ParseInt(strings.TrimSpace(req.AmountBaseUnits), 10, 64)
	if err != nil || amount <= 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid, "Amount must be a positive integer in lamports.")
		return
	}

	ctx := c.Request.Context()

	// Re-resolve the recipient from the database; never trust a client address.
	h, recipientAddr, err := s.resolveRecipientWallet(ctx, validate.NormalizeHandle(req.RecipientHandle))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeHandleNotFound, "Recipient handle was not found or has no wallet.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not resolve the recipient.")
		return
	}

	var sender *string
	if req.SenderAddress != "" {
		if !validate.IsValidSolanaAddress(req.SenderAddress) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid, "Sender address is not valid.")
			return
		}
		sender = &req.SenderAddress
	}

	tx := models.Transaction{
		Network:           network,
		Signature:         sig,
		SenderAddress:     sender,
		RecipientAddress:  recipientAddr,
		AmountBaseUnits:   strconv.FormatInt(amount, 10),
		AssetIdentifier:   asset,
		Status:            "submitted",
		CreatedByUserID:   &user.ID,
		RecipientHandleID: &h.ID,
	}

	// Best-effort immediate chain verification. RPC failure never fabricates a
	// success: it leaves the record pending.
	if s.RPC != nil {
		status, slot, verr := s.verifyAgainstChain(ctx, sig, recipientAddr, amount)
		if verr == nil {
			switch status {
			case "confirmed":
				tx.Status = "confirmed"
				tx.Slot = slot
			case "failed":
				tx.Status = "failed"
			case "mismatch":
				httpx.Fail(c, http.StatusBadRequest, httpx.CodeTxInvalid,
					"The on-chain transaction does not match this recipient and amount.")
				return
			}
		}
	}

	recorded, created, err := s.Store.IngestTransaction(ctx, tx)
	if err != nil {
		s.Log.Error("ingest tx", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not record the transaction.")
		return
	}
	if created {
		s.Store.Audit(ctx, &user.ID, "transaction.submitted", "transaction", recorded.ID.String(),
			map[string]any{"network": network, "status": recorded.Status})
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	c.JSON(code, toTxDTO(*recorded))
}

// verifyAgainstChain returns "confirmed", "failed", "mismatch", or "pending".
func (s *Server) verifyAgainstChain(ctx context.Context, signature, recipient string, lamports int64) (string, *int64, error) {
	parsed, err := s.RPC.GetTransaction(ctx, signature)
	if err != nil {
		return "pending", nil, err
	}
	if parsed == nil {
		return "pending", nil, nil
	}
	if parsed.Err {
		return "failed", &parsed.Slot, nil
	}
	for _, tr := range parsed.Transfers {
		if tr.Destination == recipient && tr.Lamports == lamports {
			return "confirmed", &parsed.Slot, nil
		}
	}
	return "mismatch", nil, nil
}

// statusesOneByOne is the fallback for a batch that the RPC rejected: it asks
// about each signature separately and reports the ones it could not resolve as
// "not found", so the caller applies the same drop logic to them.
func (s *Server) statusesOneByOne(ctx context.Context, sigs []string) []rpc.Status {
	out := make([]rpc.Status, len(sigs))
	for i, sig := range sigs {
		one, err := s.RPC.GetSignatureStatuses(ctx, []string{sig})
		if err != nil || len(one) == 0 {
			continue // Status{Found:false}
		}
		out[i] = one[0]
	}
	return out
}

// ListMyTransactions returns the caller's transaction history (scoped by owner).
func (s *Server) ListMyTransactions(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	limit, offset := pageParams(c)

	txs, total, err := s.Store.ListUserTransactions(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load transactions.")
		return
	}
	out := make([]txDTO, 0, len(txs))
	for _, t := range txs {
		out = append(out, toTxDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{
		"transactions": out,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
		"scope":        "Transactions submitted through OpenHandle on " + s.Cfg.SolanaCluster + " only.",
	})
}

// GetTransaction returns one transaction owned by the caller.
func (s *Server) GetTransaction(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	sig := c.Param("signature")
	tx, err := s.Store.GetTransactionBySignature(c.Request.Context(), s.Cfg.SolanaCluster, sig)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Transaction not found.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load the transaction.")
		return
	}
	if tx.CreatedByUserID == nil || *tx.CreatedByUserID != user.ID {
		// Do not reveal whether another user's transaction exists.
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Transaction not found.")
		return
	}
	c.JSON(http.StatusOK, toTxDTO(*tx))
}

func pageParams(c *gin.Context) (int, int) {
	limit := 20
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

// txDropTimeout is how long a submitted signature may stay invisible to the
// chain before it is treated as dropped.
//
// A Solana blockhash is only valid for roughly 90 seconds, so a transaction the
// network has still not seen after a few minutes will never confirm: it was
// dropped before it landed and the sender's funds never left their wallet.
// Reporting it as "submitted" for an hour, as the worker used to, misrepresents
// a payment that never happened -- senders saw "Pending" for a transfer that no
// node had ever accepted.
const txDropTimeout = 3 * time.Minute

// ConfirmPending polls the RPC for pending transactions and advances their
// status. Called periodically by the background worker.
func (s *Server) ConfirmPending(ctx context.Context) {
	if s.RPC == nil {
		return
	}
	pending, err := s.Store.ListPendingTransactions(ctx, 50)
	if err != nil || len(pending) == 0 {
		return
	}
	sigs := make([]string, len(pending))
	for i, t := range pending {
		sigs[i] = t.Signature
	}
	statuses, err := s.RPC.GetSignatureStatuses(ctx, sigs)
	if err != nil {
		// One signature the RPC refuses (an older record written before
		// signatures were validated, say) fails the whole call. Retry them one by
		// one so it cannot stall confirmation for everything else.
		statuses = s.statusesOneByOne(ctx, sigs)
	}
	for i, t := range pending {
		if i >= len(statuses) {
			break
		}
		st := statuses[i]
		switch {
		case !st.Found:
			// Still inside the blockhash window: the network may simply not have
			// confirmed it yet, so leave the record pending.
			if time.Since(t.SubmittedAt) < txDropTimeout {
				continue
			}
			// The status index can lag or omit a signature the ledger does
			// contain, so ask authoritatively before declaring the transfer lost.
			amount, _ := strconv.ParseInt(t.AmountBaseUnits, 10, 64)
			status, slot, _ := s.verifyAgainstChain(ctx, t.Signature, t.RecipientAddress, amount)
			switch status {
			case "confirmed":
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "confirmed", slot)
			case "failed", "mismatch":
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "failed", slot)
			default:
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "expired", nil)
			}
		case st.Err:
			_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "failed", &st.Slot)
		case st.ConfirmationStatus == rpc.StatusConfirmed || st.ConfirmationStatus == rpc.StatusFinalized:
			// Re-verify the recorded details against the chain before confirming.
			amount, _ := strconv.ParseInt(t.AmountBaseUnits, 10, 64)
			status, slot, _ := s.verifyAgainstChain(ctx, t.Signature, t.RecipientAddress, amount)
			switch status {
			case "confirmed":
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "confirmed", slot)
			case "failed", "mismatch":
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "failed", slot)
			default:
				_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "confirmed", &st.Slot)
			}
		default:
			_ = s.Store.UpdateTransactionStatus(ctx, t.ID, "submitted", &st.Slot)
		}
	}
}
