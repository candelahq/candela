# 💰 Cost Calculation Engine

Candela calculates the USD cost of every LLM API call in real-time, enabling budget tracking, threshold alerts, and per-user reporting.

## Core Principle

> **$0.00 is only valid for local models.** Every cloud model reachable through the proxy has real pricing. An unknown cloud model is a gap — the server logs a `⚠️ missing pricing` warning so operators can add it.

## How Cost Is Calculated

```
cost = (input_tokens / 1,000,000 × input_rate)
     + (output_tokens / 1,000,000 × output_rate)
     × (1 - model_discount)     # optional per-model discount
     × (1 - global_discount)    # optional enterprise-wide discount
```

---

## Pricing Resolution Order

When a model call completes, the calculator resolves pricing in this order:

```
1. Config overrides (exact provider/model match)     → use it
2. Built-in defaults (exact provider/model match)    → use it
3. Provider-agnostic match (model name only)         → use it
4. Provider is "local"?                              → $0.00 (correct)
5. Otherwise                                         → $0.00 + WARNING LOG
```

Step 3 handles cases like `gemini-oai/gemini-2.5-pro` matching the `google/gemini-2.5-pro` default pricing.

Step 5 means there's a gap — the model is missing from both the config and built-in defaults. Check the server logs for:

```
⚠️ missing pricing for cloud model — cost will be $0.00 (inaccurate)
  provider=openai model=some-new-model input_tokens=1500 output_tokens=300
```

---

## Built-In Default Pricing

The calculator ships with built-in list prices for **all cloud models** reachable through the Candela proxy (OpenAI, Google, Anthropic). These are standard list prices — not negotiated rates.

See the current pricing table in [`pkg/costcalc/pricing.yaml`](../pkg/costcalc/pricing.yaml), which is embedded at compile time via `//go:embed`. The `loadDefaults()` function in `calculator.go` parses this file at startup.

**Coverage:**
- **Google**: Gemini 3.5 Flash, 3.1 (Pro/Flash-Lite), 2.5 (Pro/Flash/Flash-Lite)
- **Anthropic**: Claude 4.8/4.7/4.6 Opus, Sonnet 4.6, Haiku 4.5, Claude 4 (short names + Vertex AI dated IDs)
- **OpenAI**: GPT-4.1 family (4.1/4.1-mini/4.1-nano), o3, o4-mini, GPT-4o (legacy)
- **Mistral**: mistral-small-2503, mistral-medium-3, codestral-2 (via Vertex AI rawPredict)
- **DeepSeek**: R1, V3 (via Vertex AI)
- **Qwen**: qwen3-235b, qwen3-coder-480b (via Vertex AI OpenAI-compat)
- **Z.AI**: GLM-5 (via Vertex AI OpenAI-compat)
- **Local**: Always $0.00 — runs on your hardware

### Pricing Degradation Chain

Candela supports multiple pricing sources with graceful degradation:

```
1. Firestore Model Catalog  (catalog.backend: "firestore")  → dynamic, admin-managed
2. Config YAML overrides    (pricing.models in config.yaml) → per-deployment negotiated rates
3. Frozen pricing.yaml      (//go:embed at compile time)    → built-in list prices
```

If the Firestore catalog is unavailable, the server falls back to config overrides, then to the embedded `pricing.yaml`. This ensures pricing is **always available** even in offline or degraded environments.

### Model ID Normalization

Vertex AI model IDs use hyphens for version suffixes (e.g., `claude-opus-4-7`), while pricing entries use dots (e.g., `claude-opus-4.7`). The `normalizeModelID()` function in `calculator.go` converts trailing `digit-digit` patterns automatically:

```
claude-opus-4-7             →  claude-opus-4.7   (trailing version normalized)
claude-sonnet-4-6           →  claude-sonnet-4.6  (trailing version normalized)
claude-3-opus-20240229      →  (no change — date suffix, not a version)
claude-3-5-sonnet           →  (no change — "5" not at end of string)
```

