"use client";

import { useCallback, useEffect, useReducer, useMemo, useRef } from "react";
import { catalogClient } from "@/lib/api";
import type { ModelCatalogEntry } from "@/gen/candela/types/model_catalog_pb";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

interface CatalogState {
  models: ModelCatalogEntry[];
  source: string;
  adminEditable: boolean;
  loading: boolean;
  error: string | null;
}

type CatalogAction =
  | { type: "LOADING" }
  | { type: "SUCCESS"; models: ModelCatalogEntry[]; source: string; adminEditable: boolean }
  | { type: "ERROR"; error: string };

function reducer(state: CatalogState, action: CatalogAction): CatalogState {
  switch (action.type) {
    case "LOADING":
      return { ...state, loading: true, error: null };
    case "SUCCESS":
      return { models: action.models, source: action.source, adminEditable: action.adminEditable, loading: false, error: null };
    case "ERROR":
      return { ...state, loading: false, error: action.error };
  }
}

export interface CatalogPricing {
  inputPerMillion: number;
  outputPerMillion: number;
}

// ──────────────────────────────────────────
// Hook
// ──────────────────────────────────────────

/**
 * Hook for fetching the model catalog via ListModelCatalog RPC.
 *
 * Returns the full list of catalog entries, metadata (source, adminEditable),
 * loading/error state, a refresh function, and a pricing lookup helper.
 */
export function useCatalog() {
  const [state, dispatch] = useReducer(reducer, {
    models: [],
    source: "",
    adminEditable: false,
    loading: true,
    error: null,
  });

  const fetchRef = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    fetchRef.current?.abort();
    const controller = new AbortController();
    fetchRef.current = controller;

    dispatch({ type: "LOADING" });
    try {
      const resp = await catalogClient.listModelCatalog({}, { signal: controller.signal });
      if (!controller.signal.aborted) {
        dispatch({
          type: "SUCCESS",
          models: resp.models,
          source: resp.source,
          adminEditable: resp.adminEditable,
        });
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        dispatch({ type: "ERROR", error: err instanceof Error ? err.message : "Failed to load catalog" });
      }
    }
  }, []);

  useEffect(() => {
    refresh();
    return () => fetchRef.current?.abort();
  }, [refresh]);

  // Build a pricing lookup map indexed by provider/model and model-only (fallback).
  const pricingMap = useMemo(() => {
    const map = new Map<string, CatalogPricing>();
    for (const m of state.models) {
      if (!m.provider || !m.modelId) continue; // skip invalid entries
      const pricing: CatalogPricing = {
        inputPerMillion: m.inputPerMillion ?? 0,
        outputPerMillion: m.outputPerMillion ?? 0,
      };
      // Primary key: provider/modelId
      map.set(`${m.provider}/${m.modelId}`.toLowerCase(), pricing);
      // Fallback key: modelId only
      map.set(m.modelId.toLowerCase(), pricing);
    }
    return map;
  }, [state.models]);

  const getPricing = useCallback(
    (provider: string, model: string): CatalogPricing | null => {
      return pricingMap.get(`${provider}/${model}`.toLowerCase())
        ?? pricingMap.get(model.toLowerCase())
        ?? null;
    },
    [pricingMap],
  );

  return {
    models: state.models,
    source: state.source,
    adminEditable: state.adminEditable,
    loading: state.loading,
    error: state.error,
    refresh,
    getPricing,
  };
}
