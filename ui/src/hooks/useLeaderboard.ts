"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { type TimeRange, timeRangeToMs } from "@/lib/timeUtils";

export interface UserRanking {
  userId: string;
  email: string;
  displayName: string;
  callCount: number;
  totalTokens: number;
  costUsd: number;
  avgLatencyMs: number;
  topModel: string;
}

type State = {
  rankings: UserRanking[];
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; rankings: UserRanking[] }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, rankings: action.rankings };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
    case "setTimeRange":
      if (state.timeRange === action.range) return state;
      return { ...state, timeRange: action.range };
  }
}

export function useLeaderboard() {
  const [state, dispatch] = useReducer(reducer, {
    rankings: [],
    loading: true,
    error: null,
    timeRange: "7d",
    fetchCount: 0,
  });

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
    dispatch({ type: "fetch" });

    const now = new Date();
    const start = new Date(now.getTime() - timeRangeToMs(state.timeRange));

    dashboardClient.getTeamLeaderboard({
      projectId: DEFAULT_PROJECT_ID,
      timeRange: {
        start: timestampFromDate(start),
        end: timestampFromDate(now),
      },
      limit: 20,
    }, { signal })
      .then((res) => {
        if (signal.aborted) return;
        dispatch({
          type: "success",
          rankings: res.users.map((u) => ({
            userId: u.userId,
            email: u.email,
            displayName: u.displayName || u.email.split("@")[0],
            callCount: Number(u.callCount),
            totalTokens: Number(u.totalTokens),
            costUsd: u.costUsd,
            avgLatencyMs: u.avgLatencyMs,
            topModel: u.topModel || "—",
          })),
        });
      })
      .catch((err) => {
        if (!signal.aborted) dispatch({ type: "error", message: err.message });
      });

    return () => controller.abort();
  }, [state.fetchCount, state.timeRange]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);
  const setTimeRange = useCallback(
    (range: TimeRange) => dispatch({ type: "setTimeRange", range }),
    []
  );

  return {
    rankings: state.rankings,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}
