import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useWallet } from "../lib/wallet";
import { Alert, Badge, CopyButton } from "../components/ui";
import { shortenAddress } from "../lib/format";

function WalletSection() {
  const { me, refresh } = useAuth();
  const wallet = useWallet();
  const [error, setError] = useState<string | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);

  const alreadyVerified =
    wallet.address !== null &&
    (me?.wallets ?? []).some((w) => w.address === wallet.address && w.ownership_verified);

  async function verify() {
    if (!wallet.address) return;
    setError(null);
    setVerifying(true);
    try {
      const challenge = await api.createChallenge(wallet.address);
      const signature = await wallet.signMessage(challenge.message);
      await api.verifyWallet(wallet.address, challenge.nonce, signature, challenge.network);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not verify the wallet.");
    } finally {
      setVerifying(false);
    }
  }

  async function remove(id: string) {
    setBusyId(id);
    setError(null);
    try {
      await api.deleteWallet(id);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not remove the wallet.");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <section className="card p-5">
      <h2 className="text-base font-semibold">Wallets</h2>
      <p className="mt-1 text-sm muted">
        Only public addresses are stored. Ownership is proven by signing a short, single-use message
        that the server verifies. We never see your keys.
      </p>
      {error ? (
        <div className="mt-3">
          <Alert kind="error">{error}</Alert>
        </div>
      ) : null}

      <div className="mt-4 space-y-3">
        {(me?.wallets ?? []).length === 0 ? (
          <Alert kind="info">No wallet connected yet.</Alert>
        ) : null}
        {(me?.wallets ?? []).map((w) => (
          <div key={w.id} className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-line bg-bg p-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <code className="font-mono text-sm">{shortenAddress(w.address, 6)}</code>
                <Badge>{w.network}</Badge>
                {w.ownership_verified ? (
                  <Badge kind="success">Verified</Badge>
                ) : (
                  <Badge kind="warning">Unverified</Badge>
                )}
                {w.is_primary ? <Badge>Primary</Badge> : null}
              </div>
              <code className="mt-1 block break-all font-mono text-xs muted">{w.address}</code>
            </div>
            <button
              type="button"
              className="btn-danger"
              onClick={() => void remove(w.id)}
              disabled={busyId === w.id}
            >
              {busyId === w.id ? "Removing…" : "Remove"}
            </button>
          </div>
        ))}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        {!wallet.connected ? (
          wallet.available.length > 0 ? (
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
              No Solana wallet detected. Install Phantom or Solflare to connect one.
            </span>
          )
        ) : (
          <>
            <span className="text-sm muted">
              Connected: {wallet.providerName} · {shortenAddress(wallet.address ?? "")}
            </span>
            {alreadyVerified ? (
              <Badge kind="success">This address is verified on your account</Badge>
            ) : (
              <button type="button" className="btn-primary" onClick={() => void verify()} disabled={verifying}>
                {verifying ? "Waiting for signature…" : "Verify ownership"}
              </button>
            )}
            <button type="button" className="btn-ghost" onClick={() => void wallet.disconnect()}>
              Disconnect
            </button>
          </>
        )}
      </div>
      {wallet.error ? <p className="mt-2 text-xs text-danger">{wallet.error}</p> : null}
    </section>
  );
}

export default function Settings() {
  const { me, logout, refresh } = useAuth();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  async function signOutEverywhere() {
    try {
      await logout();
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign out.");
    }
  }

  async function deleteAccount() {
    setError(null);
    if (confirm !== "DELETE") {
      setError('Type DELETE exactly to confirm account deletion.');
      return;
    }
    setBusy(true);
    try {
      const res = await api.deleteAccount();
      setNotice(res.note);
      await refresh();
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not delete the account.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="h2">Settings</h1>
        <p className="mt-1 text-sm muted">Manage your wallet, privacy and account.</p>
      </div>

      {error ? <Alert kind="error">{error}</Alert> : null}
      {notice ? <Alert kind="success">{notice}</Alert> : null}

      <WalletSection />

      <section className="card p-5">
        <h2 className="text-base font-semibold">Profile &amp; privacy</h2>
        <p className="mt-1 text-sm muted">
          Edit your display name, bio and public visibility on the{" "}
          <Link to="/app/handle" className="link">
            My Handle
          </Link>{" "}
          page.
        </p>
        <p className="mt-2 text-sm muted">
          Signed in as <span className="font-medium text-ink">{me?.email}</span>.
        </p>
      </section>

      <section className="card p-5">
        <h2 className="text-base font-semibold">Handle policy</h2>
        <p className="mt-1 text-sm muted">
          Handles are immutable in this release. If you delete your account, your handle is released
          and may be claimed by someone else later.
        </p>
      </section>

      <section className="card p-5">
        <h2 className="text-base font-semibold">Session</h2>
        <p className="mt-1 text-sm muted">
          Signing out invalidates this session everywhere it is used.
        </p>
        <button type="button" className="btn-secondary mt-3" onClick={() => void signOutEverywhere()}>
          Sign out
        </button>
      </section>

      <section className="card border-danger/30 p-5">
        <h2 className="text-base font-semibold text-danger">Delete account</h2>
        <p className="mt-1 text-sm muted">
          This removes your account data (email, password, wallets, identities, sessions) and releases
          your handle. On-chain transactions are public and permanent — we cannot delete them, and
          anyone can still see them on a block explorer.
        </p>
        <div className="mt-3 max-w-sm">
          <label className="label" htmlFor="confirm">
            Type DELETE to confirm
          </label>
          <input
            id="confirm"
            className="input"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>
        <button
          type="button"
          className="btn-danger mt-3"
          onClick={() => void deleteAccount()}
          disabled={busy || confirm !== "DELETE"}
        >
          {busy ? "Deleting…" : "Delete my account"}
        </button>
      </section>

      <section className="card p-5">
        <h2 className="text-base font-semibold">Security &amp; privacy</h2>
        <div className="mt-2 flex flex-wrap gap-4 text-sm">
          <Link to="/docs#security" className="link">
            Security notes
          </Link>
          <Link to="/privacy" className="link">
            Privacy policy (draft)
          </Link>
          <Link to="/terms" className="link">
            Terms (draft)
          </Link>
        </div>
        <div className="mt-3">
          <CopyButton value="security@example.invalid" label="Copy security contact" className="btn-ghost" />
        </div>
      </section>
    </div>
  );
}
