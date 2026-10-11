import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Tx } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useWallet } from "../lib/wallet";
import { Alert, Badge, CopyButton, EmptyState, Skeleton, TxStatus } from "../components/ui";
import { formatDate, lamportsToSol, shortenAddress } from "../lib/format";

function WalletStatusCard() {
  const { me } = useAuth();
  const { connected, address, providerName, error } = useWallet();
  const primary = me?.wallets.find((w) => w.is_primary) ?? me?.wallets[0];

  return (
    <div className="card p-5">
      <h2 className="text-base font-semibold">Wallet</h2>
      {primary ? (
        <div className="mt-3 space-y-2 text-sm">
          <div className="flex items-center justify-between gap-3">
            <span className="muted">Address</span>
            <span className="font-mono">{shortenAddress(primary.address)}</span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="muted">Network</span>
            <span>{primary.network}</span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="muted">Ownership</span>
            {primary.ownership_verified ? (
              <Badge kind="success">Verified by signature</Badge>
            ) : (
              <Badge kind="warning">Not verified</Badge>
            )}
          </div>
          <div className="pt-1">
            <CopyButton value={primary.address} label="Copy address" className="btn-secondary" />
          </div>
        </div>
      ) : (
        <div className="mt-3">
          <Alert kind="warning">
            No wallet connected yet. Connect a wallet and verify ownership to receive payments.
          </Alert>
          <Link to="/app/settings" className="btn-primary mt-3">
            Connect a wallet
          </Link>
        </div>
      )}
      {connected ? (
        <p className="mt-3 text-xs muted">
          Browser wallet connected{providerName ? ` (${providerName})` : ""}
          {address ? ` · ${shortenAddress(address)}` : ""}. This is separate from the stored,
          ownership-verified wallet above.
        </p>
      ) : null}
      {error ? <p className="mt-2 text-xs text-danger">{error}</p> : null}
    </div>
  );
}

export default function Dashboard() {
  const { me } = useAuth();
  const [txs, setTxs] = useState<Tx[] | null>(null);
  const [txError, setTxError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .listTransactions(5, 0)
      .then((res) => {
        if (!cancelled) setTxs(res.transactions);
      })
      .catch((err) => {
        if (!cancelled) setTxError(err instanceof Error ? err.message : "Could not load activity.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handle = me?.handle;
  const profileUrl = handle ? `${window.location.origin}/@${handle.handle}` : null;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="h2">Welcome{handle ? `, @${handle.handle}` : ""}</h1>
        <p className="mt-1 text-sm muted">
          Your identity, your money, your handle. Nothing here is simulated — every figure comes from
          your account or the Solana network.
        </p>
      </div>

      {!handle ? (
        <EmptyState
          title="Claim your handle"
          action={
            <Link to="/app/handle" className="btn-primary">
              Claim a handle
            </Link>
          }
        >
          You need a handle before you can share a payment profile.
        </EmptyState>
      ) : (
        <div className="card p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="text-xs font-medium uppercase tracking-wide muted">Your handle</div>
              <div className="mt-1 text-xl font-semibold">@{handle.handle}</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <CopyButton value={profileUrl ?? ""} label="Copy profile link" className="btn-secondary" />
              <Link to={`/@${handle.handle}`} className="btn-secondary">
                View public profile
              </Link>
              <Link to="/app/send" className="btn-primary">
                Send payment
              </Link>
            </div>
          </div>
        </div>
      )}

      <div className="grid gap-5 md:grid-cols-2">
        <WalletStatusCard />
        <div className="card p-5">
          <h2 className="text-base font-semibold">Balance</h2>
          <p className="mt-3 text-sm muted">
            Not shown. We do not display a balance unless it comes from a reliable, authoritative
            source, and no balance indexer is configured in this release. Check your wallet for the
            current balance.
          </p>
        </div>
      </div>

      <section>
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">Recent activity</h2>
          <Link to="/app/payments" className="text-sm link">
            View all
          </Link>
        </div>
        <div className="mt-3 space-y-3">
          {txError ? <Alert kind="error">{txError}</Alert> : null}
          {!txs && !txError ? (
            <div className="card space-y-3 p-5">
              <Skeleton width="60%" />
              <Skeleton width="40%" />
            </div>
          ) : null}
          {txs && txs.length === 0 ? (
            <EmptyState title="No activity yet">
              Transactions you submit through OpenHandle will appear here. History covers only
              OpenHandle-submitted transactions.
            </EmptyState>
          ) : null}
          {txs?.map((tx) => (
            <div key={tx.id} className="card flex flex-wrap items-center justify-between gap-3 p-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <TxStatus status={tx.status} />
                  <span className="text-sm font-medium">
                    {tx.asset_identifier === "SOL"
                      ? `${lamportsToSol(tx.amount_base_units)} SOL`
                      : `${tx.amount_base_units} ${tx.asset_identifier}`}
                  </span>
                </div>
                <div className="mt-1 truncate text-xs muted">
                  to <span className="font-mono">{shortenAddress(tx.recipient_address)}</span> ·{" "}
                  {formatDate(tx.submitted_at)}
                </div>
              </div>
              <code className="max-w-[40%] truncate font-mono text-xs muted">{tx.signature}</code>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
