import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, ApiError, SOLANA_CLUSTER, type PublicProfile, type Tx } from "../lib/api";
import { useWallet } from "../lib/wallet";
import { Alert, Badge, CopyButton, Field, Spinner, TxStatus } from "../components/ui";
import { lamportsToSol, solToLamports } from "../lib/format";

type Stage = "form" | "review" | "sending" | "done";

function explorerUrl(signature: string): string {
  const cluster = SOLANA_CLUSTER === "mainnet-beta" ? "" : `?cluster=${SOLANA_CLUSTER}`;
  return `https://explorer.solana.com/tx/${signature}${cluster}`;
}

export default function Send() {
  const [params] = useSearchParams();
  const wallet = useWallet();

  const [recipientInput, setRecipientInput] = useState(params.get("to") ?? "");
  const [recipient, setRecipient] = useState<PublicProfile | null>(null);
  const [resolveError, setResolveError] = useState<string | null>(null);
  const [resolving, setResolving] = useState(false);

  const [amount, setAmount] = useState("");
  const [amountError, setAmountError] = useState<string | null>(null);
  const [stage, setStage] = useState<Stage>("form");
  const [error, setError] = useState<string | null>(null);
  const [dropped, setDropped] = useState<string | null>(null);
  const [tx, setTx] = useState<Tx | null>(null);
  const [signature, setSignature] = useState<string | null>(null);
  // Mainnet moves real funds, so the sender has to say so explicitly.
  const [riskAccepted, setRiskAccepted] = useState(false);
  const pollRef = useRef<number | null>(null);

  const resolve = useCallback(async (handle: string) => {
    setResolveError(null);
    setRecipient(null);
    if (!handle) {
      setResolveError("Enter the handle you want to pay, for example @love.");
      return;
    }
    setResolving(true);
    try {
      const profile = await api.resolveHandle(handle.replace(/^@/, ""));
      setRecipient(profile);
      if (!profile.wallets.some((w) => w.is_primary) && profile.wallets.length === 0) {
        setResolveError("That profile has no wallet configured, so it cannot receive payments.");
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === "HANDLE_NOT_FOUND") {
        setResolveError("No public handle was found for that name.");
      } else if (err instanceof ApiError && err.code === "HANDLE_INVALID") {
        setResolveError("That handle is not valid.");
      } else {
        setResolveError(err instanceof Error ? err.message : "Could not resolve the handle.");
      }
    } finally {
      setResolving(false);
    }
  }, []);

  useEffect(() => {
    const to = params.get("to");
    if (to) void resolve(to);
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, [params, resolve]);

  const primaryWallet = recipient?.wallets.find((w) => w.is_primary) ?? recipient?.wallets[0];
  const lamports = solToLamports(amount);

  function goToReview(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!recipient || !primaryWallet) {
      setError("Resolve a recipient with a wallet first.");
      return;
    }
    if (lamports === null || lamports <= 0n) {
      setAmountError("Enter a positive amount, up to 9 decimal places.");
      return;
    }
    setAmountError(null);
    setStage("review");
  }

  async function signAndSend() {
    if (!recipient || !primaryWallet || lamports === null) return;
    setError(null);
    setDropped(null);
    if (!wallet.connected) {
      setError("Connect a wallet to sign and send this payment.");
      return;
    }
    // Re-resolve the recipient immediately before building the transaction, and
    // never rely on the handle alone once the user has reviewed.
    setStage("sending");
    try {
      const fresh = await api.resolveHandle(recipient.handle);
      const freshWallet = fresh.wallets.find((w) => w.is_primary) ?? fresh.wallets[0];
      if (!freshWallet || freshWallet.address !== primaryWallet.address) {
        throw new Error(
          "The recipient's wallet changed since you reviewed it. Please review again before sending.",
        );
      }

      const outcome = await wallet.sendSol(freshWallet.address, lamports);
      setSignature(outcome.signature);

      if (outcome.landing === "dropped") {
        // The network never accepted the transaction (its blockhash expired
        // before a node took it). Nothing left the wallet and the recipient got
        // nothing, so there is no payment to record -- recording one would put a
        // payment that never happened into the history.
        setSignature(null);
        setStage("review");
        setDropped(
          `The ${SOLANA_CLUSTER} network never accepted that transaction, so no funds left your wallet and @${fresh.handle} received nothing. ` +
            "This usually happens when the blockhash expires while the wallet approval is open, or when your wallet is connected to a different network than this app. " +
            "Check that your wallet is on the same network, then sign again — a fresh transaction is built each time.",
        );
        return;
      }

      const recorded = await api.createTransaction({
        signature: outcome.signature,
        recipient_handle: fresh.handle,
        amount_base_units: lamports.toString(),
        asset: "SOL",
        sender_address: wallet.address ?? undefined,
      });
      setTx(recorded);
      setStage("done");

      // Bounded polling with backoff until the chain reports a final status.
      let attempt = 0;
      pollRef.current = window.setInterval(async () => {
        attempt += 1;
        try {
          const latest = await api.getTransaction(outcome.signature);
          setTx(latest);
          if (latest.status === "confirmed" || latest.status === "failed" || latest.status === "expired") {
            if (pollRef.current) window.clearInterval(pollRef.current);
          }
        } catch {
          // ignore transient errors; keep polling
        }
        if (attempt > 20 && pollRef.current) {
          window.clearInterval(pollRef.current);
        }
      }, 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "The payment could not be completed.");
      setStage("review");
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="h2">Send a payment</h1>
        <p className="mt-1 text-sm muted">
          OpenHandle resolves a handle to a wallet address. You sign and submit from your own wallet —
          we never sign for you, and we never hold funds.
        </p>
      </div>

      <Alert kind={wallet.cluster === "mainnet-beta" ? "error" : "warning"}>
        Blockchain transactions are generally irreversible. Network fees may apply and are paid by
        your wallet. Always confirm the recipient, network, asset and amount before signing.
        {wallet.cluster === "mainnet-beta" ? (
          <span className="mt-1 block">
            This deployment is on <strong>mainnet-beta</strong> — payments move <strong>real SOL</strong>{" "}
            and cannot be refunded or reversed.
          </span>
        ) : (
          <span className="mt-1 block">
            This deployment is on <strong>{wallet.cluster}</strong> — test funds only.
          </span>
        )}
      </Alert>

      {error ? <Alert kind="error">{error}</Alert> : null}
      {dropped ? <Alert kind="warning">{dropped}</Alert> : null}

      {/* Step 1: recipient */}
      <form
        className="card flex flex-col gap-4 p-5"
        onSubmit={(e) => {
          e.preventDefault();
          void resolve(recipientInput);
        }}
      >
        <h2 className="text-base font-semibold">1. Recipient</h2>
        <Field
          label="Recipient handle"
          htmlFor="recipient"
          hint="For example love or @love"
          error={resolveError ?? undefined}
        >
          <input
            id="recipient"
            className="input"
            value={recipientInput}
            onChange={(e) => setRecipientInput(e.target.value)}
            autoCapitalize="none"
            spellCheck={false}
          />
        </Field>
        <div className="flex items-center gap-3">
          <button type="submit" className="btn-secondary" disabled={resolving}>
            {resolving ? "Resolving…" : "Resolve"}
          </button>
          {resolving ? <Spinner label="" /> : null}
        </div>

        {recipient && primaryWallet ? (
          <div className="rounded-control border border-line bg-bg p-4 text-sm">
            <div className="flex items-center gap-2">
              <span className="font-semibold">@{recipient.handle}</span>
              <Badge>solana · {recipient.network}</Badge>
              {primaryWallet.ownership_verified ? (
                <Badge kind="success">Ownership verified</Badge>
              ) : (
                <Badge kind="warning">Not verified</Badge>
              )}
            </div>
            <div className="mt-2">
              <div className="text-xs muted">Full destination address</div>
              <code className="mt-1 block break-all font-mono text-xs">{primaryWallet.address}</code>
            </div>
          </div>
        ) : null}
      </form>

      {/* Step 2: amount */}
      <div className="card flex flex-col gap-4 p-5">
        <h2 className="text-base font-semibold">2. Amount</h2>
        <Field label="Amount (SOL)" htmlFor="amount" error={amountError ?? undefined} hint="Asset: native SOL">
          <input
            id="amount"
            className="input"
            inputMode="decimal"
            placeholder="0.01"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
        </Field>
        <div className="flex items-center gap-3">
          <button
            type="button"
            className="btn-primary"
            disabled={!recipient || !primaryWallet}
            onClick={goToReview}
          >
            Review payment
          </button>
          {!recipient ? <span className="text-xs muted">Resolve a recipient first.</span> : null}
        </div>
      </div>

      {/* Step 3: review + send */}
      {stage !== "form" ? (
        <div className="card flex flex-col gap-4 p-5">
          <h2 className="text-base font-semibold">3. Review and sign</h2>
          <dl className="divide-y divide-line text-sm">
            <div className="flex justify-between gap-3 py-2">
              <dt className="muted">Recipient</dt>
              <dd className="font-medium">@{recipient?.handle}</dd>
            </div>
            <div className="flex flex-col gap-1 py-2">
              <dt className="muted">Destination address (full)</dt>
              <dd className="break-all font-mono text-xs">{primaryWallet?.address}</dd>
            </div>
            <div className="flex justify-between gap-3 py-2">
              <dt className="muted">Network</dt>
              <dd>solana · {SOLANA_CLUSTER}</dd>
            </div>
            <div className="flex justify-between gap-3 py-2">
              <dt className="muted">Asset</dt>
              <dd>SOL</dd>
            </div>
            <div className="flex justify-between gap-3 py-2">
              <dt className="muted">Amount</dt>
              <dd className="font-medium">
                {lamports !== null ? `${lamportsToSol(lamports.toString())} SOL` : amount}
              </dd>
            </div>
          </dl>

          {!wallet.connected ? (
            <div className="flex flex-col gap-3">
              <Alert kind="warning">Connect a Solana wallet to sign this payment.</Alert>
              <div className="flex flex-wrap gap-2">
                {wallet.available.length > 0 ? (
                  wallet.available.map((w) => (
                    <button
                      key={w.name}
                      type="button"
                      className="btn-secondary"
                      onClick={() => void wallet.connect(w.name)}
                      disabled={wallet.connecting}
                    >
                      Connect {w.name}
                    </button>
                  ))
                ) : (
                  <span className="text-sm muted">
                    No Solana wallet detected. Install Phantom or Solflare.
                  </span>
                )}
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {wallet.cluster === "mainnet-beta" ? (
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-0.5"
                    checked={riskAccepted}
                    onChange={(e) => setRiskAccepted(e.target.checked)}
                    disabled={stage === "sending"}
                  />
                  <span>
                    I understand this sends real SOL on Solana mainnet to the address above, and that
                    it cannot be reversed.
                  </span>
                </label>
              ) : null}
              <button
                type="button"
                className="btn-primary"
                onClick={() => void signAndSend()}
                disabled={stage === "sending" || (wallet.cluster === "mainnet-beta" && !riskAccepted)}
              >
                {stage === "sending" ? "Waiting for your wallet…" : "Sign and send"}
              </button>
            </div>
          )}

          <button
            type="button"
            className="btn-ghost self-start"
            onClick={() => setStage("form")}
            disabled={stage === "sending"}
          >
            Back to details
          </button>
        </div>
      ) : null}

      {/* Result */}
      {tx && signature ? (
        <div className="card flex flex-col gap-3 p-5">
          <div className="flex items-center gap-3">
            <TxStatus status={tx.status} />
            <span className="text-sm font-semibold">
              {tx.status === "confirmed"
                ? "Confirmed on-chain"
                : tx.status === "failed"
                  ? "Transaction failed"
                  : tx.status === "expired"
                    ? "Never reached the network"
                    : "Submitted — awaiting confirmation"}
            </span>
          </div>
          <p className="text-sm muted">
            {tx.status === "expired"
              ? `${SOLANA_CLUSTER} never accepted this transaction, so no funds left your wallet and the recipient received nothing. A transaction that is not on the network within about 90 seconds will never be: its blockhash has expired. Send it again to build a fresh one.`
              : `Status is read from the ${SOLANA_CLUSTER} network, not from the browser. If it stays pending, the network may simply be slow; your wallet also shows the final result.`}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <code className="break-all font-mono text-xs">{signature}</code>
            <CopyButton value={signature} label="Copy signature" className="btn-ghost" />
          </div>
          <div className="flex flex-wrap gap-3">
            <a
              className="btn-secondary"
              href={explorerUrl(signature)}
              target="_blank"
              rel="noreferrer noopener"
            >
              View on explorer
            </a>
            <Link to="/app/payments" className="btn-secondary">
              Go to payments
            </Link>
            {tx.status === "expired" ? (
              <button
                type="button"
                className="btn-primary"
                onClick={() => {
                  setTx(null);
                  setSignature(null);
                  setStage("review");
                }}
              >
                Send again
              </button>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
