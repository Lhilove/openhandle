package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	// Direction is "sent" or "received" from the caller's point of view.
	Direction string `json:"direction"`
	// Counterparty is the other side of the payment, for display: the recipient
	// on a payment the caller sent, the sender on one they received.
	CounterpartyHandle  *string `json:"counterparty_handle"`
	CounterpartyAddress string  `json:"counterparty_address"`
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

func toPaymentDTO(p models.Payment) txDTO {
	d := toTxDTO(p.Transaction)
	d.Direction = p.Direction
	d.CounterpartyHandle = p.CounterpartyHandle
	d.CounterpartyAddress = p.CounterpartyAddress
	return d
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

	// Record who sent it as well as who received it. The recipient's history
	// shows a payment the sender's client created, so the sender's handle has to
	// be stored with it: without it the recipient sees an anonymous address.
	var senderHandleID *uuid.UUID
	if sh, err := s.Store.GetActiveHandleForUser(ctx, user.ID); err == nil && sh != nil {
		senderHandleID = &sh.ID
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
		SenderHandleID:    senderHandleID,
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
// about each signature separately so one bad signature cannot hide the rest.
//
// A signature the endpoint failed to answer is reported as unavailable, not as
// missing. Only an answer the endpoint actually gave is evidence about the
// chain.
func (s *Server) statusesOneByOne(ctx context.Context, sigs []string) []rpc.Status {
	out := make([]rpc.Status, len(sigs))
	for i, sig := range sigs {
		one, err := s.RPC.GetSignatureStatuses(ctx, []string{sig})
		if err != nil || len(one) == 0 {
			out[i] = rpc.Status{Unavailable: true}
			continue
		}
		out[i] = one[0]
	}
	return out
}

// allUnavailable reports whether the endpoint failed to answer for every
// signature, which means the batch produced no information at all.
func allUnavailable(statuses []rpc.Status) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, st := range statuses {
		if !st.Unavailable {
			return false
		}
	}
	return true
}

// ListMyTransactions returns the caller's payments: the ones they sent and the
// ones they received, on one timeline.
func (s *Server) ListMyTransactions(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	limit, offset := pageParams(c)

	payments, total, err := s.Store.ListUserTransactions(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load transactions.")
		return
	}
	out := make([]txDTO, 0, len(payments))
	for _, p := range payments {
		out = append(out, toPaymentDTO(p))
	}
	c.JSON(http.StatusOK, gin.H{
		"transactions": out,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
		"scope": "Payments sent and received through OpenHandle on " + s.Cfg.SolanaCluster +
			" only; it is not a full view of your wallet's on-chain activity.",
	})
}

// GetTransaction returns one payment the caller is party to: the sender who
// submitted it, or the recipient whose funds arrived.
func (s *Server) GetTransaction(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	sig := c.Param("signature")

	visible, err := s.Store.TransactionVisibleTo(c.Request.Context(), user.ID, s.Cfg.SolanaCluster, sig)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load the transaction.")
		return
	}
	if !visible {
		// Do not reveal whether a transaction the caller is not party to exists.
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Transaction not found.")
		return
	}
	tx, err := s.Store.GetTransactionBySignature(c.Request.Context(), s.Cfg.SolanaCluster, sig)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Transaction not found.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load the transaction.")
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
//
// The rule this worker must never break: an endpoint that cannot answer says
// nothing about a transaction. A provider outage -- a rejected credential, a
// rate limit, a timeout -- is not evidence that the network dropped a transfer.
// Treating it as one marked a payment that had already finalized as "Not sent",
// which is the worst outcome of all: the sender is told their money never left
// when it did. So every verdict below is written only from a reply the endpoint
// actually gave; anything else leaves the record pending for the next tick.
func (s *Server) ConfirmPending(ctx context.Context) {
	if s.RPC == nil {
		return
	}
	pending, err := s.Store.ListPendingTransactions(ctx, 50)
	if err != nil {
		s.Log.Warn("list pending transactions", "err", err)
		return
	}
	if len(pending) == 0 {
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
		// one so it cannot stall confirmation for everything else -- but a call
		// that failed for every signature leaves all of them pending.
		statuses = s.statusesOneByOne(ctx, sigs)
		if allUnavailable(statuses) {
			s.Log.Warn("rpc cannot be reached; transactions left pending",
				"err", err, "pending", len(pending))
			return
		}
	}
	for i, t := range pending {
		if i >= len(statuses) {
			break
		}
		s.applyChainStatus(ctx, t, statuses[i])
	}
}

// applyChainStatus advances one record from a single endpoint reply.
func (s *Server) applyChainStatus(ctx context.Context, t models.Transaction, st rpc.Status) {
	switch {
	case st.Unavailable:
		// We learned nothing. Try again on the next tick.
		return
	case !st.Found:
		// Still inside the blockhash window: the network may simply not have
		// confirmed it yet, so leave the record pending.
		if time.Since(t.SubmittedAt) < txDropTimeout {
			return
		}
		// The status index can lag or omit a signature the ledger does contain,
		// so ask authoritatively before declaring the transfer lost.
		amount, _ := strconv.ParseInt(t.AmountBaseUnits, 10, 64)
		status, slot, verr := s.verifyAgainstChain(ctx, t.Signature, t.RecipientAddress, amount)
		if verr != nil {
			// The lookup failed, so the chain was never actually asked. Hold the
			// record pending rather than calling a real payment lost.
			s.Log.Warn("transaction status deferred: rpc lookup failed",
				"signature", t.Signature, "err", verr)
			return
		}
		switch status {
		case "confirmed":
			s.setTxStatus(ctx, t, "confirmed", slot)
		case "failed", "mismatch":
			s.setTxStatus(ctx, t, "failed", slot)
		default:
			// The endpoint looked for the transaction and could not find it, past
			// the blockhash window: it will never land.
			s.setTxStatus(ctx, t, "expired", nil)
		}
	case st.Err:
		s.setTxStatus(ctx, t, "failed", &st.Slot)
	case st.ConfirmationStatus == rpc.StatusConfirmed || st.ConfirmationStatus == rpc.StatusFinalized:
		// Re-verify the recorded details against the chain before confirming.
		amount, _ := strconv.ParseInt(t.AmountBaseUnits, 10, 64)
		status, slot, verr := s.verifyAgainstChain(ctx, t.Signature, t.RecipientAddress, amount)
		if verr != nil {
			// The index already saw the transaction, which is enough to confirm
			// it; a failed detail lookup must not hold a landed payment pending.
			s.setTxStatus(ctx, t, "confirmed", &st.Slot)
			return
		}
		switch status {
		case "confirmed":
			if slot == nil {
				slot = &st.Slot
			}
			s.setTxStatus(ctx, t, "confirmed", slot)
		case "failed", "mismatch":
			s.setTxStatus(ctx, t, "failed", slot)
		default:
			s.setTxStatus(ctx, t, "confirmed", &st.Slot)
		}
	default:
		s.setTxStatus(ctx, t, "submitted", &st.Slot)
	}
}

// setTxStatus persists a status change and records it. Nothing here fabricates a
// verdict: the caller only reaches it with an answer from the chain.
func (s *Server) setTxStatus(ctx context.Context, t models.Transaction, status string, slot *int64) {
	if err := s.Store.UpdateTransactionStatus(ctx, t.ID, status, slot); err != nil {
		s.Log.Error("update transaction status", "signature", t.Signature, "status", status, "err", err)
		return
	}
	if status != "submitted" && status != t.Status {
		s.Log.Info("transaction status", "signature", t.Signature, "network", t.Network,
			"from", t.Status, "to", status)
	}
}