This means you only need to add the canonical (dotted) form to `pricing.yaml` — the hyphenated Vertex AI variants are resolved automatically.

---

## Configuring Custom Pricing

For negotiated enterprise rates, volume discounts, or custom pricing, add a `pricing:` section to `config.yaml`:

### Global Discount

Apply a percentage discount to **all** model pricing:

```yaml
pricing:
  discount_percent: 0.15  # 15% off all list prices
```

### Per-Model Overrides

Override pricing for specific models (e.g., negotiated rates):

```yaml
pricing:
  models:
    - provider: openai
      model: gpt-4o
      input_per_million: 2.00     # negotiated rate (list: $2.50)
      output_per_million: 8.00    # negotiated rate (list: $10.00)

    - provider: google
      model: gemini-2.5-pro
      input_per_million: 1.00     # volume discount
      output_per_million: 8.00
```

### Stacked Discounts

Model-level and global discounts stack multiplicatively:

```yaml
pricing:
  discount_percent: 0.10  # 10% enterprise-wide

  models:
    - provider: openai
      model: gpt-4o
      input_per_million: 2.50
      output_per_million: 10.00
      discount_percent: 0.20  # additional 20% on GPT-4o
```

Effective cost for GPT-4o: `base × 0.80 × 0.90 = base × 0.72` (28% total discount).

---

## Where Cost Is Calculated

Cost enrichment happens at **two points** in the pipeline:

### 1. Proxy Mode (real-time)

When an LLM call completes through `/proxy/*`, the proxy extracts the token count from the provider's response and calls `calc.Calculate()` inline:

```
Client → Proxy → Upstream Provider → Response
                                       ↓
                               Extract tokens → Calculate cost → Build span
```

The proxy handles provider-specific token extraction:
- **OpenAI**: `usage.prompt_tokens`, `usage.completion_tokens`
- **Google Gemini**: `usageMetadata.promptTokenCount`, `usageMetadata.candidatesTokenCount`
- **Anthropic**: `usage.input_tokens`, `usage.output_tokens`

### 2. OTel Collector (pipeline enrichment)

When spans arrive via OTLP (Agent Mode), the GenAI Processor enriches them before export:

```
Agent SDK → OTLP → Collector → GenAI Processor → Calculate cost → Export to Candela
```

The processor reads `gen_ai.usage.input_tokens` and `gen_ai.usage.output_tokens` from span attributes.

---

## Adding a New Model

When a provider releases a new model:

### Option A: Config Override (no redeploy)

Add it to your `config.yaml`:

```yaml
pricing:
  models:
    - provider: openai
      model: gpt-6
      input_per_million: 5.00
      output_per_million: 20.00
```

Restart the server. The new model will be priced correctly.

### Option B: Update Built-In Defaults (permanent)

1. Edit `pkg/costcalc/pricing.yaml` — add the new model entry
2. Run tests: `nix develop -c go test ./pkg/costcalc -v`
3. Deploy — the updated `pricing.yaml` is embedded at compile time via `//go:embed`

---

## Implementation Files

| File | Purpose |
|------|---------|
| `pkg/costcalc/pricing.yaml` | Built-in list prices for all cloud models (embedded via `//go:embed`) |
| `pkg/costcalc/calculator.go` | `Calculator`, `loadDefaults()`, `LoadFromConfig()`, `resolve()`, `normalizeModelID()`, discount math |
| `pkg/costcalc/calculator_test.go` | Unit tests for pricing, discounts, overrides, and model ID normalization |
| `pkg/billing/` | Storage-agnostic billing types (`BudgetRecord`, `GrantRecord`, `BudgetCheckResult`) and `Service` interface |
| `pkg/proxy/proxy.go` | Token extraction from provider responses |
| `collector/processors/genaiprocessor/processor.go` | OTel pipeline cost enrichment |
| `cmd/candela-server/main.go` | Wiring `cfg.Pricing` → `calc.LoadFromConfig()` |
