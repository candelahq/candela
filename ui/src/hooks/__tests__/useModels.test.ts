import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import type { ModelUsageRow } from '../useDashboard';
import type { CacheEfficiency } from '@/lib/cacheUtils';

// ---------------------------------------------------------------------------
// Mock useDashboard — useModels consumes it internally.
// ---------------------------------------------------------------------------

const mockSetTimeRange = vi.fn();
const mockRefresh = vi.fn();

/** Default useDashboard return value — overridden per-test. */
let dashboardReturn: {
  models: ModelUsageRow[];
  summary: null;
  recentTraces: never[];
  budgetContext: null;
  loading: boolean;
  error: string | null;
  timeRange: '24h' | '7d' | '30d';
  setTimeRange: typeof mockSetTimeRange;
  refresh: typeof mockRefresh;
};

function resetDashboard(overrides?: Partial<typeof dashboardReturn>) {
  dashboardReturn = {
    models: [],
    summary: null,
    recentTraces: [],
    budgetContext: null,
    loading: false,
    error: null,
    timeRange: '24h',
    setTimeRange: mockSetTimeRange,
    refresh: mockRefresh,
    ...overrides,
  };
}

vi.mock('@/hooks/useDashboard', () => ({
  useDashboard: vi.fn(() => dashboardReturn),
}));

// ---------------------------------------------------------------------------
// Mock useCatalog — useModels enriches rows with getPricing.
// ---------------------------------------------------------------------------

type PricingResult = { inputPerMillion: number; outputPerMillion: number } | null;
let pricingMap: Map<string, PricingResult>;

function resetCatalog(overrides?: {
  loading?: boolean;
  error?: string | null;
  source?: string;
}) {
  pricingMap = new Map();
  catalogReturn = {
    getPricing: vi.fn((provider: string, model: string) => {
      const key = `${provider.toLowerCase()}:${model.toLowerCase()}`;
      return pricingMap.get(key) ?? null;
    }),
    loading: overrides?.loading ?? false,
    error: overrides?.error ?? null,
    source: overrides?.source ?? 'config',
    models: [],
    adminEditable: false,
    refresh: vi.fn(),
  };
}

let catalogReturn: {
  getPricing: ReturnType<typeof vi.fn>;
  loading: boolean;
  error: string | null;
  source: string;
  models: unknown[];
  adminEditable: boolean;
  refresh: ReturnType<typeof vi.fn>;
};

vi.mock('@/hooks/useCatalog', () => ({
  useCatalog: vi.fn(() => catalogReturn),
}));

// ---------------------------------------------------------------------------
// Import useModels *after* vi.mock so mocks are in place.
// ---------------------------------------------------------------------------

