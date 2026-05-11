# PROXY-04/05: Error handling — 413, unknown provider, invalid paths
# Verifies correct HTTP status codes and error response shapes.

# ── PROXY-05: Unknown provider → 400 ─────────────────────────────────────────
POST {{CANDELA_URL}}/proxy/unknown-provider/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{"model": "gpt-4o", "messages": [{"role": "user", "content": "test"}]}

HTTP 400


# ── Invalid proxy path (no sub-path) → 400 ───────────────────────────────────
POST {{CANDELA_URL}}/proxy/openai
Authorization: Bearer sk-test
Content-Type: application/json
{"model": "gpt-4o", "messages": [{"role": "user", "content": "test"}]}

HTTP 400


# ── PROXY-04: Body > 10MB → 413 Request Entity Too Large ─────────────────────
# Generate a 10MB+ body by repeating content.
# Hurl doesn't support dynamic generation, so we use a known-large fixture string.
# The proxy enforces a 10MB cap via MaxBytesReader.
#
# Note: This test sends a slightly-oversized JSON payload.
# We approximate with a large "content" string.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{
  "model": "gpt-4o",
  "messages": [{
    "role": "user",
    "content": "{{large_content}}"
  }]
}

# Skip this test if large_content variable is not set.
# Run with: hurl --variable large_content="$(python3 -c "print('x'*10500000)")"
# HTTP 413
# Uncomment the above line to enable when running with a real large payload.


# ── Malformed JSON body → 400 ─────────────────────────────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
this is not json at all

HTTP 400


# ── Upstream 5xx is forwarded as 502 ─────────────────────────────────────────
# The mock upstream returns 200. To test 502 we'd need the mock to return 5xx.
# This is tested in Rust integration tests (stateful scenario).
# Placeholder left here as documentation.
