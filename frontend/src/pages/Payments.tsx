import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, SOLANA_CLUSTER, type TransactionList } from "../lib/api";
import { Alert, Badge, EmptyState, Skeleton, TxStatus } from "../components/ui";
import { formatDate, lamportsToSol, shortenAddress } from "../lib/format";

const PAGE = 10;

function explorerUrl(signature: string): string {
  const cluster = SOLANA_CLUSTER === "mainnet-beta" ? "" : `?cluster=${SOLANA_CLUSTER}`;
  return `https://explorer.solana.com/tx/${signature}${cluster}`;
}

export default function Payments() {
  const [data, setData] = useState<TransactionList | null>(null);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .listTransactions(PAGE, offset)
      .then((res) => {
        if (!cancelled) setData(res);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load payments.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [offset]);

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="h2">Payments</h1>
          <p className="mt-1 text-sm muted">
            {data?.scope ?? `Payments sent and received through OpenHandle on ${SOLANA_CLUSTER}.`}
          </p>
        </div>
        <Link to="/app/send" className="btn-primary">
          Send payment
        </Link>
      </div>

      {error ? <Alert kind="error">{error}</Alert> : null}

      {loading && !data ? (
        <div className="card space-y-3 p-5">
          <Skeleton width="50%" />
          <Skeleton width="30%" />
        </div>
      ) : null}

      {data && data.transactions.length === 0 && !loading ? (
        <EmptyState title="No payments yet">
          History lists the payments you sent and received through OpenHandle. It is not a full view
          of your wallet's on-chain activity.
        </EmptyState>
      ) : null}

      {data && data.transactions.length > 0 ? (
        // The table is wider than a phone. Scroll it rather than clipping the
        // columns off the edge, where they cannot be reached at all.
        <div className="card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <caption className="sr-only">Payments sent and received</caption>
              <thead className="bg-bg text-left text-xs uppercase tracking-wide muted">
                <tr>
                  <th className="px-4 py-3 font-medium">Status</th>
                  <th className="px-4 py-3 font-medium">Amount</th>
                  <th className="px-4 py-3 font-medium">Sent / received</th>
                  <th className="px-4 py-3 font-medium">Counterparty</th>
                  <th className="px-4 py-3 font-medium">Submitted</th>
                  <th className="px-4 py-3 font-medium">Signature</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {data.transactions.map((tx) => {
                  const inbound = tx.direction === "received";
                  return (
                    <tr key={tx.id}>
                      <td className="px-4 py-3">
                        <TxStatus status={tx.status} />
                      </td>
                      <td className="px-4 py-3 font-medium">
                        {tx.asset_identifier === "SOL"
                          ? `${lamportsToSol(tx.amount_base_units)} SOL`
                          : `${tx.amount_base_units} ${tx.asset_identifier}`}
                      </td>
                      <td className="px-4 py-3">
                        <Badge kind={inbound ? "success" : "neutral"}>
                          {inbound ? "Received" : "Sent"}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-xs muted">{inbound ? "From " : "To "}</span>
                        <span className="font-mono text-xs">
                          {tx.counterparty_handle
                            ? `@${tx.counterparty_handle}`
                            : shortenAddress(tx.counterparty_address, 6)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-xs muted">{formatDate(tx.submitted_at)}</td>
                      <td className="px-4 py-3">
                        <a
                          className="link font-mono text-xs"
                          href={explorerUrl(tx.signature)}
                          target="_blank"
                          rel="noreferrer noopener"
                        >
                          {shortenAddress(tx.signature, 6)}
                        </a>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {data && data.total > PAGE ? (
        <div className="flex items-center justify-between">
          <span className="text-sm muted">
            Showing {offset + 1}–{Math.min(offset + PAGE, data.total)} of {data.total}
          </span>
          <div className="flex gap-2">
            <button
              type="button"
              className="btn-secondary"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE))}
            >
              Previous
            </button>
            <button
              type="button"
              className="btn-secondary"
              disabled={offset + PAGE >= data.total}
              onClick={() => setOffset(offset + PAGE)}
            >
              Next
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
