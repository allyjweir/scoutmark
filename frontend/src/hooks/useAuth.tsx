import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import type { User } from '../lib/types';
import * as api from '../lib/api';

interface AuthContextValue {
  user: User | null;
  loading: boolean;
  passwordChangeRequired: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  clearPasswordChangeRequired: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [passwordChangeRequired, setPasswordChangeRequired] = useState(false);

  useEffect(() => {
    const token = localStorage.getItem('session_token');
    if (!token) {
      setLoading(false);
      return;
    }

    api.getCurrentUser()
      .then(setUser)
      .catch(() => {
        localStorage.removeItem('session_token');
      })
      .finally(() => setLoading(false));
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const result = await api.login(username, password);
    setUser(result.user);
    setPasswordChangeRequired(result.password_change_required);
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // Always log out client-side even if server call fails
    }
    localStorage.removeItem('session_token');
    setUser(null);
    setPasswordChangeRequired(false);
  }, []);

  const clearPasswordChangeRequired = useCallback(() => {
    setPasswordChangeRequired(false);
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, passwordChangeRequired, login, logout, clearPasswordChangeRequired }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = (): AuthContextValue => {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
};
