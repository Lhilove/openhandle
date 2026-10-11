import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api } from "../lib/api";
import { bioSchema, displayNameSchema } from "../lib/schemas";
import { useAuth } from "../lib/auth";
import { Alert, Badge, CopyButton, EmptyState, Field } from "../components/ui";

export default function MyHandle() {
  const { me, refresh } = useAuth();
  const handle = me?.handle ?? null;

  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [isPublic, setIsPublic] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (handle) {
      setDisplayName(handle.display_name ?? "");
      setBio(handle.bio ?? "");
      setIsPublic(handle.is_public);
    }
  }, [handle]);

  if (!handle) {
    return (
      <EmptyState
        title="You have not claimed a handle yet"
        action={
          <Link to="/claim" className="btn-primary">
            Claim a handle
          </Link>
        }
      >
        A handle is how people find and pay you.
      </EmptyState>
    );
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    const name = displayNameSchema.safeParse(displayName);
    const about = bioSchema.safeParse(bio);
    if (!name.success) {
      setError(name.error.issues[0]?.message ?? "Invalid display name.");
      return;
    }
    if (!about.success) {
      setError(about.error.issues[0]?.message ?? "Invalid bio.");
      return;
    }
    setBusy(true);
    try {
      await api.updateProfile({ display_name: name.data, bio: about.data, is_public: isPublic });
      await refresh();
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save.");
    } finally {
      setBusy(false);
    }
  }

  const profileUrl = `${window.location.origin}/@${handle.handle}`;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="h2">My handle</h1>
        <p className="mt-1 text-sm muted">
          Your handle is immutable in this release. You can edit how your public profile looks.
        </p>
      </div>

      <div className="card p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="text-xs font-medium uppercase tracking-wide muted">Handle</div>
            <div className="mt-1 text-xl font-semibold">@{handle.handle}</div>
          </div>
          <div className="flex items-center gap-2">
            <Badge kind={handle.is_public ? "success" : "warning"}>
              {handle.is_public ? "Public" : "Private"}
            </Badge>
            <CopyButton value={profileUrl} label="Copy link" className="btn-secondary" />
            <Link to={`/@${handle.handle}`} className="btn-secondary">
              View
            </Link>
          </div>
        </div>
      </div>

      <form className="card flex flex-col gap-4 p-5" onSubmit={save} noValidate>
        <h2 className="text-base font-semibold">Public profile</h2>
        {error ? <Alert kind="error">{error}</Alert> : null}
        {saved ? <Alert kind="success">Profile saved.</Alert> : null}

        <Field label="Display name" htmlFor="display_name" hint="Optional. Shown next to your handle.">
          <input
            id="display_name"
            className="input"
            value={displayName}
            maxLength={60}
            onChange={(e) => setDisplayName(e.target.value)}
          />
        </Field>

        <Field label="Bio" htmlFor="bio" hint="Optional. Up to 280 characters.">
          <textarea
            id="bio"
            className="input min-h-24"
            value={bio}
            maxLength={280}
            onChange={(e) => setBio(e.target.value)}
          />
        </Field>

        <label className="flex items-start gap-3">
          <input
            type="checkbox"
            className="mt-1"
            checked={isPublic}
            onChange={(e) => setIsPublic(e.target.checked)}
          />
          <span className="text-sm">
            Make my profile public
            <span className="mt-1 block text-xs muted">
              When public, your handle, display name, bio, public wallet addresses and verified
              accounts are visible to anyone. When private, the profile resolves to &ldquo;not
              found&rdquo;.
            </span>
          </span>
        </label>

        <div>
          <button type="submit" className="btn-primary" disabled={busy}>
            {busy ? "Saving…" : "Save profile"}
          </button>
        </div>
      </form>
    </div>
  );
}
