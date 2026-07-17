#!/usr/bin/env bash
set -euo pipefail

RESULTS_FILE="/Users/ab/code/candelahq/candela/.gemini/issue_results.txt"
> "$RESULTS_FILE"

create_issue() {
  local title="$1"
  local labels="$2"
  local body="$3"
  
  echo "Creating: $title"
  url=$(gh issue create --title "$title" --label "$labels" --body "$body" 2>&1)
  echo "$url" | tail -1
  echo "$url" | tail -1 >> "$RESULTS_FILE"
  sleep 1
}

# ============================================================
# WEEK 1 CRITICAL FIXES
# ============================================================

create_issue \
  "[P0] DuckDB SetMaxOpenConns(1) — concurrent writers corrupt database" \
  "bug,priority:p0,backend,data-integrity" \
  "## Description

DuckDB does not support concurrent writers. The current configuration allows multiple goroutines to write simultaneously, causing silent data corruption.

## Flagged By
- Principal Go Engineer

## Estimated Effort
1 line change

## File Paths
- Storage layer DuckDB connection initialization

## Acceptance Criteria
- [ ] \`SetMaxOpenConns(1)\` is set on the DuckDB connection pool
- [ ] Concurrent write tests pass without corruption
- [ ] Verified no performance regression on read path

## Context
Data corruption is happening silently in production. This is the highest-priority fix."

create_issue \
  "[P0] Fix TotalCount pagination — returns page size instead of true count" \
  "bug,priority:p0,backend,data-integrity" \
  "## Description

\`TotalCount\` in both DuckDB and SQLite storage backends returns \`len(traces)\` (the page size) instead of the actual total count. \`ListProjects\` also ignores \`PageToken\`/offset entirely. Every list view in the app lies about total results.

## Flagged By
- Principal Go Engineer (CRIT-2, HIGH-8)
- Principal TS Engineer
- CPO

## Estimated Effort
30 minutes

## File Paths
- DuckDB storage: query traces / list projects
- SQLite storage: query traces / list projects

## Acceptance Criteria
- [ ] \`TotalCount\` returns actual \`COUNT(*)\` from database, not \`len(results)\`
- [ ] \`ListProjects\` correctly applies offset/page token
- [ ] Both DuckDB and SQLite backends are fixed
- [ ] Frontend displays correct total counts

## Context
Blocks Week 2 pagination hackathon project. Every list view is broken for datasets > 1 page."

create_issue \
  "[P0] Fix stream capture billing data loss on large responses" \
  "bug,priority:p0,backend,data-integrity" \
  "## Description

The proxy's stream capture has a 10MB truncation limit. When LLM responses exceed this, token/usage data in the final streaming chunk is lost, causing revenue leakage. The \$0.001 spend reservation also allows massive budget overdraft.

## Flagged By
- Principal Go Engineer

## Estimated Effort
2 hours

## File Paths
- Proxy stream capture handler
- Budget reservation logic

## Acceptance Criteria
- [ ] Token/usage data from final streaming chunks is always captured regardless of response size
- [ ] Spend reservation uses a reasonable minimum (not \$0.001)
- [ ] Billing reconciliation handles truncated streams gracefully
- [ ] Test with >10MB streaming responses confirms no data loss

## Context
Revenue leakage on large responses. Financial accuracy is a core value proposition."

create_issue \
  "[P0] Wrap DuckDB trace ingest in transaction — non-atomic writes" \
  "bug,priority:p0,backend,data-integrity" \
  "## Description

DuckDB trace ingestion writes spans and the outbox record non-atomically. If the process crashes between the two writes, spans are stored but never synced to BigQuery, causing silent data loss.

## Flagged By
- Principal Go Engineer

## Estimated Effort
1 hour

## File Paths
- DuckDB storage ingest path
- Outbox write logic

## Acceptance Criteria
- [ ] Spans and outbox record are written in a single database transaction
- [ ] Crash between writes cannot leave orphaned spans
- [ ] Transaction rollback on partial failure
- [ ] Integration test verifying atomicity

## Context
Spans stored but never synced to BigQuery means customers lose visibility into their AI spend."

create_issue \
  "[P1] Add cache token columns to DuckDB/SQLite SELECT statements" \
  "bug,priority:p1,backend" \
  "## Description

Cache token columns are ingested into DuckDB/SQLite but never included in SELECT statements, so the Cache Savings feature shows \$0 for all local/self-hosted users.

## Flagged By
- Principal Go Engineer

## Estimated Effort
15 minutes

## File Paths
- DuckDB storage: trace query SELECT
- SQLite storage: trace query SELECT

## Acceptance Criteria
- [ ] Cache token columns (prompt_cache_hit_tokens, prompt_cache_miss_tokens, etc.) included in SELECT
- [ ] Cache Savings dashboard shows correct values for DuckDB/SQLite users
- [ ] No regression on BigQuery backend

## Context
Part of broader DuckDB/SQLite schema drift issue (Theme 4). Quick fix that unblocks cache savings visibility."

create_issue \
  "[P1] Authorization: Add admin check to ListAPIKeys" \
  "bug,priority:p1,backend,security" \
  "## Description

\`ListAPIKeys\` is missing an admin check — any authenticated user can enumerate all API keys in the project.

## Flagged By
- Principal Go Engineer
- Principal Security Engineer

## Estimated Effort
15 minutes

## File Paths
- ListAPIKeys handler

## Acceptance Criteria
- [ ] \`ListAPIKeys\` requires admin role
- [ ] Non-admin users get \`PermissionDenied\` error
- [ ] Test coverage for authorization check

## Context
Read endpoints and auxiliary paths are consistently less guarded than mutation endpoints (Theme 5: Authorization Gaps at API Boundaries)."

create_issue \
  "[P1] Add user scoping to DuckDB/SQLite GetTrace — cross-user data leakage" \
  "bug,priority:p1,backend,security" \
  "## Description

DuckDB/SQLite \`GetTrace\` has no user scoping — any authenticated user can read any trace by ID, causing cross-user data leakage. BigQuery backend already has this filter.

## Flagged By
- Principal Go Engineer
- Principal Security Engineer

## Estimated Effort
15 minutes

## File Paths
- DuckDB storage: GetTrace query
- SQLite storage: GetTrace query

## Acceptance Criteria
- [ ] \`GetTrace\` in DuckDB/SQLite filters by user ID (matching BigQuery behavior)
- [ ] Non-matching user gets \`NotFound\` response
- [ ] Test coverage for cross-user access attempt

## Context
Authorization gap at API boundary. Data leakage vulnerability."

create_issue \
  "[P1] Pin all CI release actions to SHA + persist-credentials: false" \
  "bug,priority:p1,supply-chain,security,ci" \
  "## Description

6/10 CI workflows use unpinned actions — especially release/publish workflows that handle secrets and push production artifacts. GoReleaser installed via \`@latest\`. The main \`ci.yml\` is well-pinned, but release workflows are not.

## Flagged By
- Principal Security Engineer (5 critical + 13 high findings)
- Principal Systems Engineer

## Estimated Effort
1 hour

## File Paths
- \`.github/workflows/release*.yml\`
- \`.github/workflows/publish*.yml\`
- GoReleaser installation step

## Acceptance Criteria
- [ ] All GitHub Actions in release/publish workflows pinned to full SHA
- [ ] \`persist-credentials: false\` set on all checkout steps
- [ ] GoReleaser pinned to specific version (not \`@latest\`)
- [ ] No mutable tags (\`:latest\`, \`@v3\`) in any workflow

## Context
The most frequently run workflow (ci.yml) is well-pinned. All release workflows — the ones that handle secrets — are not. This is the highest-risk gap in CI/CD."

create_issue \
  "[P1] Pin Nix installer with SHA256 in Dockerfile.ci" \
  "bug,priority:p1,supply-chain,security,ci" \
  "## Description

Nix installer is piped to shell without checksum verification in \`Dockerfile.ci\`. A compromised installer would backdoor all CI runs.

## Flagged By
- Principal Security Engineer
- Principal Systems Engineer

## Estimated Effort
15 minutes

## File Paths
- \`Dockerfile.ci\`

## Acceptance Criteria
- [ ] Nix installer downloaded with SHA256 verification
- [ ] Installer fetched from pinned URL (not \`latest\`)
- [ ] Checksum mismatch fails the build

## Context
Supply chain security. Pipe-to-shell without verification is a critical anti-pattern."

create_issue \
  "[P1] Cloud Run allUsers IAM bypass — direct URL circumvents IAP" \
  "bug,priority:p1,security,infrastructure" \
  "## Description

The Cloud Run \`allUsers\` IAM binding makes the \`*.run.app\` URL publicly accessible, completely bypassing IAP. The entire auth layer can be circumvented via the direct Cloud Run URL.

## Flagged By
- Principal Security Engineer

## Estimated Effort
15 minutes

## File Paths
- Terraform Cloud Run IAM configuration

## Acceptance Criteria
- [ ] Cloud Run \`allUsers\` IAM binding is conditional on IAP being enabled
- [ ] Direct \`*.run.app\` URLs return 403 when IAP is configured
- [ ] Test with direct URL confirms IAP enforcement

## Context
Authorization boundary issue (Theme 5). This effectively makes IAP optional."

create_issue \
  "[P1] Add email_verified check to Firebase token validation" \
  "bug,priority:p1,security,backend" \
  "## Description

Firebase token validation doesn't check the \`email_verified\` claim, allowing users with unverified emails to authenticate. This risks email impersonation attacks.

## Flagged By
- Principal Security Engineer

## Estimated Effort
10 minutes

## File Paths
- Firebase auth middleware / token validation

## Acceptance Criteria
- [ ] Firebase token validation rejects tokens where \`email_verified\` is false
- [ ] Appropriate error message returned to unverified users
- [ ] Test coverage for unverified email scenario

## Context
Defense-in-depth. Unverified email can lead to impersonation."

create_issue \
  "[P1] Add AbortController to useTraces — stale data on rapid filter changes" \
  "bug,priority:p1,frontend" \
  "## Description

\`useTraces\` has zero request cancellation. When users rapidly change filters, previous requests can resolve after newer ones, displaying stale/incorrect data. The debounce cleanup on unmount is also missing.

## Flagged By
- Principal TS Engineer

## Estimated Effort
1 hour

## File Paths
- \`useTraces\` hook
- Related filter/fetch hooks

## Acceptance Criteria
- [ ] \`AbortController\` wired to all fetch calls in \`useTraces\`
- [ ] Previous requests cancelled when new filter values arrive
- [ ] Debounce timer cleaned up on component unmount
- [ ] No stale data displayed after rapid filter toggles
- [ ] Manual testing with network throttling confirms fix

## Context
Part of Theme 6 (Frontend SPA issues). Blocks Week 2 frontend hardening project."

create_issue \
  "[P1] Add root error.tsx and not-found.tsx error boundaries" \
  "bug,priority:p1,frontend" \
  "## Description

The Next.js app has zero \`error.tsx\` or \`not-found.tsx\` files. Unhandled throws show raw React error screens in production.

## Flagged By
- Principal TS Engineer

## Estimated Effort
30 minutes

## File Paths
- \`app/error.tsx\` (create)
- \`app/not-found.tsx\` (create)
- \`app/loading.tsx\` (create)

## Acceptance Criteria
- [ ] Root \`error.tsx\` catches unhandled errors with user-friendly UI and retry button
- [ ] Root \`not-found.tsx\` shows branded 404 page
- [ ] Root \`loading.tsx\` shows appropriate loading state
- [ ] Error boundary logs errors to console/monitoring
- [ ] No raw React errors visible in production

## Context
Basic Next.js best practice. Blocks Week 2 frontend error boundaries project."

create_issue \
  "[P2] Clamp negative tokens in Calculate()" \
  "bug,priority:p2,backend" \
  "## Description

Negative token counts in \`Calculate()\` credit user budgets instead of debiting them. Should clamp to zero.

## Flagged By
- Principal Go Engineer

## Estimated Effort
5 minutes

## File Paths
- Cost calculation: \`Calculate()\` function

## Acceptance Criteria
- [ ] \`Calculate()\` clamps negative token values to 0
- [ ] Test for negative token edge case

## Context
Budget integrity issue. Negative tokens could be exploited to inflate budget remaining."

create_issue \
  "[P2] Replace os.Exit(1) with graceful shutdown in ListenAndServe goroutine" \
  "bug,priority:p2,backend" \
  "## Description

Calling \`os.Exit(1)\` in the \`ListenAndServe\` goroutine skips all deferred cleanup (connection closing, buffer flush, etc.). Should call the server's \`stop()\` function instead.

## Flagged By
- Principal Go Engineer
- Principal Systems Engineer

## Estimated Effort
10 minutes

## File Paths
- Server startup: \`ListenAndServe\` goroutine

## Acceptance Criteria
- [ ] \`os.Exit(1)\` replaced with \`stop()\` or graceful shutdown signal
- [ ] Deferred cleanup functions execute on server error
- [ ] Verified all resources are released on shutdown

## Context
Surgical fix. Prevents resource leaks on server errors."

create_issue \
  "[P2] Increase LB backend timeout 30s → 300s for LLM streaming" \
  "bug,priority:p2,infrastructure" \
  "## Description

Load balancer backend timeout is 30s, but LLM streaming responses routinely exceed this, causing premature connection drops.

## Flagged By
- Principal Systems Engineer

## Estimated Effort
5 minutes

## File Paths
- Terraform/LB configuration: backend timeout

## Acceptance Criteria
- [ ] LB backend timeout set to 300s (or configurable)
- [ ] Long-running LLM streams complete without timeout
- [ ] Health check timeout adjusted accordingly

## Context
Production reliability issue for any model call >30s."

create_issue \
  "[P2] Fix docker-compose Dockerfile reference — Dockerfile.server doesn't exist" \
  "bug,priority:p2,infrastructure" \
  "## Description

Docker Compose references \`Dockerfile.server\` which doesn't exist. Docker Compose is broken out of the box for new contributors.

## Flagged By
- Principal Systems Engineer

## Estimated Effort
5 minutes

## File Paths
- \`docker-compose.yml\`

## Acceptance Criteria
- [ ] \`docker-compose up\` works without modification
- [ ] Correct Dockerfile reference
- [ ] Verified with clean clone

## Context
Developer experience. New contributors can't run the project locally via Docker."

create_issue \
  "[P2] Wire Prometheus /metrics endpoint — metrics computed but never exposed" \
  "bug,priority:p2,backend,observability" \
  "## Description

Prometheus metrics are computed internally but the \`/metrics\` HTTP endpoint is never wired up, so Prometheus can't scrape them.

## Flagged By
- Principal Systems Engineer

## Estimated Effort
30 minutes

## File Paths
- Go server: HTTP mux / Prometheus handler registration

## Acceptance Criteria
- [ ] \`/metrics\` endpoint returns Prometheus-format metrics
- [ ] All existing metric collectors are registered
- [ ] Health check endpoint includes all backends (add Firestore)
- [ ] Grafana dashboard template (optional)

## Context
Observability gap. Metrics are being tracked but can't be consumed."

# ============================================================
# WEEK 2 HACKATHON PROJECTS
# ============================================================

create_issue \
  "[Hackathon] Proxy Retry with Provider Fallback ## Hackathon Sprint" \
  "enhancement,hackathon,backend,reliability" \
  "## Description

When a provider returns 500 or 429, automatically retry with a configurable fallback chain (e.g., OpenAI → Anthropic → Google). Transforms the proxy from 'transparent passthrough' to 'reliability layer.'

Without this, Candela is a single point of failure. No serious team will adopt a proxy that makes their LLM calls *less* reliable.

## Flagged By
- CPO (existential, #1 roadmap item)
- Principal Go Engineer (architecture ready — circuit breakers and provider abstraction exist)

## Estimated Effort
2–3 days (M)

## Impact
**10/10** — Existential feature. Without retry/fallback, Candela adds latency and a failure point with no compensating reliability benefit.

## File Paths
- Proxy handler
- Provider configuration / registry
- Circuit breaker infrastructure

## Acceptance Criteria
- [ ] Configurable fallback chain per-project (e.g., primary: OpenAI, fallback: Anthropic)
- [ ] Automatic retry on 500/502/503/429 with configurable backoff
- [ ] Provider health tracking (circuit breaker integration)
- [ ] Fallback triggers when primary circuit is open
- [ ] Request/response format translation between providers
- [ ] Metrics: fallback trigger count, success rate after fallback
- [ ] Dashboard visibility into fallback events
- [ ] Documentation for configuration

## Dependencies
None from Week 1"

create_issue \
  "[Hackathon] Slack Budget Alerts ## Hackathon Sprint" \
  "enhancement,hackathon,backend" \
  "## Description

When a developer hits 80%/90%/100% of their daily budget, send a Slack notification:
> ⚠️ Dev X has used 85% of their daily AI budget.

Makes Candela visible in daily workflow without requiring anyone to open the dashboard.

## Flagged By
- CPO (#1 hackathon pick)
- Principal Go Engineer (notification infra partially exists in \`notify/notifier.go\`)

## Estimated Effort
1–2 days (S)

## Impact
**9/10** — Transforms budgets from invisible backend enforcement to a visible, actionable workflow tool.

## File Paths
- \`notify/notifier.go\` (existing notification infrastructure)
- Budget enforcement / threshold detection
- New: Slack webhook integration

## Acceptance Criteria
- [ ] Slack webhook URL configurable per-project
- [ ] Notifications at configurable thresholds (default: 80%, 90%, 100%)
- [ ] Notification includes: user name, current spend, budget limit, percentage
- [ ] Rate limiting to prevent notification spam
- [ ] Slack message formatting with rich blocks
- [ ] Configuration UI in project settings
- [ ] Test webhook endpoint for setup validation

## Dependencies
None"

create_issue \
  "[Hackathon] candela watch — Live Trace Tailing CLI ## Hackathon Sprint" \
  "enhancement,hackathon,backend" \
  "## Description

\`tail -f\` for AI spend. Streams traces in real-time with color-coded costs in the terminal:

\`\`\`
\$ candela watch
14:32:05 │ gpt-4o   │ user:alice │ 1,247 tok │ \$0.03 │ 1.2s │ ✓
14:32:08 │ claude-4 │ user:bob   │ 12,453 tok │ \$0.22 │ 3.4s │ ✓
\`\`\`

High delight, incredible for demos. The OTel pipeline already exists.

## Flagged By
- CPO (#2 hackathon pick)

## Estimated Effort
1 day (S)

## Impact
**8/10** — Developer delight and demo-ability.

## File Paths
- CLI: Cobra command structure
- Backend: SSE or gRPC stream endpoint
- OTel pipeline (existing)

## Acceptance Criteria
- [ ] \`candela watch\` command streams traces in real-time
- [ ] Color-coded output: model name, user, tokens, cost, latency, status
- [ ] Filter flags: \`--user\`, \`--model\`, \`--min-cost\`
- [ ] Graceful connection handling (reconnect on drop)
- [ ] \`--json\` flag for machine-readable output
- [ ] Works with both local and remote Candela instances

## Dependencies
None"

create_issue \
  "[Hackathon] Full-Stack Cursor Pagination ## Hackathon Sprint" \
  "enhancement,hackathon,backend,frontend" \
  "## Description

Implement cursor-based pagination for all list endpoints (traces, projects, spans, leaderboards). Add \`COUNT(*)\` queries for true totals. Add pagination UI components in the frontend.

## Flagged By
- Principal Go Engineer (CRIT-2, HIGH-8)
- Principal TS Engineer (m10)
- CPO

## Estimated Effort
2 days (M)

## Impact
**8/10** — Foundational infrastructure. Every list view is broken without this.

## File Paths
- All storage backends: DuckDB, SQLite, BigQuery list queries
- API layer: pagination request/response types
- Frontend: pagination UI components
- \`useTraces\`, \`useProjects\`, and other list hooks

## Acceptance Criteria
- [ ] Cursor-based pagination on all list endpoints
- [ ] \`COUNT(*)\` queries return true totals
- [ ] Consistent pagination contract across DuckDB, SQLite, BigQuery
- [ ] Frontend pagination component (next/prev/page size)
- [ ] URL-synced pagination state
- [ ] Loading states during page transitions
- [ ] Test coverage for edge cases (empty pages, last page)

## Dependencies
- Week 1: TotalCount fix must land first"

create_issue \
  "[Hackathon] DuckDB/SQLite Feature Parity Sprint ## Hackathon Sprint" \
  "enhancement,hackathon,backend,data-integrity" \
  "## Description

Close the schema drift gap between BigQuery and DuckDB/SQLite backends. Add all missing columns, filters, and scoping to make local deployments first-class citizens.

Missing items:
- Cache token columns in SELECT
- Labels, ReasoningContent, pricing fields in schema
- Tenant filter in \`GetUsageSummary\`
- User scoping in all query paths

## Flagged By
- Principal Go Engineer (CRIT-5, MED-11, MED-22, HIGH-13)

## Estimated Effort
1–2 days (S-M)

## Impact
**8/10** — Local/self-hosted users have a materially inferior product without knowing it.

## File Paths
- DuckDB storage: schema, queries, migrations
- SQLite storage: schema, queries, migrations
- BigQuery storage (reference implementation)

## Acceptance Criteria
- [ ] DuckDB and SQLite schemas match BigQuery for all user-facing fields
- [ ] All SELECT queries include Labels, ReasoningContent, pricing fields
- [ ] \`GetUsageSummary\` includes tenant filter
- [ ] User scoping applied consistently across all query paths
- [ ] Feature parity test: run same test suite against all three backends
- [ ] Migration script for existing databases

## Dependencies
- Week 1: cache token columns and user scoping fixes"

create_issue \
  "[Hackathon] Data Export — CSV/JSON Download ## Hackathon Sprint" \
  "enhancement,hackathon,frontend" \
  "## Description

Add 'Download CSV' and 'Download JSON' buttons to traces, costs, and usage pages. Enterprise procurement asks 'can we export our data?' — the answer must be yes.

## Flagged By
- CPO (enterprise blocker)
- Principal TS Engineer

## Estimated Effort
1 day (S)

## Impact
**7/10** — Compliance blocker (GDPR, SOC2). Data already in hooks.

## File Paths
- Traces page component
- Costs/usage page components
- New: export utility functions

## Acceptance Criteria
- [ ] 'Download CSV' button on traces, costs, and usage pages
- [ ] 'Download JSON' button on same pages
- [ ] Client-side export for small datasets (<1000 rows)
- [ ] Server-side streaming export for large datasets
- [ ] Date range filter applied to exports
- [ ] Export respects user data access permissions

## Dependencies
None"

create_issue \
  "[Hackathon] README + Docs Governance Positioning Pivot ## Hackathon Sprint" \
  "enhancement,hackathon,documentation" \
  "## Description

Align all public-facing documentation from 'LLM Observability' to 'AI Spend Governance.' Candela should own the governance category where budget enforcement, grants, eBPF enforcement, and model access control give it an unassailable lead.

## Flagged By
- CPO (existential positioning fix)

## Estimated Effort
0.5 days (XS)

## Impact
**8/10** — 2 hours of work with outsized impact on first impressions.

## File Paths
- \`README.md\`
- Documentation site content

## Acceptance Criteria
- [ ] README rewritten with governance-first messaging
- [ ] New tagline: 'See everything. Enforce anything.' (or similar)
- [ ] 5 dashboard screenshots added to README
- [ ] 'Why Candela?' competitive comparison section
- [ ] Feature matrix: governance differentiators vs. observability tools

## Dependencies
None"

create_issue \
  "[Hackathon] Frontend Error Boundaries + Hook Hardening ## Hackathon Sprint" \
  "enhancement,hackathon,frontend" \
  "## Description

Comprehensive frontend resilience sprint:

1. **Standardize AbortController** across all 12 data-fetching hooks
2. **Error/loading boundaries** at route segment level
3. **Fix stale closure** in \`useTraces.updateFilters\`
4. **Fix \`useCurrentUser\` null user hang**
5. **Extract \`timeRangeToMs\`** to shared utils (currently duplicated)

## Flagged By
- Principal TS Engineer (C0-C6, M1-M5)

## Estimated Effort
2 days (M)

## Impact
**7/10** — Prevents stale data, crash-to-blank-screen, and silent hangs.

## File Paths
- All hooks in \`hooks/\` directory
- \`app/\` route segments
- \`useTraces\` hook
- \`useCurrentUser\` hook
- New: \`utils/timeRange.ts\`

## Acceptance Criteria
- [ ] All 12 data-fetching hooks use \`AbortController\` with cleanup
- [ ] \`error.tsx\` and \`loading.tsx\` at every route segment
- [ ] \`useTraces.updateFilters\` uses functional state update
- [ ] \`useCurrentUser\` handles null user without hanging
- [ ] \`timeRangeToMs\` extracted to shared utils
- [ ] No \`alert()\` calls remain in production code

## Dependencies
- Week 1: AbortController and error.tsx fixes"

create_issue \
  "[Hackathon] Security Headers + HTTP Hardening ## Hackathon Sprint" \
  "enhancement,hackathon,security,backend" \
  "## Description

Add defense-in-depth security headers and HTTP hardening:

1. Security headers middleware (HSTS, CSP, X-Frame-Options, X-Content-Type-Options)
2. Response header allowlist for proxy (prevent leaking internal headers)
3. Set \`0o600\` permissions on database files
4. Default \`max_content_len\` to 1000 chars (prevent log injection)

## Flagged By
- Principal Security Engineer (MED-2, MED-12, HIGH-5)

## Estimated Effort
1 day (S)

## Impact
**6/10** — None are exploitable today but all are defense-in-depth gaps.

## File Paths
- Go server: HTTP middleware
- Proxy response handler
- Storage file creation
- Logging configuration

## Acceptance Criteria
- [ ] HSTS, CSP, X-Frame-Options, X-Content-Type-Options headers on all responses
- [ ] Proxy strips internal headers from responses
- [ ] Database files created with 0o600 permissions
- [ ] \`max_content_len\` defaults to 1000 chars
- [ ] Security headers verified with Mozilla Observatory or similar

## Dependencies
None"

create_issue \
  "[Hackathon] IDE Extension v2 — Budget Warning Notifications ## Hackathon Sprint" \
  "enhancement,hackathon" \
  "## Description

When a developer approaches their daily budget, show a VS Code/JetBrains notification: 'You've used \$8.50 of your \$10 daily AI budget.' Add per-session cost tracking.

## Flagged By
- CPO (#6 hackathon pick)

## Estimated Effort
1–2 days (S-M)

## Impact
**7/10** — Makes Candela indispensable at the moment a dev would otherwise be silently cut off.

## File Paths
- IDE extension source
- Budget query API endpoint

## Acceptance Criteria
- [ ] VS Code notification at configurable thresholds (80%, 90%, 100%)
- [ ] Per-session cost tracking in status bar
- [ ] JetBrains plugin parity (stretch goal)
- [ ] Non-intrusive notification style

## Dependencies
None"

echo ""
echo "=== ALL ISSUES CREATED ==="
echo "Results saved to: $RESULTS_FILE"
cat "$RESULTS_FILE"
