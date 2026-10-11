import { Link } from "react-router-dom";

export default function NotFound() {
  return (
    <div className="mx-auto max-w-xl px-4 py-20 text-center sm:px-6">
      <p className="text-sm font-semibold uppercase tracking-wide text-accent">404</p>
      <h1 className="h1 mt-2">Page not found</h1>
      <p className="mt-3 text-sm muted">
        The page you are looking for does not exist. If you were looking for a handle, try{" "}
        <code className="font-mono">/@handle</code>.
      </p>
      <div className="mt-6 flex justify-center gap-3">
        <Link to="/" className="btn-primary">
          Go home
        </Link>
        <Link to="/claim" className="btn-secondary">
          Claim a handle
        </Link>
      </div>
    </div>
  );
}
