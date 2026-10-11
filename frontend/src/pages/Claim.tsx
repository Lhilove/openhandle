import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../lib/api";
import { handleSchema } from "../lib/schemas";
import { useAuth } from "../lib/auth";
import { Alert, Field, Spinner } from "../components/ui";

type AvailabilityState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "available"; handle: string }
  | { kind: "taken" }
  | { kind: "invalid"; message: string }
  | { kind: "error"; message: string };

export default function Claim() {
  const { me, loading, refresh } = useAuth();
  const navigate = useNavigate();
  const [handle, setHandle] = useState("");
  const [state, setState] = useState<AvailabilityState>({ kind: "idle" });
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const timer = useRef<number | null>(null);

  const check = useCallback(async (value: string) => {
    const parsed = handleSchema.safeParse(value);
    if (!parsed.success) {
      setState({ kind: "invalid", message: parsed.error.issues[0]?.message ?? "Invalid handle." });
      return;
    }
    setState({ kind: "checking" });
    try {
      const res = await api.availability(parsed.data);
      setState(res.available ? { kind: "available", handle: res.handle } : { kind: "taken" });
    } catch {
      setState({ kind: "error", message: "Could not check availability. Check your connection." });
    }
  }, []);

  useEffect(() => {
    if (timer.current) window.clearTimeout(timer.current);
    if (!handle) {
      setState({ kind: "idle" });
      return;
    }
    timer.current = window.setTimeout(() => void check(handle), 400);
    return () => {
      if (timer.current) window.clearTimeout(timer.current);
    };
  }, [handle, check]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setSubmitError(null);
    const parsed = handleSchema.safeParse(handle);
    if (!parsed.success) {
      setState({ kind: "invalid", message: parsed.error.issues[0]?.message ?? "Invalid handle." });
      return;
    }
    setBusy(true);
    try {
      await api.claimHandle(parsed.data);
      await refresh();
      navigate("/app", { replace: true });
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Could not claim the handle.");
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="mx-auto max-w-md px-4 py-12 sm:px-6">
        <Spinner label="Checking your session…" />
      </div>
    );
  }

  if (!me) {
    return (
      <div className="mx-auto max-w-md px-4 py-12 sm:px-6">
        <h1 className="h2">Claim your handle</h1>
        <Alert kind="info">
          You need an account before you can claim a handle. Creating one takes a moment and stores
          only an email and a password hash.
        </Alert>
        <div className="mt-4 flex gap-3">
          <Link to="/signup?next=/claim" className="btn-primary">
            Create account
          </Link>
          <Link to="/signin?next=/claim" className="btn-secondary">
            Sign in
          </Link>
        </div>
      </div>
    );
  }

  if (me.handle) {
    return (
      <div className="mx-auto max-w-md px-4 py-12 sm:px-6">
        <h1 className="h2">You already have a handle</h1>
        <p className="mt-2 text-sm muted">
          Your handle <span className="font-semibold text-ink">@{me.handle.handle}</span> is
          immutable in this release.
        </p>
        <div className="mt-4 flex gap-3">
          <Link to="/app" className="btn-primary">
            Go to dashboard
          </Link>
          <Link to={`/@${me.handle.handle}`} className="btn-secondary">
            View public profile
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-md px-4 py-12 sm:px-6">
      <h1 className="h2">Claim your handle</h1>
      <p className="mt-2 text-sm muted">
        Lowercase letters, numbers and single underscores. 3–20 characters. Handles are immutable
        once claimed.
      </p>

      <form className="card mt-6 flex flex-col gap-4 p-6" onSubmit={submit} noValidate>
        {submitError ? <Alert kind="error">{submitError}</Alert> : null}
        <Field label="Handle" htmlFor="handle" hint="Reserved names like admin, api and support are not available.">
          <div className="flex items-center gap-2">
            <span aria-hidden="true" className="text-muted">
              @
            </span>
            <input
              id="handle"
              className="input"
              inputMode="text"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              value={handle}
              onChange={(e) => setHandle(e.target.value)}
              aria-describedby="handle-status"
              required
            />
          </div>
        </Field>

        <div id="handle-status" aria-live="polite" className="min-h-[1.25rem] text-sm">
          {state.kind === "checking" ? <span className="muted">Checking availability…</span> : null}
          {state.kind === "available" ? (
            <span className="text-success">@{state.handle} is available.</span>
          ) : null}
          {state.kind === "taken" ? <span className="text-danger">That handle is already taken.</span> : null}
          {state.kind === "invalid" ? <span className="text-danger">{state.message}</span> : null}
          {state.kind === "error" ? <span className="text-danger">{state.message}</span> : null}
        </div>

        <button
          type="submit"
          className="btn-primary"
          disabled={busy || state.kind !== "available"}
        >
          {busy ? "Claiming…" : "Claim handle"}
        </button>
      </form>
    </div>
  );
}
