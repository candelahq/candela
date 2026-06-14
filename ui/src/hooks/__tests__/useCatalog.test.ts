import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useCatalog } from '../useCatalog';

// ---------------------------------------------------------------------------
// Mock catalogClient — the hook imports it from "@/lib/api".
// We replace listModelCatalog with a vi.fn() we control per-test.
// ---------------------------------------------------------------------------

const mockListModelCatalog = vi.fn();

vi.mock('@/lib/api', () => ({
  catalogClient: {
    listModelCatalog: (...args: unknown[]) => mockListModelCatalog(...args),
  },
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Build a minimal ModelCatalogEntry-shaped object for tests. */
function catalogEntry(overrides: {
  modelId: string;
  provider: string;
  inputPerMillion?: number;
  outputPerMillion?: number;
}) {
  return {
    modelId: overrides.modelId,
    provider: overrides.provider,
    displayName: overrides.modelId,
    inputPerMillion: overrides.inputPerMillion ?? 0,
    outputPerMillion: overrides.outputPerMillion ?? 0,
    enabled: true,
    category: 'chat',
    contextWindow: BigInt(128000),
    inputPerMillionHigh: 0,
    outputPerMillionHigh: 0,
    tierThresholdTokens: BigInt(0),
    aliases: [],
    allowedTenants: [],
    discountPercent: 0,
    $typeName: 'candela.types.ModelCatalogEntry' as const,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useCatalog', () => {
  beforeEach(() => {
    mockListModelCatalog.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('starts in loading state', () => {
    // Never resolve — we just want to check initial state.
    mockListModelCatalog.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useCatalog());

    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.models).toEqual([]);
  });

  it('fetches catalog on mount and transitions to success', async () => {
    const models = [
      catalogEntry({ modelId: 'gemini-2.5-pro', provider: 'google', inputPerMillion: 1.25, outputPerMillion: 10 }),
      catalogEntry({ modelId: 'claude-sonnet-4', provider: 'anthropic', inputPerMillion: 3, outputPerMillion: 15 }),
    ];
    mockListModelCatalog.mockResolvedValue({
      models,
      source: 'database',
      adminEditable: true,
    });

    const { result } = renderHook(() => useCatalog());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBeNull();
    expect(result.current.models).toHaveLength(2);
    expect(result.current.source).toBe('database');
    expect(result.current.adminEditable).toBe(true);
  });

  it('sets error state on RPC failure', async () => {
    mockListModelCatalog.mockRejectedValue(new Error('network down'));

    const { result } = renderHook(() => useCatalog());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe('network down');
    expect(result.current.models).toEqual([]);
  });

  it('sets generic error for non-Error throws', async () => {
    mockListModelCatalog.mockRejectedValue('string error');

    const { result } = renderHook(() => useCatalog());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe('Failed to load catalog');
  });

  // ---------------------------------------------------------------------------
  // getPricing
  // ---------------------------------------------------------------------------

  describe('getPricing()', () => {
    beforeEach(() => {
      mockListModelCatalog.mockResolvedValue({
        models: [
          catalogEntry({ modelId: 'gemini-2.5-pro', provider: 'google', inputPerMillion: 1.25, outputPerMillion: 10 }),
          catalogEntry({ modelId: 'gpt-4o', provider: 'openai', inputPerMillion: 5, outputPerMillion: 15 }),
        ],
        source: 'config',
        adminEditable: false,
      });
    });

    it('returns pricing for known provider/model', async () => {
      const { result } = renderHook(() => useCatalog());
      await waitFor(() => expect(result.current.loading).toBe(false));

      const pricing = result.current.getPricing('google', 'gemini-2.5-pro');
      expect(pricing).toEqual({ inputPerMillion: 1.25, outputPerMillion: 10 });
    });

    it('returns pricing via model-only fallback', async () => {
      const { result } = renderHook(() => useCatalog());
      await waitFor(() => expect(result.current.loading).toBe(false));

      // Wrong provider, but model exists — falls back to model-only key.
      const pricing = result.current.getPricing('unknown-provider', 'gpt-4o');
      expect(pricing).toEqual({ inputPerMillion: 5, outputPerMillion: 15 });
    });

    it('is case-insensitive', async () => {
      const { result } = renderHook(() => useCatalog());
      await waitFor(() => expect(result.current.loading).toBe(false));

      const pricing = result.current.getPricing('Google', 'Gemini-2.5-Pro');
      expect(pricing).toEqual({ inputPerMillion: 1.25, outputPerMillion: 10 });
    });

    it('returns null for completely unknown model', async () => {
      const { result } = renderHook(() => useCatalog());
      await waitFor(() => expect(result.current.loading).toBe(false));

      expect(result.current.getPricing('foo', 'bar-model')).toBeNull();
    });
  });

  // ---------------------------------------------------------------------------
  // refresh
  // ---------------------------------------------------------------------------

  it('refresh() re-fetches and updates state', async () => {
    mockListModelCatalog.mockResolvedValueOnce({
      models: [catalogEntry({ modelId: 'model-v1', provider: 'acme' })],
      source: 'config',
      adminEditable: false,
    });

    const { result } = renderHook(() => useCatalog());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.models).toHaveLength(1);

    // Prepare next response
    mockListModelCatalog.mockResolvedValueOnce({
      models: [
        catalogEntry({ modelId: 'model-v1', provider: 'acme' }),
        catalogEntry({ modelId: 'model-v2', provider: 'acme' }),
      ],
      source: 'database',
      adminEditable: true,
    });

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.models).toHaveLength(2);
    expect(result.current.source).toBe('database');
  });

  // ---------------------------------------------------------------------------
  // AbortController on unmount
  // ---------------------------------------------------------------------------

  it('aborts in-flight request on unmount', async () => {
    let capturedSignal: AbortSignal | null = null;
    mockListModelCatalog.mockImplementation((_req: unknown, opts: { signal: AbortSignal }) => {
      capturedSignal = opts?.signal ?? null;
      return new Promise(() => {}); // Never resolves
    });

    const { unmount } = renderHook(() => useCatalog());
    await waitFor(() => expect(mockListModelCatalog).toHaveBeenCalled());

    unmount();

    expect(capturedSignal).not.toBeNull();
    expect(capturedSignal!.aborted).toBe(true);
  });
});
