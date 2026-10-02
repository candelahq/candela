"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import {
  onAuthStateChanged,
  signInWithPopup,
  signOut as firebaseSignOut,
  type User,
} from "firebase/auth";
import { firebaseAuth, googleProvider } from "@/lib/firebase";

interface AuthContextType {
  user: User | null;
  loading: boolean;
  /** Whether Firebase Auth is configured (false in local dev without env vars). */
  configured: boolean;
  signIn: () => Promise<void>;
  signOut: () => Promise<void>;
  getIdToken: () => Promise<string | null>;
  authError: string | null;
  clearAuthError: () => void;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  loading: true,
  configured: false,
  signIn: async () => {},
  signOut: async () => {},
  getIdToken: async () => null,
  authError: null,
  clearAuthError: () => {},
});

export function useAuth() {
  return useContext(AuthContext);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);
  const configured = firebaseAuth !== null;
  // Start as not-loading when Firebase isn't configured (no auth to wait for).
  const [loading, setLoading] = useState(configured);

  useEffect(() => {
    if (!firebaseAuth) return;
    const unsubscribe = onAuthStateChanged(firebaseAuth, async (user) => {
      setUser(user);
      setLoading(false);
      if (user) {
        try {
          const token = await user.getIdToken();
          const tokenResult = await user.getIdTokenResult();
          const role =
            (tokenResult.claims.role as string) ||
            (tokenResult.claims.admin ? "admin" : "developer");
          const secure =
            typeof window !== "undefined" && window.location.protocol === "https:"
              ? "; Secure"
              : "";
          document.cookie = `__session=${token}; path=/; max-age=3600; SameSite=Lax${secure}`;
          document.cookie = `candela_role=${role}; path=/; max-age=3600; SameSite=Lax${secure}`;
        } catch {
          // Token retrieval failed
        }
      } else {
        const secure =
          typeof window !== "undefined" && window.location.protocol === "https:"
            ? "; Secure"
            : "";
        document.cookie = `__session=; path=/; max-age=0; SameSite=Lax${secure}`;
        document.cookie = `candela_role=; path=/; max-age=0; SameSite=Lax${secure}`;
      }
    });
    return unsubscribe;
  }, []);

  const clearAuthError = () => setAuthError(null);

  const signIn = async () => {
    if (!firebaseAuth) return;
    try {
      await signInWithPopup(firebaseAuth, googleProvider);
      setAuthError(null);
    } catch (err: unknown) {
      const code = (err as { code?: string }).code;
      if (code === "auth/popup-closed-by-user") {
        return; // User cancelled — not an error
      }
      if (code === "auth/popup-blocked") {
        setAuthError("Sign-in popup was blocked. Please allow popups.");
      } else {
        setAuthError(err instanceof Error ? err.message : "Failed to sign in");
      }
    }
  };

  const signOut = async () => {
    if (!firebaseAuth) return;
    try {
      await firebaseSignOut(firebaseAuth);
      const secure =
        typeof window !== "undefined" && window.location.protocol === "https:"
          ? "; Secure"
          : "";
      document.cookie = `__session=; path=/; max-age=0; SameSite=Lax${secure}`;
      document.cookie = `candela_role=; path=/; max-age=0; SameSite=Lax${secure}`;
      setAuthError(null);
    } catch (err: unknown) {
      setAuthError(err instanceof Error ? err.message : "Failed to sign out");
    }
  };

  const getIdToken = async (): Promise<string | null> => {
    if (!firebaseAuth?.currentUser) return null;
    return firebaseAuth.currentUser.getIdToken();
  };

  return (
    <AuthContext.Provider
      value={{ user, loading, configured, signIn, signOut, getIdToken, authError, clearAuthError }}
    >
      {children}
    </AuthContext.Provider>
  );
}
