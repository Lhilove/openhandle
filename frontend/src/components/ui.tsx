import { useState, type ReactNode } from "react";

export function Spinner({ label = "Loading" }: { label?: string }) {
  return (
    <span role="status" aria-live="polite" className="inline-flex items-center gap-2 text-sm text-muted">
      <span
        aria-hidden="true"
        className="h-4 w-4 animate-spin rounded-full border-2 border-line border-t-accent"
      />
      {label}
    </span>
  );
}

export function Skeleton({ width = "100%", height = 16 }: { width?: string | number; height?: number }) {
  return (
    <span
      aria-hidden="true"
      className="block animate-pulse rounded bg-line/60"
      style={{ width, height }}
    />
  );
}

type AlertKind = "info" | "success" | "warning" | "error";

export function Alert({ kind = "info", children }: { kind?: AlertKind; children: ReactNode }) {
  const styles: Record<AlertKind, string> = {
    info: "border-line bg-bg text-ink",
    success: "border-transparent bg-success/10 text-success",
    warning: "border-transparent bg-warning/10 text-warning",
    error: "border-transparent bg-danger/10 text-danger",
  };
  const role = kind === "error" ? "alert" : "status";
  return (
    <div role={role} className={`rounded-control border px-3.5 py-3 text-sm ${styles[kind]}`}>
      {children}
    </div>
  );
}

export function CopyButton({
  value,
  label = "Copy",
  className = "btn-secondary",
}: {
  value: string;
  label?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // Clipboard API can be unavailable (e.g. insecure context); fall back.
      const el = document.createElement("textarea");
      el.value = value;
      document.body.appendChild(el);
      el.select();
      document.execCommand("copy");
      document.body.removeChild(el);
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }
  return (
    <button type="button" className={className} onClick={copy} aria-live="polite">
      {copied ? "Copied" : label}
    </button>
  );
}

type BadgeKind = "neutral" | "success" | "warning" | "danger";

export function Badge({ kind = "neutral", children }: { kind?: BadgeKind; children: ReactNode }) {
  const cls = {
    neutral: "badge",
    success: "badge-success",
    warning: "badge-warning",
    danger: "badge-danger",
  }[kind];
  return <span className={cls}>{children}</span>;
}

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="card flex flex-col items-start gap-3 p-6">
      <h3 className="text-base font-semibold">{title}</h3>
      {children ? <div className="text-sm muted">{children}</div> : null}
      {action}
    </div>
  );
}

export function Field({
  label,
  hint,
  error,
  htmlFor,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label className="label" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {hint && !error ? (
        <p id={`${htmlFor}-hint`} className="mt-1 text-xs muted">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${htmlFor}-error`} role="alert" className="mt-1 text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

/** Status chip for a transaction. */
export function TxStatus({ status }: { status: string }) {
  const map: Record<string, { kind: BadgeKind; label: string }> = {
    confirmed: { kind: "success", label: "Confirmed" },
    submitted: { kind: "warning", label: "Pending" },
    failed: { kind: "danger", label: "Failed" },
    expired: { kind: "neutral", label: "Expired" },
  };
  const entry = map[status] ?? { kind: "neutral" as BadgeKind, label: status };
  return <Badge kind={entry.kind}>{entry.label}</Badge>;
}
