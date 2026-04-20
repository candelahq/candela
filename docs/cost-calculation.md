# 💰 Cost Calculation Engine

Candela provides real-time cost tracking for every LLM call. The `pkg/costcalc` package maintains a pricing table and calculates USD costs from token usage.

## How Costs Flow Through the System

```
1. LLM Call (via Proxy or OTel)
         │
2. Token extraction (from response body or span attributes)
         │
3. Cost calculation (costcalc.Calculate)
         │
4. Enrichment ──┬── Proxy: writes cost into span.GenAI.CostUSD
                └── Collector: writes cost as gen_ai.usage.cost_usd attribute
         │
5. Storage (DuckDB/SQLite/BigQuery gen_ai_cost_usd column)
         │
6. Display ──┬── Dashboard: total cost, cost-over-time chart
             ├── Trace detail: per-span cost breakdown
             └── Budget enforcement: deduct from user budget
```

### Two Enrichment Points

Cost is calculated in **two places**, depending on the ingestion path:

| Path | Where | When |
|------|-------|------|
| **Proxy Mode** | `pkg/proxy/proxy.go` → `buildSpan()` | After upstream response is parsed |
| **Collector Mode** | `collector/processors/genaiprocessor` | In the OTel pipeline before export |
| **Processor (fallback)** | `pkg/processor/processor.go` → `flush()` | During batch flush, if cost is still $0 |

The processor applies cost calculation as a **safety net** — if the proxy or collector already set the cost, the processor does not override it.

---

## Pricing Table

The calculator ships with built-in pricing for common models:

### Google

| Model | Input ($/1M tokens) | Output ($/1M tokens) |
|-------|--------------------:|---------------------:|
| `gemini-2.0-flash` | $0.10 | $0.40 |
| `gemini-2.0-pro` | $1.25 | $10.00 |
| `gemini-1.5-flash` | $0.075 | $0.30 |
| `gemini-1.5-pro` | $1.25 | $5.00 |

### OpenAI

| Model | Input ($/1M tokens) | Output ($/1M tokens) |
|-------|--------------------:|---------------------:|
| `gpt-4o` | $2.50 | $10.00 |
| `gpt-4o-mini` | $0.15 | $0.60 |
| `gpt-4-turbo` | $10.00 | $30.00 |
| `gpt-3.5-turbo` | $0.50 | $1.50 |
| `o1` | $15.00 | $60.00 |
| `o1-mini` | $3.00 | $12.00 |

### Anthropic (via Vertex AI)

| Model | Input ($/1M tokens) | Output ($/1M tokens) |
|-------|--------------------:|---------------------:|
| `claude-sonnet-4-20250514` | $3.00 | $15.00 |
| `claude-haiku-3-5-20241022` | $0.80 | $4.00 |
| `claude-3-5-sonnet-20241022` | $3.00 | $15.00 |
| `claude-3-opus-20240229` | $15.00 | $75.00 |

### Local Models

All local models (provider = `local`) return **$0.00** — they run on your hardware with no API cost.

---

## Cost Formula

```
cost = (input_tokens / 1,000,000 × input_per_million)
     + (output_tokens / 1,000,000 × output_per_million)
```

Example: `gpt-4o` with 1,500 input tokens and 500 output tokens:
```
cost = (1500 / 1M × $2.50) + (500 / 1M × $10.00)
     = $0.00375 + $0.005
     = $0.00875
```

---

## Model Matching

The calculator uses a two-pass lookup strategy:

1. **Exact match** by `provider/model` key (e.g., `openai/gpt-4o`)
2. **Provider-agnostic fallback** — if the exact key misses, search for any entry ending with `/model`

This means `anthropic/claude-sonnet-4-20250514` will match even if the request comes through the `gemini-oai` provider route, as long as the model name matches.

If no match is found, the cost is **$0.00** — the span is still captured, just without cost data.

---

## Adding or Updating Pricing

### At Build Time

Edit `pkg/costcalc/calculator.go` → `loadDefaults()`:

```go
func (c *Calculator) loadDefaults() {
    defaults := []ModelPricing{
        // Add new models here:
        {Provider: "google", Model: "gemini-2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
        {Provider: "google", Model: "gemini-2.5-flash", InputPerMillion: 0.15, OutputPerMillion: 0.60},
        {Provider: "openai", Model: "o3-mini", InputPerMillion: 1.10, OutputPerMillion: 4.40},
        // ...
    }
}
```

### At Runtime

The `Calculator` is thread-safe and supports runtime pricing updates:

```go
calc := costcalc.New()

// Add pricing for a new model
calc.SetPricing(costcalc.ModelPricing{
    Provider:         "anthropic",
    Model:            "claude-opus-4-20250514",
    InputPerMillion:  15.00,
    OutputPerMillion: 75.00,
})
```

> [!TIP]
> A future enhancement could load pricing from a configuration file or remote endpoint, enabling pricing updates without code changes.

---

## Accuracy Considerations

| Factor | Impact | Notes |
|--------|--------|-------|
| **Stale pricing** | Medium | Pricing tables must be manually updated when providers change rates |
| **Token counting** | Low | Token counts come from the provider's response — highly accurate |
| **Streaming responses** | Low | Tokens are extracted from the final SSE `usage` chunk |
| **Vertex AI pricing** | Medium | Vertex AI may have different pricing than direct API; current table uses direct API rates |
| **Cached tokens** | Not tracked | Some providers offer cached token discounts; not currently differentiated |
| **Prompt caching** | Not tracked | Anthropic/Google prompt caching discounts not yet modeled |

---

## Implementation Files

| File | Purpose |
|------|---------|
| `pkg/costcalc/calculator.go` | `Calculator` struct, pricing table, `Calculate()` method |
| `pkg/costcalc/calculator_test.go` | Unit tests for cost calculation |
| `pkg/proxy/proxy.go` | Calls `calc.Calculate()` in `buildSpan()` |
| `pkg/processor/processor.go` | Fallback cost enrichment during batch flush |
| `collector/processors/genaiprocessor/processor.go` | OTel pipeline cost enrichment |
