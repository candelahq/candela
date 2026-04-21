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

See the current pricing table in [`pkg/costcalc/calculator.go` → `loadDefaults()`](../pkg/costcalc/calculator.go) for exact rates.

**Coverage:**
- **Google**: Gemini 3.1, 2.5 (Pro/Flash/Flash-Lite), 2.0, 1.5
- **OpenAI**: GPT-5.4 family, GPT-4o, reasoning models (o1/o3)
- **Anthropic**: Claude 4.7/4.6/4.5, Claude 4 (Vertex AI IDs), Claude 3.5 legacy
- **Local**: Always $0.00 — runs on your hardware

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

1. Edit `pkg/costcalc/calculator.go` → `loadDefaults()`
2. Add the new model with its list pricing
3. Run tests: `nix develop -c go test ./pkg/costcalc -v`
4. Deploy

---

## Implementation Files

| File | Purpose |
|------|---------|
| `pkg/costcalc/calculator.go` | `Calculator`, `loadDefaults()`, `LoadFromConfig()`, `resolve()`, discount math |
| `pkg/costcalc/calculator_test.go` | Unit tests for pricing, discounts, and overrides |
| `pkg/proxy/proxy.go` | Token extraction from provider responses |
| `collector/processors/genaiprocessor/processor.go` | OTel pipeline cost enrichment |
| `cmd/candela-server/main.go` | Wiring `cfg.Pricing` → `calc.LoadFromConfig()` |
