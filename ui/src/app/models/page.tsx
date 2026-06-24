"use client";

import { useState, useMemo, useEffect, useCallback } from "react";
import { create } from "@bufbuild/protobuf";
import { useModels, type ModelSortKey } from "@/hooks/useModels";
import { useCatalog } from "@/hooks/useCatalog";
import { useCatalogEntryValidation } from "@/hooks/useProtoValidation";
import { catalogClient } from "@/lib/api";
import { ModelCatalogEntrySchema } from "@/gen/candela/types/model_catalog_pb";
import { type CacheEfficiency } from "@/lib/cacheUtils";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ScopeToggle } from "@/components/ScopeToggle";
import { useScope } from "@/components/UserScopeProvider";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SkeletonCard } from "@/components/SkeletonCard";

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toLocaleString();
}

function fmtContextWindow(n: bigint): string {
  if (n <= BigInt(0)) return "—";
  const num = Number(n);
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(0)}k`;
  return num.toLocaleString();
}

// ──────────────────────────────────────────
// Sort header (shared)
// ──────────────────────────────────────────

function SortTh({
  label,
  sortKey,
  currentKey,
  desc,
  onSort,
  align,
}: {
  label: string;
  sortKey: ModelSortKey;
  currentKey: ModelSortKey;
  desc: boolean;
  onSort: (k: ModelSortKey) => void;
  align?: "right";
}) {
  const active = currentKey === sortKey;
  return (
    <th
      onClick={() => onSort(sortKey)}
      style={{ cursor: "pointer", textAlign: align ?? "left" }}
    >
      {label}{" "}
      {active && (
        <span style={{ fontSize: 10, opacity: 0.6 }}>
          {desc ? "▼" : "▲"}
        </span>
      )}
    </th>
  );
}

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

type Tab = "catalog" | "usage";

export default function ModelsPage() {
  const [activeTab, setActiveTab] = useState<Tab>("catalog");

  return (
    <>
      <header className="main-header">
        <h1>Models</h1>
        <div className="tab-bar">
          <button
            className={`tab-btn ${activeTab === "catalog" ? "tab-active" : ""}`}
            onClick={() => setActiveTab("catalog")}
          >
            📋 Catalog
          </button>
          <button
            className={`tab-btn ${activeTab === "usage" ? "tab-active" : ""}`}
            onClick={() => setActiveTab("usage")}
          >
            📊 Usage
          </button>
        </div>
      </header>

      <div className="main-body">
        <div style={{ display: activeTab === "catalog" ? "block" : "none" }}>
          <CatalogTab />
        </div>
        <div style={{ display: activeTab === "usage" ? "block" : "none" }}>
          <UsageTab />
        </div>
      </div>
    </>
  );
}

// ──────────────────────────────────────────
// Catalog Tab
// ──────────────────────────────────────────

const PROVIDERS = [
  "anthropic",
  "google",
  "gemini-oai",
  "mistral",
  "deepseek",
  "openai",
  "qwen",
] as const;

interface AddModelForm {
  modelId: string;
  provider: string;
  displayName: string;
  inputPerMillion: number | "";
  outputPerMillion: number | "";
  providerModelId: string;
  region: string;
  category: string;
  enabled: boolean;
}

const EMPTY_FORM: AddModelForm = {
  modelId: "",
  provider: "",
  displayName: "",
  inputPerMillion: "",
  outputPerMillion: "",
  providerModelId: "",
  region: "",
  category: "",
  enabled: true,
};

function CatalogTab() {
  const { models, source, adminEditable, loading, error, refresh } = useCatalog();
  const [search, setSearch] = useState("");
  // NOTE: toggle is local-only in v1. Persistence via UpdateCatalogEntry comes in a follow-up.
  const [enabledOverrides, setEnabledOverrides] = useState<Record<string, boolean>>({});

  // Add Model modal state
  const [showAddModal, setShowAddModal] = useState(false);
  const [addForm, setAddForm] = useState<AddModelForm>(EMPTY_FORM);
  const [createError, setCreateError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const { validate, getError, clearErrors } = useCatalogEntryValidation();

  const filtered = useMemo(() => {
    if (!search) return models;
    const q = search.toLowerCase();
    return models.filter(
      (m) =>
        m.modelId.toLowerCase().includes(q) ||
        (m.provider ?? "").toLowerCase().includes(q) ||
        (m.displayName ?? "").toLowerCase().includes(q) ||
        (m.category ?? "").toLowerCase().includes(q),
    );
  }, [models, search]);

  const getEnabled = (modelId: string, provider: string, defaultEnabled: boolean) => {
    const key = `${provider}/${modelId}`;
    return enabledOverrides[key] ?? defaultEnabled;
  };

  const toggleEnabled = (modelId: string, provider: string, currentEnabled: boolean) => {
    const key = `${provider}/${modelId}`;
    setEnabledOverrides((prev) => ({ ...prev, [key]: !currentEnabled }));
  };

  const handleCloseModal = useCallback(() => {
    setShowAddModal(false);
    setAddForm(EMPTY_FORM);
    setCreateError(null);
    clearErrors();
  }, [clearErrors]);

  // Close modal on Escape key
  useEffect(() => {
    if (!showAddModal) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") handleCloseModal();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [showAddModal, handleCloseModal]);

  const handleAddModel = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);

    const inputPrice = addForm.inputPerMillion === "" ? 0 : addForm.inputPerMillion;
    const outputPrice = addForm.outputPerMillion === "" ? 0 : addForm.outputPerMillion;

    const valid = await validate({
      modelId: addForm.modelId,
      provider: addForm.provider,
      displayName: addForm.displayName,
      inputPerMillion: inputPrice,
      outputPerMillion: outputPrice,
      enabled: addForm.enabled,
      category: addForm.category,
      providerModelId: addForm.providerModelId,
      region: addForm.region,
    });
    if (!valid) return;

    setSubmitting(true);
    try {
      await catalogClient.updateModelCatalogEntry({
        entry: create(ModelCatalogEntrySchema, {
          modelId: addForm.modelId,
          provider: addForm.provider,
          displayName: addForm.displayName,
          inputPerMillion: inputPrice,
          outputPerMillion: outputPrice,
          enabled: addForm.enabled,
          category: addForm.category,
          providerModelId: addForm.providerModelId,
          region: addForm.region,
        }),
      });
      handleCloseModal();
      refresh();
    } catch (err: unknown) {
      setCreateError(err instanceof Error ? err.message : "Failed to add model");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      {error && <ErrorBanner title="Catalog Error">{error}</ErrorBanner>}

      {/* Summary cards */}
      {loading && models.length === 0 ? (
        <div className="stats-grid animate-in">
          <SkeletonCard />
          <SkeletonCard />
          <SkeletonCard />
        </div>
      ) : (
        <div className="stats-grid animate-in">
          <div className="card">
            <div className="card-title">Total Models</div>
            <div className="card-value">{models.length}</div>
            <div className="card-subtitle">In catalog</div>
          </div>
          <div className="card">
            <div className="card-title">Active</div>
            <div className="card-value">
              {models.filter((m) => m.enabled).length}
            </div>
            <div className="card-subtitle">Enabled models</div>
          </div>
          <div className="card">
            <div className="card-title">Providers</div>
            <div className="card-value">
              {new Set(models.filter((m) => m.provider).map((m) => m.provider)).size}
            </div>
            <div className="card-subtitle">Unique providers</div>
          </div>
        </div>
      )}

      {/* Search + source + Add Model */}
      <div
        className="animate-in"
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          marginBottom: 16,
          animationDelay: "0.05s",
        }}
      >
        <div className="models-search" style={{ flex: 1 }}>
          <input
            type="text"
            className="models-search-input"
            placeholder="Search models, providers, categories…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        {source && (
          <span className="catalog-source">
            <span className="catalog-source-dot" />
            Source: {source}
          </span>
        )}
        <button className="btn" onClick={refresh}>
          🔄
        </button>
        {adminEditable && (
          <button
            className="btn btn-primary"
            onClick={() => setShowAddModal(true)}
          >
            + Add Model
          </button>
        )}
      </div>

      {/* Catalog table */}
      <div className="table-container animate-in" style={{ animationDelay: "0.08s" }}>
        <div className="table-header">
          <span className="table-title">Model Catalog</span>
          <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
            {filtered.length} model{filtered.length !== 1 ? "s" : ""}
            {adminEditable && (
              <span style={{ marginLeft: 8, color: "var(--accent)", fontWeight: 500 }}>
                • Admin editable
              </span>
            )}
          </span>
        </div>
        {loading && filtered.length === 0 ? (
          <div className="stats-grid animate-in">
            <SkeletonCard /><SkeletonCard /><SkeletonCard />
          </div>
        ) : filtered.length === 0 ? (
          <div className="empty-state" style={{ minHeight: 200 }}>
            <div className="empty-state-icon">📋</div>
            <div className="empty-state-title">
              {search ? "No matching models" : "No catalog data"}
            </div>
            <div className="empty-state-desc">
              {search
                ? "Try a different search term."
                : "The model catalog is empty."}
            </div>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Model</th>
                <th>Provider</th>
                <th>Category</th>
                <th style={{ textAlign: "right" }}>Input $/M</th>
                <th style={{ textAlign: "right" }}>Output $/M</th>
                <th style={{ textAlign: "right" }}>Context Window</th>
                <th style={{ textAlign: "center" }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((entry) => {
                const enabled = getEnabled(entry.modelId, entry.provider, entry.enabled);
                return (
                  <tr key={`${entry.provider}/${entry.modelId}`}>
                    <td>
                      <div>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {entry.modelId}
                        </span>
                        {entry.displayName && entry.displayName !== entry.modelId && (
                          <div style={{ fontSize: 11, color: "var(--text-muted)", marginTop: 2 }}>
                            {entry.displayName}
                          </div>
                        )}
                      </div>
                    </td>
                    <td>
                      <span className="badge badge-info">{entry.provider}</span>
                    </td>
                    <td>
                      {entry.category ? (
                        <span style={{ fontSize: 12, color: "var(--text-secondary)" }}>
                          {entry.category}
                        </span>
                      ) : (
                        <span style={{ color: "var(--text-muted)", fontSize: 11 }}>—</span>
                      )}
                    </td>
                    <td
                      style={{
                        textAlign: "right",
                        fontFamily: "monospace",
                        fontSize: 11,
                        color: "var(--text-secondary)",
                      }}
                    >
                      {entry.inputPerMillion > 0 ? `$${entry.inputPerMillion.toFixed(3)}` : "—"}
                    </td>
                    <td
                      style={{
                        textAlign: "right",
                        fontFamily: "monospace",
                        fontSize: 11,
                        color: "var(--text-secondary)",
                      }}
                    >
                      {entry.outputPerMillion > 0 ? `$${entry.outputPerMillion.toFixed(3)}` : "—"}
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <span className="context-window">
                        {entry.contextWindow ? fmtContextWindow(entry.contextWindow) : "—"}
                      </span>
                    </td>
                    <td style={{ textAlign: "center" }}>
                      {adminEditable ? (
                        <button
                          className={`badge ${enabled ? "badge-success" : "badge-warning"}`}
                          style={{ cursor: "pointer", border: "none" }}
                          onClick={() => toggleEnabled(entry.modelId, entry.provider, enabled)}
                          title={enabled ? "Click to disable" : "Click to enable"}
                        >
                          {enabled ? "Active" : "Disabled"}
                        </button>
                      ) : (
                        <span
                          className={`badge ${entry.enabled ? "badge-success" : "badge-warning"}`}
                        >
                          {entry.enabled ? "Active" : "Disabled"}
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Add Model Modal */}
      {showAddModal && (
        <div className="modal-overlay" onClick={handleCloseModal} role="dialog" aria-modal="true" aria-label="Add Model">
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Add Model</h3>
              <button className="modal-close" onClick={handleCloseModal} aria-label="Close dialog">×</button>
            </div>
            <form onSubmit={handleAddModel} className="modal-body">
              <div className="form-group">
                <label htmlFor="add-model-id">Model ID *</label>
                <input
                  id="add-model-id"
                  type="text"
                  required
                  value={addForm.modelId}
                  onChange={(e) => setAddForm({ ...addForm, modelId: e.target.value })}
                  placeholder="claude-sonnet-4"
                  className="form-input"
                />
                {getError("model_id") && <div className="form-field-error">{getError("model_id")}</div>}
              </div>
              <div className="form-group">
                <label htmlFor="add-provider">Provider *</label>
                <input
                  id="add-provider"
                  type="text"
                  required
                  list="provider-options"
                  value={addForm.provider}
                  onChange={(e) => setAddForm({ ...addForm, provider: e.target.value })}
                  placeholder="anthropic"
                  className="form-input"
                  autoComplete="off"
                />
                <datalist id="provider-options">
                  {PROVIDERS.map((p) => (
                    <option key={p} value={p} />
                  ))}
                </datalist>
                {getError("provider") && <div className="form-field-error">{getError("provider")}</div>}
              </div>
              <div className="form-group">
                <label htmlFor="add-display-name">Display Name</label>
                <input
                  id="add-display-name"
                  type="text"
                  value={addForm.displayName}
                  onChange={(e) => setAddForm({ ...addForm, displayName: e.target.value })}
                  placeholder="Claude Sonnet 4"
                  className="form-input"
                />
              </div>
              <div style={{ display: "flex", gap: 12 }}>
                <div className="form-group" style={{ flex: 1 }}>
                  <label htmlFor="add-input-price">Input $/M *</label>
                  <input
                    id="add-input-price"
                    type="number"
                    required
                    min="0"
                    step="0.001"
                    value={addForm.inputPerMillion}
                    onChange={(e) => setAddForm({ ...addForm, inputPerMillion: e.target.value === "" ? "" : Number(e.target.value) })}
                    placeholder="3.000"
                    className="form-input"
                  />
                  {getError("input_per_million") && <div className="form-field-error">{getError("input_per_million")}</div>}
                </div>
                <div className="form-group" style={{ flex: 1 }}>
                  <label htmlFor="add-output-price">Output $/M *</label>
                  <input
                    id="add-output-price"
                    type="number"
                    required
                    min="0"
                    step="0.001"
                    value={addForm.outputPerMillion}
                    onChange={(e) => setAddForm({ ...addForm, outputPerMillion: e.target.value === "" ? "" : Number(e.target.value) })}
                    placeholder="15.000"
                    className="form-input"
                  />
                  {getError("output_per_million") && <div className="form-field-error">{getError("output_per_million")}</div>}
                </div>
              </div>
              <div className="form-group">
                <label htmlFor="add-provider-model-id">Provider Model ID</label>
                <input
                  id="add-provider-model-id"
                  type="text"
                  value={addForm.providerModelId}
                  onChange={(e) => setAddForm({ ...addForm, providerModelId: e.target.value })}
                  placeholder="Upstream name when it differs"
                  className="form-input"
                />
              </div>
              <div style={{ display: "flex", gap: 12 }}>
                <div className="form-group" style={{ flex: 1 }}>
                  <label htmlFor="add-region">Region</label>
                  <input
                    id="add-region"
                    type="text"
                    value={addForm.region}
                    onChange={(e) => setAddForm({ ...addForm, region: e.target.value })}
                    placeholder="us-east5"
                    className="form-input"
                  />
                </div>
                <div className="form-group" style={{ flex: 1 }}>
                  <label htmlFor="add-category">Category</label>
                  <input
                    id="add-category"
                    type="text"
                    value={addForm.category}
                    onChange={(e) => setAddForm({ ...addForm, category: e.target.value })}
                    placeholder="flagship, lite, reasoning"
                    className="form-input"
                  />
                </div>
              </div>
              <div className="form-group">
                <label style={{ cursor: "pointer" }}>
                  <input
                    type="checkbox"
                    checked={addForm.enabled}
                    onChange={(e) => setAddForm({ ...addForm, enabled: e.target.checked })}
                    style={{ marginRight: 8 }}
                  />
                  Enabled
                </label>
              </div>
              {createError && <div className="form-error">{createError}</div>}
              <div className="modal-actions">
                <button type="button" className="btn btn-ghost" onClick={handleCloseModal}>
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>
                  {submitting ? "Adding..." : "Add Model"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}

// ──────────────────────────────────────────
// Usage Tab (existing page content)
// ──────────────────────────────────────────

function UsageTab() {
  const { includeBudget } = useScope();
  const {
    models,
    totals,
    loading,
    error,
    timeRange,
    setTimeRange,
    refresh,
    sort,
    toggleSort,
    search,
    setSearch,
    budgetContext,
  } = useModels({ includeBudget });

  return (
    <>
      {/* Controls */}
      <div className="animate-in" style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 16 }}>
        <ScopeToggle />
        <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
        <button className="btn" onClick={refresh}>🔄</button>
      </div>

      {error && <ErrorBanner title="Models Error">{error}</ErrorBanner>}

      {/* Summary cards */}
      {loading && models.length === 0 ? (
        <div className="stats-grid animate-in">
          <SkeletonCard />
          <SkeletonCard />
          <SkeletonCard />
          <SkeletonCard />
        </div>
      ) : (
        <div className="stats-grid animate-in">
          <div className="card">
            <div className="card-title">Models Used</div>
            <div className="card-value">{models.length}</div>
            <div className="card-subtitle">Unique models</div>
          </div>
          <div className="card">
            <div className="card-title">Total Calls</div>
            <div className="card-value">{totals.totalCalls.toLocaleString()}</div>
            <div className="card-subtitle">Across all models</div>
          </div>
          <div className="card">
            <div className="card-title">Total Tokens</div>
            <div className="card-value">
              {fmtTokens(totals.totalInputTokens + totals.totalOutputTokens)}
            </div>
            <div className="card-subtitle">
              {fmtTokens(totals.totalInputTokens)} in / {fmtTokens(totals.totalOutputTokens)} out
            </div>
          </div>
          <div className="card">
            <div className="card-title">Total Cost</div>
            <div className="card-value">${totals.totalCost.toFixed(2)}</div>
            <div className="card-subtitle">
              {budgetContext?.budget
                ? `$${budgetContext.totalRemainingUsd.toFixed(2)} remaining`
                : "Estimated USD"}
            </div>
          </div>
        </div>
      )}

      {/* Budget bar (if available) */}
      {budgetContext?.budget && (
        <div className="card animate-in" style={{ marginBottom: 16, animationDelay: "0.03s" }}>
          <div className="card-title">Budget</div>
          <div className="budget-bar-container">
            <div className="budget-bar-track">
              <div
                className="budget-bar-fill"
                style={{
                  width: `${(totals.totalCost + budgetContext.totalRemainingUsd) > 0 ? Math.min(100, (totals.totalCost / (totals.totalCost + budgetContext.totalRemainingUsd)) * 100) : 0}%`,
                }}
              />
            </div>
            <div className="budget-bar-labels">
              <span>${totals.totalCost.toFixed(2)} spent</span>
              <span>${budgetContext.totalRemainingUsd.toFixed(2)} remaining</span>
            </div>
          </div>
        </div>
      )}

      {/* Search */}
      <div className="models-search animate-in" style={{ animationDelay: "0.05s" }}>
        <input
          type="text"
          className="models-search-input"
          placeholder="Search models or providers…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      {/* Models table */}
      <div className="table-container animate-in" style={{ animationDelay: "0.08s" }}>
        <div className="table-header">
          <span className="table-title">Model Breakdown</span>
          <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
            {models.length} model{models.length !== 1 ? "s" : ""}
          </span>
        </div>
        {models.length === 0 ? (
          <div className="empty-state" style={{ minHeight: 200 }}>
            <div className="empty-state-icon">🤖</div>
            <div className="empty-state-title">
              {search ? "No matching models" : "No model data"}
            </div>
            <div className="empty-state-desc">
              {search
                ? "Try a different search term."
                : "Send requests through Candela to see model usage analytics."}
            </div>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <SortTh label="Model" sortKey="model" {...sort} currentKey={sort.key} onSort={toggleSort} />
                <SortTh label="Provider" sortKey="provider" {...sort} currentKey={sort.key} onSort={toggleSort} />
                <SortTh label="In $/M" sortKey="inputPrice" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Out $/M" sortKey="outputPrice" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Calls" sortKey="callCount" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Input Tokens" sortKey="inputTokens" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Output Tokens" sortKey="outputTokens" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Cost" sortKey="costUsd" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <SortTh label="Avg Latency" sortKey="avgLatencyMs" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                <th style={{ textAlign: "center" }}>Cache</th>
                <th style={{ textAlign: "right" }}>Cost %</th>
              </tr>
            </thead>
            <tbody>
              {models.map((m) => {
                const pct = totals.totalCost > 0 ? (m.costUsd / totals.totalCost) * 100 : 0;
                return (
                  <tr key={`${m.model}-${m.provider}`}>
                    <td>
                      <span className="mono" style={{ fontSize: 12 }}>{m.model}</span>
                    </td>
                    <td>
                      <span className="badge badge-info">{m.provider}</span>
                    </td>
                    <td style={{ textAlign: "right", fontFamily: "monospace", fontSize: 11, color: "var(--text-secondary)" }}>
                      {m.inputPricePerMillion != null ? `$${m.inputPricePerMillion.toFixed(3)}` : "—"}
                    </td>
                    <td style={{ textAlign: "right", fontFamily: "monospace", fontSize: 11, color: "var(--text-secondary)" }}>
                      {m.outputPricePerMillion != null ? `$${m.outputPricePerMillion.toFixed(3)}` : "—"}
                    </td>
                    <td style={{ textAlign: "right" }}>{m.callCount.toLocaleString()}</td>
                    <td style={{ textAlign: "right" }}>{fmtTokens(m.inputTokens)}</td>
                    <td style={{ textAlign: "right" }}>{fmtTokens(m.outputTokens)}</td>
                    <td style={{ textAlign: "right" }}>${m.costUsd.toFixed(4)}</td>
                    <td style={{ textAlign: "right" }}>{m.avgLatencyMs.toFixed(0)}ms</td>
                    <td style={{ textAlign: "center" }}>
                      {m.cacheEfficiency ? (
                        <CacheBadge eff={m.cacheEfficiency} />
                      ) : (
                        <span style={{ color: "var(--text-muted)", fontSize: 11 }}>—</span>
                      )}
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 8, justifyContent: "flex-end" }}>
                        <div style={{ width: 60, height: 4, background: "var(--bg-tertiary)", borderRadius: 2 }}>
                          <div
                            style={{
                              height: "100%",
                              width: `${Math.min(100, pct)}%`,
                              background: "var(--accent)",
                              borderRadius: 2,
                            }}
                          />
                        </div>
                        <span style={{ fontSize: 11, color: "var(--text-muted)", minWidth: 32 }}>
                          {pct.toFixed(1)}%
                        </span>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Cache stats */}
      {(totals.totalCacheRead > 0 || totals.totalCacheCreation > 0) && (
        <div className="card animate-in" style={{ marginTop: 16, animationDelay: "0.12s" }}>
          <div className="card-title">Cache Performance</div>
          <div className="settings-grid">
            <div className="settings-row">
              <span className="settings-label">Cache Read Tokens</span>
              <span className="settings-value">{fmtTokens(totals.totalCacheRead)}</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Cache Creation Tokens</span>
              <span className="settings-value">{fmtTokens(totals.totalCacheCreation)}</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Effective Hit Rate</span>
              <span className="settings-value">
                {totals.totalInputTokens > 0
                  ? `${((totals.totalCacheRead / totals.totalInputTokens) * 100).toFixed(1)}%`
                  : "—"}
              </span>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// ──────────────────────────────────────────
// Cache badge inline component
// ──────────────────────────────────────────

function CacheBadge({ eff }: { eff: CacheEfficiency }) {
  const color = eff.color;
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        fontSize: 10,
        fontWeight: 700,
        fontFamily: "monospace",
        color,
        background: `color-mix(in srgb, ${color} 12%, transparent)`,
        border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
        borderRadius: 6,
      }}
    >
      {eff.label} {(eff.rate * 100).toFixed(0)}%
    </span>
  );
}
