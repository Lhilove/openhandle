import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, ApiError, type PublicProfile as Profile } from "../lib/api";
import { Alert, Badge, CopyButton, EmptyState, Spinner } from "../components/ui";
import { shortenAddress } from "../lib/format";
import NotFound from "./NotFound";

type State =
  | { kind: "loading" }
  | { kind: "ready"; profile: Profile }
  | { kind: "notfound" }
  | { kind: "invalid" }
  | { kind: "error"; message: string };

export default function PublicProfile() {
  const params = useParams();
  const raw = params.handleWithAt ?? "";
  const isHandlePath = raw.startsWith("@") && raw.length > 1;
  const handle = isHandlePath ? raw.slice(1) : "";
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    if (!isHandlePath) return;
    let cancelled = false;
    setState({ kind: "loading" });
    api
      .resolveHandle(handle)
      .then((profile) => {
        if (!cancelled) setState({ kind: "ready", profile });
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.code === "HANDLE_INVALID") {
          setState({ kind: "invalid" });
        } else if (err instanceof ApiError && err.code === "HANDLE_NOT_FOUND") {
          setState({ kind: "notfound" });
        } else {
          setState({ kind: "error", message: err instanceof Error ? err.message : "Something went wrong." });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [handle, isHandlePath]);

  if (!isHandlePath) {
    return <NotFound />;
  }

  if (state.kind === "loading") {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16 sm:px-6">
        <Spinner label="Resolving handle…" />
      </div>
    );
  }

  if (state.kind === "invalid") {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16 sm:px-6">
        <EmptyState title="That handle is not valid">
          Handles use lowercase letters, numbers and single underscores.
        </EmptyState>
      </div>
    );
  }

  if (state.kind === "notfound") {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16 sm:px-6">
        <EmptyState
          title="No public handle was found"
          action={
            <Link to="/claim" className="btn-primary">
              Claim this handle
            </Link>
          }
        >
          This handle either does not exist or has been made private by its owner.
        </EmptyState>
      </div>
    );
  }

  if (state.kind === "error") {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16 sm:px-6">
        <Alert kind="error">{state.message}</Alert>
      </div>
    );
  }

  const p = state.profile;
  const primary = p.wallets.find((w) => w.is_primary) ?? p.wallets[0];
  const paymentLink = `${window.location.origin}/@${p.handle}`;

  return (
    <div className="mx-auto max-w-2xl px-4 py-10 sm:px-6">
      <div className="card p-6">
        <div className="flex items-start gap-4">
          <span
            aria-hidden="true"
            className="grid h-14 w-14 shrink-0 place-items-center rounded-full bg-accent/10 text-xl font-semibold text-accent"
          >
            @
          </span>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-2xl font-semibold">@{p.handle}</h1>
            {p.display_name ? <p className="mt-0.5 text-sm font-medium">{p.display_name}</p> : null}
            {p.bio ? <p className="mt-2 text-sm muted">{p.bio}</p> : null}
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <Badge>solana · {p.network}</Badge>
              {primary?.ownership_verified ? (
                <Badge kind="success">Wallet ownership verified</Badge>
              ) : primary ? (
                <Badge kind="warning">Wallet not verified</Badge>
              ) : null}
            </div>
          </div>
        </div>

        <div className="mt-6 space-y-4">
          {primary ? (
            <div className="rounded-control border border-line bg-bg p-4">
              <div className="text-xs font-medium uppercase tracking-wide muted">Wallet address</div>
              <div className="mt-1.5 flex flex-wrap items-center gap-2">
                <code className="break-all font-mono text-sm">{primary.address}</code>
                <CopyButton value={primary.address} label="Copy address" className="btn-ghost" />
              </div>
            </div>
          ) : (
            <Alert kind="warning">
              This profile has no wallet configured yet, so payments cannot be sent.
            </Alert>
          )}

          {p.identities.length > 0 ? (
            <div className="rounded-control border border-line bg-bg p-4">
              <div className="text-xs font-medium uppercase tracking-wide muted">Verified accounts</div>
              <ul className="mt-2 space-y-1.5">
                {p.identities.map((i) => (
                  <li key={i.provider} className="flex items-center gap-2 text-sm">
                    <span className="font-medium capitalize">{i.provider}</span>
                    <span className="muted">@{i.username}</span>
                    <Badge kind="success">Verified</Badge>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          <div className="flex flex-wrap gap-3">
            {primary ? (
              <Link to={`/app/send?to=${encodeURIComponent(p.handle)}`} className="btn-primary">
                Send payment
              </Link>
            ) : null}
            <CopyButton value={paymentLink} label="Copy payment link" className="btn-secondary" />
          </div>

          <Alert kind="warning">
            Blockchain transactions are generally irreversible. Before you sign, always confirm the
            recipient, network, asset and amount in your wallet.
          </Alert>
        </div>
      </div>

      <p className="mt-4 text-xs muted">
        {primary ? `Address shown in short form: ${shortenAddress(primary.address)}. ` : ""}
        Only fields the owner has made public are displayed. Social accounts are marked verified only
        after a provider's official API confirmation.
      </p>
    </div>
  );
}
