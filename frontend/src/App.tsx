import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import type { ReactNode } from "react";
import { useAuth } from "./lib/auth";
import { AppShell, MarketingShell } from "./components/Shell";
import { Spinner } from "./components/ui";
import Landing from "./pages/Landing";
import SignIn from "./pages/SignIn";
import SignUp from "./pages/SignUp";
import Claim from "./pages/Claim";
import Docs from "./pages/Docs";
import Legal from "./pages/Legal";
import PublicProfile from "./pages/PublicProfile";
import NotFound from "./pages/NotFound";
import Dashboard from "./pages/Dashboard";
import MyHandle from "./pages/MyHandle";
import Payments from "./pages/Payments";
import Send from "./pages/Send";
import Identities from "./pages/Identities";
import Settings from "./pages/Settings";

function RequireAuth({ children }: { children: ReactNode }) {
  const { me, loading } = useAuth();
  const location = useLocation();
  if (loading) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-16">
        <Spinner label="Loading your account…" />
      </div>
    );
  }
  if (!me) {
    return <Navigate to={`/signin?next=${encodeURIComponent(location.pathname)}`} replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      {/* Marketing + public */}
      <Route
        path="/"
        element={
          <MarketingShell>
            <Landing />
          </MarketingShell>
        }
      />
      <Route
        path="/docs"
        element={
          <MarketingShell>
            <Docs />
          </MarketingShell>
        }
      />
      <Route
        path="/terms"
        element={
          <MarketingShell>
            <Legal kind="terms" />
          </MarketingShell>
        }
      />
      <Route
        path="/privacy"
        element={
          <MarketingShell>
            <Legal kind="privacy" />
          </MarketingShell>
        }
      />
      <Route
        path="/signin"
        element={
          <MarketingShell>
            <SignIn />
          </MarketingShell>
        }
      />
      <Route
        path="/signup"
        element={
          <MarketingShell>
            <SignUp />
          </MarketingShell>
        }
      />
      <Route
        path="/claim"
        element={
          <MarketingShell>
            <Claim />
          </MarketingShell>
        }
      />
      {/*
        React Router v6 parameters must occupy a whole path segment, so a public
        profile is matched by a single dynamic segment and the component verifies
        the leading "@" itself. Static routes above are ranked first, so /docs,
        /terms, /signin etc. still win.
      */}
      <Route
        path="/:handleWithAt"
        element={
          <MarketingShell>
            <PublicProfile />
          </MarketingShell>
        }
      />

      {/* Authenticated app */}
      <Route
        path="/app"
        element={
          <RequireAuth>
            <AppShell>
              <Dashboard />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/handle"
        element={
          <RequireAuth>
            <AppShell>
              <MyHandle />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/payments"
        element={
          <RequireAuth>
            <AppShell>
              <Payments />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/send"
        element={
          <RequireAuth>
            <AppShell>
              <Send />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/identities"
        element={
          <RequireAuth>
            <AppShell>
              <Identities />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/settings"
        element={
          <RequireAuth>
            <AppShell>
              <Settings />
            </AppShell>
          </RequireAuth>
        }
      />

      <Route
        path="*"
        element={
          <MarketingShell>
            <NotFound />
          </MarketingShell>
        }
      />
    </Routes>
  );
}
