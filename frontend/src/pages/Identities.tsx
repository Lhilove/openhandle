import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, type IdentityList } from "../lib/api";
import { Alert, Badge, EmptyState, Skeleton } from "../components/ui";

const PROVIDER_LABEL: Record<string, string> = {
  github: "GitHub",
  reddit: "Reddit",
};

export default function Identities() {
  const [data, setData] = useState<IdentityList | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [params] = useSearchParams();
  const connected = params.get("connected");

  const load = useCallback(async () => {
    try {
      setData(await api.listIdentities());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load identities.");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function disconnect(provider: string) {
    setBusy(provider);
    setError(null);
    try {
      await api.disconnectIdentity(provider);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not disconnect.");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="h2">Identities</h1>
        <p className="mt-1 text-sm muted">
          Link a social account to show a verified badge on your public profile. Verification uses the
          provider's official API — we never scrape profiles or trust a username you type in.
        </p>
      </div>

      <Alert kind="info">
        A verified account proves you control that account. It is <strong>not</strong> government or
        legal identity verification, and it does not prove anything about your legal name or
        residency.
      </Alert>

      {connected ? <Alert kind="success">Connected {PROVIDER_LABEL[connected] ?? connected}.</Alert> : null}
      {error ? <Alert kind="error">{error}</Alert> : null}

      {!data && !error ? (
        <div className="card space-y-3 p-5">
          <Skeleton width="40%" />
          <Skeleton width="60%" />
        </div>
      ) : null}

      {data ? (
        <div className="space-y-4">
          {data.providers.map((p) => {
            const linked = data.identities.find((i) => i.provider === p.provider);
            const label = PROVIDER_LABEL[p.provider] ?? p.provider;
            return (
              <div key={p.provider} className="card flex flex-wrap items-center justify-between gap-3 p-5">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-base font-semibold">{label}</span>
                    {linked ? (
                      <Badge kind="success">Verified</Badge>
                    ) : p.configured ? (
                      <Badge>Not connected</Badge>
                    ) : (
                      <Badge kind="warning">Not configured</Badge>
                    )}
                  </div>
                  {linked ? (
                    <p className="mt-1 text-sm muted">
                      Verified as <span className="font-medium text-ink">@{linked.username}</span>
                    </p>
                  ) : p.configured ? (
                    <p className="mt-1 text-sm muted">
                      Connect to prove you control this account. We request the minimum scope needed
                      and do not retain access tokens.
                    </p>
                  ) : (
                    <p className="mt-1 text-sm muted">
                      This provider is not configured on this server, so no connect button is shown.
                      Nothing here is simulated.
                    </p>
                  )}
                </div>
                <div>
                  {linked ? (
                    <button
                      type="button"
                      className="btn-danger"
                      onClick={() => void disconnect(p.provider)}
                      disabled={busy === p.provider}
                    >
                      {busy === p.provider ? "Disconnecting…" : "Disconnect"}
                    </button>
                  ) : p.configured ? (
                    <button
                      type="button"
                      className="btn-primary"
                      onClick={() => {
                        window.location.href = api.identityStartUrl(p.provider);
                      }}
                    >
                      Connect {label}
                    </button>
                  ) : (
                    <button type="button" className="btn-secondary" disabled>
                      Coming later
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      ) : null}

      {data && data.identities.length === 0 ? (
        <EmptyState title="No verified accounts yet">
          Verified social accounts appear on your public profile only after a successful provider
          verification.
        </EmptyState>
      ) : null}
    </div>
  );
}
