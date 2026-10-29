"use client";

import { useSyncExternalStore } from "react";
import { userClient } from "@/lib/api";
import type { User, UserBudget, BudgetGrant } from "@/gen/candela/types/user_pb";
import { UserRole } from "@/gen/candela/types/user_pb";

export interface CurrentUser {
  user: User | null;
  budget: UserBudget | null;
  activeGrants: BudgetGrant[];
  totalRemainingUsd: number;
  isAdmin: boolean;
  isLoading: boolean;
  error: string | null;
}

const initialState: CurrentUser = {
  user: null,
  budget: null,
  activeGrants: [],
  totalRemainingUsd: 0,
  isAdmin: false,
  isLoading: true,
  error: null,
};

type Listener = () => void;

let sharedState: CurrentUser = initialState;
const listeners = new Set<Listener>();
let inFlightController: AbortController | null = null;
let hasFetchedWhileMounted = false;

function publish(next: CurrentUser) {
  sharedState = next;
  for (const listener of listeners) {
    listener();
  }
}

async function startSharedFetch() {
  inFlightController?.abort();
  const controller = new AbortController();
  inFlightController = controller;
  hasFetchedWhileMounted = true;

  try {
    const resp = await userClient.getCurrentUser({}, { signal: controller.signal });
    if (controller.signal.aborted) return;

    if (resp.user) {
      const roleStr = resp.user.role === UserRole.ADMIN ? "admin" : "developer";
      const secure =
        typeof window !== "undefined" && window.location.protocol === "https:"
          ? "; Secure"
          : "";
      if (typeof document !== "undefined") {
        document.cookie = `candela_role=${roleStr}; path=/; max-age=3600; SameSite=Lax${secure}`;
      }
      publish({
        user: resp.user,
        budget: resp.budget ?? null,
        activeGrants: resp.activeGrants,
        totalRemainingUsd: resp.totalRemainingUsd,
        isAdmin: resp.user.role === UserRole.ADMIN,
        isLoading: false,
        error: null,
      });
    } else {
      publish({
        ...initialState,
        isLoading: false,
        error: null,
      });
    }
  } catch (err: unknown) {
    if (controller.signal.aborted) return;
    const message = err instanceof Error ? err.message : "Failed to load user";
    publish({ ...sharedState, isLoading: false, error: message });
  } finally {
    if (inFlightController === controller) {
      inFlightController = null;
    }
  }
}

function subscribe(listener: Listener): () => void {
  listeners.add(listener);
  if (!hasFetchedWhileMounted) {
    void startSharedFetch();
  }

  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      inFlightController?.abort();
      inFlightController = null;
      hasFetchedWhileMounted = false;
      sharedState = initialState;
    }
  };
}

function getSnapshot(): CurrentUser {
  return sharedState;
}

function getServerSnapshot(): CurrentUser {
  return initialState;
}

/**
 * Fetches the current authenticated user's profile, budget, and grants.
 * Shares state and deduplicates the getCurrentUser RPC across all mounted
 * consumers in the component tree.
 */
export function useCurrentUser(): CurrentUser {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
