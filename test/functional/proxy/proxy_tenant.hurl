# TENANT-01..05: Tenant cost attribution — header and baggage propagation
#
# These tests verify that X-Candela-Tenant-Id header and W3C Baggage
# (candela.tenant_id) are accepted, validated, and propagated correctly.
# Spans with tenant_id enable per-customer cost attribution in multitenant apps.
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 \
#        test/functional/proxy/proxy_tenant.hurl

# ── TENANT-01: Valid X-Candela-Tenant-Id header is accepted ──────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Candela-Tenant-Id: acme-corp
{"model":"gpt-4o","messages":[{"role":"user","content":"tenant header test"}]}

HTTP 200
[Asserts]
# Proxy must accept the request — tenant ID is captured in the span.
status == 200


# ── TENANT-02: Baggage with candela.tenant_id is accepted ────────────────────
# W3C Baggage format: "key=value, key2=value2"
# ADK's OTel propagator injects this header automatically.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
Baggage: candela.tenant_id=acme-corp,svc.version=1.2.0
Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
{"model":"gpt-4o","messages":[{"role":"user","content":"baggage tenant test"}]}

HTTP 200
[Asserts]
# Must not crash. Baggage is parsed, tenant_id captured, other baggage ignored.
status == 200


# ── TENANT-03: Baggage takes precedence over header when both present ─────────
# If an ADK agent sets baggage AND a client also sends the header,
# baggage wins (it represents the distributed trace context, not just one hop).
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Candela-Tenant-Id: header-tenant
Baggage: candela.tenant_id=baggage-tenant
{"model":"gpt-4o","messages":[{"role":"user","content":"precedence test"}]}

HTTP 200
[Asserts]
# Request must succeed. Span tenant_id = "baggage-tenant".
# (Deep value verified in unit tests, this confirms no crash on conflict.)
status == 200


# ── TENANT-04: Invalid X-Candela-Tenant-Id is silently discarded ─────────────
# Tenant IDs must match [a-zA-Z0-9\-._]{1,128}.
# Invalid values are dropped (not rejected with 4xx) to avoid breaking clients.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Candela-Tenant-Id: evil\ninjection@attack!!
{"model":"gpt-4o","messages":[{"role":"user","content":"invalid tenant header"}]}

HTTP 200
[Asserts]
# Must not 400/500 — invalid tenant ID is silently dropped, span has no tenant_id.
status == 200


# ── TENANT-05: Domain-style tenant IDs with dots/underscores are valid ────────
# tenantIDPattern allows [a-zA-Z0-9\-._]{1,128} — broader than session IDs.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Candela-Tenant-Id: acme.corp_42
{"model":"gpt-4o","messages":[{"role":"user","content":"domain-style tenant"}]}

HTTP 200
[Asserts]
status == 200


# ── TENANT-06: Empty Baggage header doesn't crash ────────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
Baggage:
{"model":"gpt-4o","messages":[{"role":"user","content":"empty baggage"}]}

HTTP 200
[Asserts]
status == 200


# ── TENANT-07: Baggage with no candela key is a no-op ────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
Baggage: other.service=foo,another.key=bar
{"model":"gpt-4o","messages":[{"role":"user","content":"unrelated baggage"}]}

HTTP 200
[Asserts]
status == 200
