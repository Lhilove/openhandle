import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { credentialsSchema } from "../lib/schemas";
import { useAuth } from "../lib/auth";
import { Alert, Field } from "../components/ui";

export default function SignUp() {
  const { register } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = params.get("next") ?? "/claim";

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const parsed = credentialsSchema.safeParse({ email, password });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? "Please check your details.");
      return;
    }
    setBusy(true);
    try {
      await register(parsed.data.email, parsed.data.password);
      navigate(next, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the account.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-md px-4 py-12 sm:px-6">
      <h1 className="h2">Create your account</h1>
      <p className="mt-2 text-sm muted">Then claim a handle and connect a wallet you control.</p>

      <form className="card mt-6 flex flex-col gap-4 p-6" onSubmit={submit} noValidate>
        {error ? <Alert kind="error">{error}</Alert> : null}
        <Field label="Email" htmlFor="email" hint="Used only to sign in. Never shown publicly.">
          <input
            id="email"
            type="email"
            autoComplete="email"
            className="input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </Field>
        <Field label="Password" htmlFor="password" hint="At least 8 characters.">
          <input
            id="password"
            type="password"
            autoComplete="new-password"
            className="input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </Field>
        <button type="submit" className="btn-primary" disabled={busy}>
          {busy ? "Creating…" : "Create account"}
        </button>
        <p className="text-sm muted">
          Already have an account?{" "}
          <Link to="/signin" className="link">
            Sign in
          </Link>
        </p>
      </form>
    </div>
  );
}
