# DASH-10..15: Dashboard ConnectRPC contract tests
# These validate the JSON serialization contract between the desktop client
# and the ConnectRPC backend — specifically the proto3 JSON mapping for
# google.protobuf.Timestamp fields in TimeRange.
#
# IMPORTANT: These tests require a valid auth token passed via the
# AUTH_TOKEN variable (e.g. from `gcloud auth print-access-token`).

# ── DASH-10: GetModelBreakdown with valid TimeRange → 200 ──────────────────
# Proto: time_range is candela.types.TimeRange with google.protobuf.Timestamp
# The JSON mapping uses RFC 3339 strings, NOT protobuf enum names.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-12-31T23:59:59Z"
  }
}

HTTP 200
[Asserts]
# Response is valid JSON (may be empty {} if no data, which is fine).
header "Content-Type" contains "application/json"


# ── DASH-11: GetModelBreakdown with string enum time_range → 400 ────────────
# This is exactly the bug the desktop would hit if it sent "24h" instead
# of the proper TimeRange message. Regression guard.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": "24h"
}

HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"


# ── DASH-12: GetUsageSummary with empty body → 200 (defaults to 24h) ───────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetUsageSummary
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── DASH-13: GetMyUsage returns budget fields when available ───────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetMyUsage
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-12-31T23:59:59Z"
  }
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── DASH-14: Missing Connect-Protocol-Version header still works ───────────
# ConnectRPC should accept requests without the version header.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
{}

HTTP 200


# ── DASH-15: Non-existent RPC method → 404 ─────────────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/DoesNotExist
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 404
