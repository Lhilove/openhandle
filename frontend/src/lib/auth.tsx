import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, setCsrfToken, type Me } from "./api";

interface AuthContextValue {
  me: Me | null;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const applyMe = useCallback((next: Me | null) => {
    setMe(next);
    setCsrfToken(next?.csrf_token);
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api.me();
      applyMe(next);
      setError(null);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        applyMe(null);
        setError(null);
      } else {
        setError(e instanceof Error ? e.message : "Could not load your account.");
      }
    } finally {
      setLoading(false);
    }
  }, [applyMe]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(
    async (email: string, password: string) => {
      const next = await api.login(email, password);
      applyMe(next);
    },
    [applyMe],
  );

  const register = useCallback(
    async (email: string, password: string) => {
      const next = await api.register(email, password);
      applyMe(next);
    },
    [applyMe],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      applyMe(null);
    }
  }, [applyMe]);

  return (
    <AuthContext.Provider value={{ me, loading, error, refresh, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}
