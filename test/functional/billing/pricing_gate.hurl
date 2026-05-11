# BILLING-03/04: Pricing gate — unknown cloud model blocked, "local" provider exempt
#
# These tests require the binary to be running in team mode with a UserStore.
# In dev/local mode, pricing gates are skipped.

# ── BILLING-04: "local" provider bypasses pricing gate entirely ───────────────
# The "local" provider is for Ollama/LM Studio and is intentionally free ($0).
# It MUST NOT be blocked by the pricing gate regardless of model name.
POST {{CANDELA_URL}}/proxy/local/v1/chat/completions
Authorization: Bearer tok
Content-Type: application/json
{"model":"llama3.2:latest","messages":[{"role":"user","content":"hi"}]}

# local provider: must NOT return 402 (pricing gate exempt).
HTTP *
[Asserts]
status != 402


# ── Error shape documentation: pricing_not_configured ─────────────────────────
# When a cloud model has no pricing entry, the proxy returns:
#
# HTTP 402
# {
#   "error": {
#     "message": "no pricing configured for model mystery-model-9999 — contact your admin",
#     "type": "pricing_not_configured",
#     "code": 402
#   }
# }
#
# Tested in Rust integration tests (proxy/billing_test.go TestPricingGate_BlocksUnknownCloudModel).
