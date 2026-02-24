import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, clearApiAuthToken, setApiAuthToken } from '../api';
import type { User } from '../types';

interface AuthContextType {
  user: User | null;
  login: (username: string, authToken?: string) => Promise<boolean>;
  logout: () => void;
  isLoading: boolean;
  error: string | null;
}

const STORAGE_KEY = 'poker-arena-user';

function loadStoredUser(): User | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return null;
    }

    const parsed = JSON.parse(raw) as Partial<User>;
    if (typeof parsed.name !== 'string' || typeof parsed.token !== 'string' || typeof parsed.id !== 'string') {
      return null;
    }
    return {
      id: parsed.id,
      name: parsed.name,
      token: parsed.token,
      role: parsed.role === 'observer' ? 'observer' : 'admin',
      seatNo: typeof parsed.seatNo === 'number' ? parsed.seatNo : undefined,
    };
  } catch {
    return null;
  }
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(loadStoredUser());
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (user?.token) {
      setApiAuthToken(user.token);
      return;
    }
    clearApiAuthToken();
  }, [user]);

  useEffect(() => {
    if (!user) {
      return;
    }

    let active = true;
    const refreshSession = async () => {
      try {
        const session = await api.getSession();
        if (!active) {
          return;
        }
        setUser((prev) => {
          if (!prev) {
            return prev;
          }
          const next: User = { ...prev, role: session.role, seatNo: session.seatNo };
          localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
          return next;
        });
      } catch {
        // Keep existing local session when backend is temporarily unavailable.
      }
    };

    void refreshSession();
    return () => {
      active = false;
    };
  }, [user?.id]);

  const login = async (username: string, authToken?: string): Promise<boolean> => {
    setIsLoading(true);
    setError(null);

    try {
      const profile = await api.login(username, authToken);
      setUser(profile);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(profile));
      return true;
    } catch (caught) {
      console.error(caught);
      setError('Unable to sign in right now. Please try again.');
      return false;
    } finally {
      setIsLoading(false);
    }
  };

  const logout = () => {
    setUser(null);
    clearApiAuthToken();
    localStorage.removeItem(STORAGE_KEY);
  };

  const value = useMemo(
    () => ({
      user,
      login,
      logout,
      isLoading,
      error,
    }),
    [error, isLoading, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be called inside AuthProvider');
  }

  return context;
}
