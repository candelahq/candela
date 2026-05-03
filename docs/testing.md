# 🧪 Testing Guide

Candela has a comprehensive test suite covering unit tests, integration tests, and end-to-end browser tests. This document covers test architecture, running tests, and contributing new tests.

## Test Architecture Overview

```
tests/
├── Go Unit Tests (~20 files)
│   ├── pkg/proxy/*_test.go         — proxy, translation, circuit breaker
│   ├── pkg/costcalc/*_test.go      — cost calculation
│   ├── pkg/processor/*_test.go     — span processor batching/fanout
│   ├── pkg/auth/*_test.go          — auth middleware, admin guard, context
│   ├── pkg/runtime/*_test.go       — runtime interface, discovery, manager
│   ├── pkg/notify/*_test.go        — budget notifications
│   ├── pkg/storage/*_test.go       — storage interfaces
│   └── cmd/candela-local/*_test.go — LM handler, span capture, state, API
│
├── Go Integration Tests
│   ├── pkg/connecthandlers/integration_test.go — full service stack
│   ├── pkg/connecthandlers/project_handler_test.go
│   ├── pkg/connecthandlers/user_handler_test.go
│   └── cmd/candela-local/ui_integration_test.go
│
└── Playwright E2E Tests (3 suites)
    ├── e2e/app.spec.ts             — dashboard, traces, costs, usage
    ├── e2e/admin.spec.ts           — user mgmt, budgets, audit
    └── e2e/team_mode.spec.ts       — leaderboard, per-user, alerts
```

---

## Running Tests

### All Go Tests

```bash
# Quick run (from nix shell)
nix develop -c go test ./...

# With race detector and verbose output
nix develop -c go test ./... -v -race -count=1

# With coverage
nix develop -c go test ./... -coverprofile=coverage.out
nix develop -c go tool cover -func=coverage.out | tail -1
```

### Specific Packages

```bash
# Proxy tests
nix develop -c go test ./pkg/proxy -v

# Auth tests
nix develop -c go test ./pkg/auth -v

# Runtime tests
nix develop -c go test ./pkg/runtime/... -v

# candela-local tests
nix develop -c go test ./cmd/candela-local -v

# Integration tests (ConnectRPC handlers)
nix develop -c go test ./pkg/connecthandlers -v -run TestIntegration
```

### Playwright E2E Tests

```bash
cd ui

# Install browsers (first time only)
pnpm exec playwright install --with-deps chromium

# Run all E2E tests (headless)
pnpm run test:e2e

# Interactive UI mode (recommended for debugging)
pnpm run test:e2e:ui

# Run a specific test file
pnpm exec playwright test e2e/admin.spec.ts

# Run a specific test by name
pnpm exec playwright test -g "should show budget gauge"
```

---

## Lefthook Git Hooks

The `lefthook.yml` runs these checks on every `git commit`:

| Hook | What It Does | Blocks Commit? |
|------|-------------|---------------|
| `trailing-whitespace` | Detects trailing whitespace | Yes |
| `check-yaml` | Validates YAML syntax | Yes |
| `golangci-lint` | Go linting (5min timeout) | Yes |
| `go-vet` | Go static analysis | Yes |
| `go-mod-tidy` | Checks `go.mod` is tidy | Yes |
| `gofmt` | Go formatting check | Yes |
| `go-test` | Runs full test suite (30s timeout) | Yes |

> [!NOTE]
> Protobuf linting is handled in [`candelahq/candela-protos`](https://github.com/candelahq/candela-protos).

> [!IMPORTANT]
> Per project conventions, always run `git commit` inside the nix shell:
> ```bash
> nix develop -c git commit -m "feat: your message"
> ```
> This ensures lefthook hooks have access to all required tools.

---

## CI Pipeline

The GitHub Actions CI (`.github/workflows/ci.yml`) runs three jobs:

### 1. `build-and-test` (Go)
- Proto generation via Buf (remote, requires `BUF_TOKEN`)
- `go build ./...`
- `go vet ./...`
- `go test ./... -v -race -count=1 -coverprofile=coverage.out`
- Coverage summary
- `govulncheck ./...` (advisory, non-blocking)

### 2. `ui-build-and-lint` (Next.js)
- `pnpm install`
- Proto generation (TS stubs)
- Copy generated types to `ui/src/gen/`
- `pnpm run lint` (ESLint)
- `pnpm run build` (TypeScript type-check)

### 3. `ui-e2e` (Playwright)
- Depends on `ui-build-and-lint`
- Installs Playwright + Chromium
- Runs all E2E tests
- Uploads test report as artifact (7-day retention)

---

## Test Patterns

### Unit Test Structure

Tests follow Go conventions — same package, `_test.go` suffix:

```go
// pkg/proxy/proxy_test.go
func TestProxy_HandleStandardResponse(t *testing.T) {
    // Setup: create test server, proxy, submitter mock
    // Act: send request through proxy
    // Assert: verify span was created with correct attributes
}
```

### Table-Driven Tests

Used extensively for parameterized testing:

```go
func TestCalculator_Calculate(t *testing.T) {
    tests := []struct {
        name     string
        provider string
        model    string
        input    int64
        output   int64
        want     float64
    }{
        {"gpt-4o standard", "openai", "gpt-4o", 1000, 500, 0.0075},
        {"local model", "local", "llama3", 1000, 500, 0.0},
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Integration Tests

The `connecthandlers/integration_test.go` tests the full service stack with real storage:

```go
func TestIntegration_ListTraces(t *testing.T) {
    // Creates an in-memory SQLite store
    // Wires up ConnectRPC handlers
    // Makes real RPC calls through the handler
    // Verifies end-to-end behavior
}
```

### E2E Test Patterns

Playwright tests use route interception to mock the backend:

```typescript
test("should display cost analytics", async ({ page }) => {
    // Mock the ConnectRPC response
    await page.route("**/candela.v1.DashboardService/GetModelBreakdown", (route) => {
        route.fulfill({ body: JSON.stringify({ models: [...] }) });
    });

    await page.goto("/costs");
    await expect(page.getByText("Cost Analytics")).toBeVisible();
});
```

---

## Adding New Tests

### Go Unit Test

1. Create `yourpkg/yourfile_test.go` in the same package
2. Use `testing.T` and standard assertions
3. For storage tests, use `sqlitestore.New(":memory:")` for ephemeral databases

### E2E Test

1. Add to the appropriate suite (`app.spec.ts`, `admin.spec.ts`, or `team_mode.spec.ts`)
2. Mock backend responses via `page.route()`
3. Use semantic selectors (`getByText`, `getByRole`) over CSS selectors
4. Run with `pnpm run test:e2e:ui` during development

---

## Vulnerability Scanning

```bash
nix develop -c govulncheck ./...
```

This runs as advisory in CI (non-blocking). Fix critical vulnerabilities by updating dependencies in `go.mod`.
