"use client";

import { useCallback, useEffect, useReducer} from "react";
import { userClient } from "@/lib/api";
import type { User, UserBudget, BudgetGrant } from "@/gen/types/user_pb";
import { UserRole } from "@/gen/types/user_pb";

export interface CurrentUser {
  user: User | null;
  budget: UserBudget | null;
  activeGrants: BudgetGrant[];
  totalRemainingUsd: number;
  isAdmin: boolean;
  isLoading: boolean;
  error: string | null;
}

type Action =
  | { type: "loading" }
  | { type: "success"; user: User; budget: UserBudget | null; grants: BudgetGrant[]; remaining: number }
  | { type: "error"; message: string };

function reducer(state: CurrentUser, action: Action): CurrentUser {
  switch (action.type) {
    case "loading":
      return { ...state, isLoading: true, error: null };
    case "success":
      return {
        user: action.user,
        budget: action.budget,
        activeGrants: action.grants,
        totalRemainingUsd: action.remaining,
        isAdmin: action.user.role === UserRole.ADMIN,
        isLoading: false,
        error: null,
      };
    case "error":
      return { ...state, isLoading: false, error: action.message };
  }
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

/**
 * Fetches the current authenticated user's profile, budget, and grants.
 * Caches the result for the lifetime of the component tree.
 */
export function useCurrentUser(): CurrentUser {
  const [state, dispatch] = useReducer(reducer, initialState);

  const fetchUser = useCallback(async () => {
    dispatch({ type: "loading" });
    try {
      const resp = await userClient.getCurrentUser({});
      if (resp.user) {
        dispatch({
          type: "success",
          user: resp.user,
          budget: resp.budget ?? null,
          grants: resp.activeGrants,
          remaining: resp.totalRemainingUsd,
        });
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to load user";
      dispatch({ type: "error", message });
    }
  }, []);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  return state;
}
