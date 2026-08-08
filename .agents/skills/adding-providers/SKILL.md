---
name: adding-providers
description: >
  Checklist and guide for adding a new LLM provider to the Candela proxy.
  Activate when adding, removing, or modifying provider support — covers
  all files that must be updated across both binaries.
---

# Adding a New Provider to Candela

Adding a provider touches **multiple files across multiple packages**. Missing
any one causes silent 404s that only manifest at deploy time. Follow this
checklist completely.

## Prerequisites

- Know the provider's Vertex AI routing pattern:
  - **MaaS (OpenAI-compat)**: Uses `VertexAIGeminiOAIPathRewriter` (deepseek, qwen, zai, meta, xai)
  - **MaaS (rawPredict)**: Uses `VertexAIMaaSPathRewriter` (mistral)
  - **Native Vertex AI**: Uses custom path rewriter (anthropic, gemini)
  - **Direct API**: No Vertex AI, direct upstream URL (openai, anthropic-direct)
- Know the default Vertex AI region (e.g., `us-central1`, `us-east5`, `global`)
- Know the model prefix for MaaS providers (e.g., `meta/`, `xai/`, `deepseek-ai/`)

## Checklist

### 1. `pkg/proxy/proxy.go` — DefaultProviders()

Add the provider to the `DefaultProviders()` function:

```go
{Name: "myprovider", UpstreamURL: "https://us-east5-aiplatform.googleapis.com"},
```

### 2. `pkg/proxy/proxy.go` — handleProxy model prefix injection

If the provider needs a publisher prefix (MaaS providers), add a case to the
model prefix injection switch in `handleProxy`:

```go
case "myprovider":
    if !strings.HasPrefix(requestModel, "myprovider/") {
        upstreamBody = rewriteModelField(upstreamBody, "myprovider/"+requestModel)
    }
```

### 3. `pkg/proxy/parsers.go` — Provider prefix stripping

If the provider uses a model prefix, add prefix stripping in `init()` so
responses show clean model names:

```go
"myprovider": func(m string) string { return strings.TrimPrefix(m, "myprovider/") },
```

### 4. `pkg/costcalc/pricing.yaml` — Model pricing

Add pricing entries for each model under the provider:

```yaml
myprovider:
  my-model-name:
    input_per_million: 0.20
    output_per_million: 0.60
```

### 5. `cmd/candela-server/provider_config.go` — Server declarative config

> [!IMPORTANT]
> This is the source of truth that the server binary uses. Tests validate
> against it. Missing this causes 404s in staging/prod.

For **MaaS providers** (OpenAI-compat via Vertex AI):
```go
// Add to maaSProviderRegion map:
var maaSProviderRegion = map[string]string{
    // ...existing...
    "myprovider": "us-east5",
}
```

For **non-Vertex providers** (direct API):
```go
// Add to nonVertexProviders map:
var nonVertexProviders = map[string]bool{
    // ...existing...
    "myprovider": true,
}
```

For **Vertex AI providers that aren't MaaS** (e.g., custom path rewriters):
Add to `vertexAIProviders` init function directly, and add configuration
logic in `cmd/candela-server/main.go` (similar to anthropic/gemini blocks).

### 6. `cmd/candela/main.go` — CLI binary buildCloudProxy()

> [!IMPORTANT]
> The CLI binary has a SEPARATE provider switch from the server binary.
> Missing this causes 404s when running `candela` locally in solo/cloud mode.

Add a case to the switch in `buildCloudProxy()`:

```go
case "myprovider":
    // MyProvider via Vertex AI OpenAI-compat endpoint.
    p.UpstreamURL = proxy.VertexAIUpstreamURL("us-east5")
    p.TokenSource = tokenSource
    p.PathRewriter = &proxy.VertexAIGeminiOAIPathRewriter{
        ProjectID: project,
        Region:    "us-east5",
    }
```

### 7. `deploy/entrypoint-server.sh` — Docker entrypoint config

> [!CAUTION]
> This was the ROOT CAUSE of staging 404s that took 5 deployment attempts
> to find. The entrypoint generates `config.yaml` with an explicit
> `providers:` list. If the provider is not listed here, it will NOT get
> routes registered regardless of what `DefaultProviders()` returns.

Add to both the `providers:` list AND `provider_overrides:` (for region):

```yaml
    provider_overrides:
      # ...existing...
      myprovider:
        region: "${CANDELA_MYPROVIDER_REGION:-us-east5}"
  providers:
    # ...existing...
    - myprovider
```

### 8. `test/functional/test_config.yaml` — Disable in functional tests

Add to `disabled_providers` so Hurl tests don't hit real endpoints:

```yaml
disabled_providers:
  # ...existing...
  - myprovider
```

### 9. Firestore model catalog — Seed models

Seed model entries into ALL Firestore databases:
- `candela` (production)
- `candela-stg` (staging)

```bash
# Use the Firestore REST API or the seed script
curl -X PATCH \
  "https://firestore.googleapis.com/v1/projects/PROJECT/databases/DATABASE/documents/model_catalog/PROVIDER_MODEL" \
  -H "Authorization: Bearer TOKEN" \
  -d '{"fields": {...}}'
```

### 10. Release & Deploy

1. Commit, push, create PR
2. After merge: tag release (`git tag vX.Y.Z && git push origin vX.Y.Z`)
3. Wait for GitHub Actions Publish to complete (~5 min)
4. Update `candela-deploy/.gitlab-ci.yml` with new version
5. Push deploy, verify staging routes in Cloud Run logs
6. Trigger manual `deploy-production` job

## Verification

- [ ] `go test ./cmd/candela-server/ -run TestAllDefaultProviders` passes
- [ ] `go test ./pkg/proxy/ -run TestMaaSProviders` passes
- [ ] Local: `candela start` → no "unknown provider" warnings in logs
- [ ] Staging: Cloud Run logs show `🔐 myprovider via Vertex AI configured`
- [ ] Staging: `POST /proxy/myprovider/v1/chat/completions` returns non-404

## Common Pitfalls

| Symptom | Cause |
|---------|-------|
| 404 in staging/prod, code looks correct | **Missing from `deploy/entrypoint-server.sh`** `providers:` list |
| 404 on `/proxy/myprovider/` in staging/prod | Missing from `provider_config.go` |
| 404 on `/proxy/myprovider/` locally | Missing from `cmd/candela/main.go` `buildCloudProxy()` |
| "unknown provider" warning in CLI logs | Missing from CLI switch |
| Test passes but deploy still 404s | Test map updated but not `provider_config.go` (old bug, now fixed) |
| Model returns wrong name | Missing prefix stripping in `parsers.go` |
| GHCR image missing new tag | Publish workflow cancelled by concurrency group |
