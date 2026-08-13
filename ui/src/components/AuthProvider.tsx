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
    const unsubscribe = onAuthStateChanged(firebaseAuth, (user) => {
      setUser(user);
      setLoading(false);
    });
    return unsubscribe;
  }, []);

  const clearAuthError = () => setAuthError(null);

  const signIn = async () => {
    if (!firebaseAuth) return;
    try {
      await signInWithPopup(firebaseAuth, googleProvider);
      setAuthError(null);
    } catch (err: any) {
      if (err.code === "auth/popup-blocked") {
        setAuthError("Sign-in popup was blocked. Please allow popups.");
      } else if (err.code === "auth/popup-closed-by-user") {
        // Silently ignore
      } else {
        setAuthError(err.message || "Failed to sign in");
      }
      throw err;
    }
  };

  const signOut = async () => {
    if (!firebaseAuth) return;
    try {
      await firebaseSignOut(firebaseAuth);
      setAuthError(null);
    } catch (err: any) {
      setAuthError(err.message || "Failed to sign out");
      throw err;
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
