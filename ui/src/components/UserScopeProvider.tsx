"use client";

import React, { createContext, useCallback, useContext, useMemo, useState } from "react";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

export type ScopeMode = "personal" | "global";

export interface UserScopeContextValue {
  /** Current data scope — "personal" shows only the current user's data */
  mode: ScopeMode;
  /** Toggle between personal and global views */
  setMode: (mode: ScopeMode) => void;
  /** Whether the GetDashboardData RPC should set include_budget=true */
  includeBudget: boolean;
  /** Whether the user is viewing their own data */
  isPersonalScope: boolean;
}

const STORAGE_KEY = "candela-scope-mode";

function getInitialMode(): ScopeMode {
  if (typeof window === "undefined") return "personal";
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "global" || stored === "personal") return stored;
  return "personal"; // Default: desktop users see their own data
}

// ──────────────────────────────────────────
// Context
// ──────────────────────────────────────────

const UserScopeContext = createContext<UserScopeContextValue | undefined>(undefined);

export function UserScopeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setModeRaw] = useState<ScopeMode>(getInitialMode);

  const setMode = useCallback((m: ScopeMode) => {
    setModeRaw(m);
    try { localStorage.setItem(STORAGE_KEY, m); } catch { /* SSR / privacy */ }
  }, []);

  const value = useMemo<UserScopeContextValue>(
    () => ({
      mode,
      setMode,
      includeBudget: mode === "personal",
      isPersonalScope: mode === "personal",
    }),
    [mode, setMode]
  );

  return (
    <UserScopeContext.Provider value={value}>
      {children}
    </UserScopeContext.Provider>
  );
}

/**
 * Read the current data scope.
 * Must be used inside <UserScopeProvider>.
 */
export function useScope(): UserScopeContextValue {
  const ctx = useContext(UserScopeContext);
  if (!ctx) {
    throw new Error("useScope must be used within <UserScopeProvider>");
  }
  return ctx;
}