import { useModels } from '../useModels';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function modelRow(overrides: Partial<ModelUsageRow> & { model: string; provider: string }): ModelUsageRow {
  return {
    callCount: 10,
    inputTokens: 5000,
    outputTokens: 3000,
    costUsd: 1.0,
    avgLatencyMs: 150,
    cacheReadTokens: 0,
    cacheCreationTokens: 0,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useModels', () => {
  beforeEach(() => {
    resetDashboard();
    resetCatalog();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── 1. Enriches models with pricing ─────────────────────────────────

  it('enriches models with pricing from catalog', () => {
    const models = [
      modelRow({ model: 'gemini-2.5-pro', provider: 'google' }),
    ];
    resetDashboard({ models });
    pricingMap.set('google:gemini-2.5-pro', {
      inputPerMillion: 1.25,
      outputPerMillion: 10,
    });

    const { result } = renderHook(() => useModels());

    expect(result.current.models).toHaveLength(1);
    expect(result.current.models[0].inputPricePerMillion).toBe(1.25);
    expect(result.current.models[0].outputPricePerMillion).toBe(10);
  });

  // ── 2. Returns null pricing for unknown model ───────────────────────

  it('returns null pricing for a model not in catalog', () => {
    const models = [
      modelRow({ model: 'mystery-model', provider: 'unknown-provider' }),
    ];
    resetDashboard({ models });
    // pricingMap is empty — no catalog entry

    const { result } = renderHook(() => useModels());

    expect(result.current.models).toHaveLength(1);
    expect(result.current.models[0].inputPricePerMillion).toBeNull();
    expect(result.current.models[0].outputPricePerMillion).toBeNull();
  });

  // ── 3. Search filter ────────────────────────────────────────────────

  it('filters models by search term', () => {
    const models = [
      modelRow({ model: 'gemini-2.5-pro', provider: 'google', costUsd: 2 }),
      modelRow({ model: 'claude-sonnet-4', provider: 'anthropic', costUsd: 3 }),
      modelRow({ model: 'gpt-4o', provider: 'openai', costUsd: 1 }),
    ];
    resetDashboard({ models });

    const { result } = renderHook(() => useModels());
    expect(result.current.models).toHaveLength(3);

    act(() => {
      result.current.setSearch('claude');
    });

    expect(result.current.models).toHaveLength(1);
    expect(result.current.models[0].model).toBe('claude-sonnet-4');
  });

  // ── 4. Sort toggle ─────────────────────────────────────────────────

  it('toggles sort order', () => {
    const models = [
      modelRow({ model: 'alpha', provider: 'google', costUsd: 1 }),
      modelRow({ model: 'bravo', provider: 'google', costUsd: 3 }),
      modelRow({ model: 'charlie', provider: 'google', costUsd: 2 }),
    ];
    resetDashboard({ models });

    const { result } = renderHook(() => useModels());

    // Default sort is costUsd desc
    expect(result.current.sort).toEqual({ key: 'costUsd', desc: true });
    expect(result.current.models[0].model).toBe('bravo');
    expect(result.current.models[2].model).toBe('alpha');

    // Toggle same key — should flip to asc
    act(() => {
      result.current.toggleSort('costUsd');
    });

    expect(result.current.sort).toEqual({ key: 'costUsd', desc: false });
    expect(result.current.models[0].model).toBe('alpha');
    expect(result.current.models[2].model).toBe('bravo');

    // Toggle different key — should reset to desc
    act(() => {
      result.current.toggleSort('model');
    });

    expect(result.current.sort).toEqual({ key: 'model', desc: true });
    expect(result.current.models[0].model).toBe('charlie');
  });

  // ── 5. Cache efficiency ─────────────────────────────────────────────

  it('populates cacheEfficiency for model with cache tokens', () => {
    const models = [
      modelRow({
        model: 'gemini-2.5-pro',
        provider: 'google',
        cacheReadTokens: 3000,
        inputTokens: 5000,
      }),
    ];
    resetDashboard({ models });

    const { result } = renderHook(() => useModels());

    const eff: CacheEfficiency | null = result.current.models[0].cacheEfficiency;
    expect(eff).not.toBeNull();
    expect(eff!.rate).toBeCloseTo(0.6);
    expect(eff!.tier).toBe('excellent');
  });

  // ── 6. Combined loading state ───────────────────────────────────────

  it('returns loading=true when dashboard is loading', () => {
    resetDashboard({ loading: true });
    resetCatalog({ loading: false });

    const { result } = renderHook(() => useModels());
    expect(result.current.loading).toBe(true);
  });

  it('returns loading=true when catalog is loading', () => {
    resetDashboard({ loading: false });
    resetCatalog({ loading: true });

    const { result } = renderHook(() => useModels());
    expect(result.current.loading).toBe(true);
  });

  // ── 7. Combined error state ─────────────────────────────────────────

  it('surfaces dashboard error preferentially', () => {
    resetDashboard({ error: 'dashboard failure' });
    resetCatalog({ error: 'catalog failure' });

    const { result } = renderHook(() => useModels());
    expect(result.current.error).toBe('dashboard failure');
  });

  it('surfaces catalog error when dashboard has none', () => {
    resetDashboard({ error: null });
    resetCatalog({ error: 'catalog failure' });

    const { result } = renderHook(() => useModels());
    expect(result.current.error).toBe('catalog failure');
  });
});
