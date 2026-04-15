import { test, expect } from "@playwright/test";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8181";

/**
 * Mock a ConnectRPC unary call by intercepting the POST to the service method URL.
 * ConnectRPC uses POST with JSON body to `baseUrl/package.ServiceName/MethodName`.
 */
async function mockConnectRPC(
  page: import("@playwright/test").Page,
  servicePath: string,
  responseBody: Record<string, unknown>,
) {
  await page.route(`${API_BASE}${servicePath}`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(responseBody),
    });
  });
}

// ──────────────────────────────────────────
// Navigation & Layout
// ──────────────────────────────────────────

test.describe("App Shell", () => {
  test("renders sidebar with navigation links", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".sidebar-logo")).toHaveText("Candela");
    await expect(page.locator(".nav-item").filter({ hasText: "Dashboard" })).toBeVisible();
    await expect(page.locator(".nav-item").filter({ hasText: "Traces" })).toBeVisible();
    await expect(page.locator(".nav-item").filter({ hasText: "Costs" })).toBeVisible();
    await expect(page.locator(".nav-item").filter({ hasText: "Projects" })).toBeVisible();
    await expect(page.locator(".nav-item").filter({ hasText: "Settings" })).toBeVisible();
  });

  test("highlights active nav item", async ({ page }) => {
    await page.goto("/traces");
    const tracesNav = page.locator(".nav-item").filter({ hasText: "Traces" });
    await expect(tracesNav).toHaveClass(/active/);
  });

  test("navigates between pages", async ({ page }) => {
    await page.goto("/");
    await page.locator(".nav-item").filter({ hasText: "Traces" }).click();
    await expect(page).toHaveURL("/traces");
    await expect(page.locator(".main-header h1")).toHaveText("Traces");

    await page.locator(".nav-item").filter({ hasText: "Projects" }).click();
    await expect(page).toHaveURL("/projects");
    await expect(page.locator(".main-header h1")).toHaveText("Projects");
  });

  test("shows environment indicator in sidebar footer", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".sidebar-env")).toContainText("Development");
  });
});

// ──────────────────────────────────────────
// Dashboard — with mocked ConnectRPC
// ──────────────────────────────────────────

test.describe("Dashboard", () => {
  test("shows stats when backend responds", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "42",
      totalSpans: "128",
      totalLlmCalls: "42",
      totalInputTokens: "10000",
      totalOutputTokens: "5000",
      totalCostUsd: 3.47,
      avgLatencyMs: 234.5,
      errorRate: 0.02,
      tracesOverTime: [],
      costOverTime: [],
      tokensOverTime: [],
    });
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
      pagination: { totalCount: 0, nextPageToken: "" },
    });

    await page.goto("/");
    await expect(page.locator(".card-value").first()).toHaveText("42");
    await expect(
      page.locator(".card").filter({ hasText: "Total Tokens" }).locator(".card-value")
    ).toHaveText("15,000");
    await expect(
      page.locator(".card").filter({ hasText: "Total Cost" }).locator(".card-value")
    ).toHaveText("$3.47");
    await expect(
      page.locator(".card").filter({ hasText: "Avg Latency" }).locator(".card-value")
    ).toHaveText("235ms");
  });

  test("shows error banner when backend is down", async ({ page }) => {
    // Abort all backend requests to simulate offline
    await page.route(`${API_BASE}/**`, (route) => route.abort());
    await page.goto("/");
    await expect(
      page.locator(".card-title").filter({ hasText: "Backend Unavailable" })
    ).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Traces — with mocked ConnectRPC
// ──────────────────────────────────────────

test.describe("Traces", () => {
  test("shows empty state when no traces", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
    });

    await page.goto("/traces");
    await expect(page.locator(".empty-state-title")).toHaveText("No traces found");
  });

  test("shows error when backend is down", async ({ page }) => {
    // Abort all backend requests to simulate offline
    await page.route(`${API_BASE}/**`, (route) => route.abort());
    await page.goto("/traces");
    await expect(
      page.locator(".card-title").filter({ hasText: "Could not load traces" })
    ).toBeVisible();
  });

  test("renders search bar and filter controls", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
    });

    await page.goto("/traces");
    await expect(page.locator(".filter-search-input")).toBeVisible();
    await expect(page.locator(".filter-search-input")).toHaveAttribute(
      "placeholder",
      "Search traces by name..."
    );
    // Sort dropdown
    await expect(page.locator(".filter-select")).toBeVisible();
    // Sort direction button
    await expect(page.locator(".filter-dir-btn")).toBeVisible();
  });

  test("opens filter panel on click", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
    });

    await page.goto("/traces");
    // Panel should be hidden initially
    await expect(page.locator(".filter-panel")).not.toBeVisible();

    // Click filters button
    await page.locator(".btn").filter({ hasText: "Filters" }).click();
    await expect(page.locator(".filter-panel")).toBeVisible();

    // Should show Model, Provider, Status filters
    await expect(page.locator(".filter-label").filter({ hasText: "Model" })).toBeVisible();
    await expect(page.locator(".filter-label").filter({ hasText: "Provider" })).toBeVisible();
    await expect(page.locator(".filter-label").filter({ hasText: "Status" })).toBeVisible();
  });

  test("shows filtered empty state", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
    });

    await page.goto("/traces");
    // Type in search
    await page.locator(".filter-search-input").fill("nonexistent-trace");
    await page.waitForTimeout(400); // wait for debounce

    await expect(page.locator(".empty-state-title")).toHaveText("No traces match filters");
  });

  test("shows trace count in table header", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [
        {
          traceId: "trace-1",
          rootSpanName: "chat",
          primaryModel: "gpt-4o",
          primaryProvider: "openai",
          environment: "prod",
          duration: "0.5s",
          totalTokens: "100",
          totalCostUsd: 0.001,
          status: 1,
          startTime: "2024-03-31T00:00:00Z",
          spanCount: 2,
          llmCallCount: 1,
        },
      ],
    });

    await page.goto("/traces");
    await expect(page.locator(".table-title")).toHaveText("1 trace");
  });
});

// ──────────────────────────────────────────
// Trace Detail (Waterfall)
// ──────────────────────────────────────────

test.describe("Trace Waterfall", () => {
  const mockTrace = {
    trace: {
      traceId: "abc123def456",
      startTime: "2024-03-31T00:00:00Z",
      endTime: "2024-03-31T00:00:01.500Z",
      duration: "1.5s",
      projectId: "proj-1",
      environment: "production",
      spanCount: 3,
      totalTokens: "2500",
      totalCostUsd: 0.0125,
      rootSpanName: "chat_completion",
      spans: [
        {
          spanId: "span-root",
          traceId: "abc123def456",
          parentSpanId: "",
          name: "chat_completion",
          kind: "SPAN_KIND_AGENT",
          status: "SPAN_STATUS_OK",
          statusMessage: "",
          startTime: "2024-03-31T00:00:00Z",
          endTime: "2024-03-31T00:00:01.500Z",
          duration: "1.5s",
          attributes: [],
          events: [],
          projectId: "proj-1",
          environment: "production",
          serviceName: "my-app",
        },
        {
          spanId: "span-llm",
          traceId: "abc123def456",
          parentSpanId: "span-root",
          name: "gpt-4o-mini",
          kind: "SPAN_KIND_LLM",
          status: "SPAN_STATUS_OK",
          statusMessage: "",
          startTime: "2024-03-31T00:00:00.100Z",
          endTime: "2024-03-31T00:00:01.200Z",
          duration: "1.1s",
          genAi: {
            model: "gpt-4o-mini",
            provider: "openai",
            inputTokens: "1500",
            outputTokens: "1000",
            totalTokens: "2500",
            costUsd: 0.0125,
            temperature: 0.7,
            inputContent: "Hello, how are you?",
            outputContent: "I'm doing great, thanks for asking!",
          },
          attributes: [
            { key: "gen_ai.system", stringValue: "openai" },
          ],
          events: [],
          projectId: "proj-1",
          environment: "production",
          serviceName: "my-app",
        },
        {
          spanId: "span-tool",
          traceId: "abc123def456",
          parentSpanId: "span-root",
          name: "search_web",
          kind: "SPAN_KIND_TOOL",
          status: "SPAN_STATUS_OK",
          statusMessage: "",
          startTime: "2024-03-31T00:00:00.050Z",
          endTime: "2024-03-31T00:00:00.090Z",
          duration: "0.040s",
          tool: {
            toolName: "search_web",
            toolInput: '{"query": "weather today"}',
            toolOutput: '{"result": "sunny, 72°F"}',
          },
          attributes: [],
          events: [],
          projectId: "proj-1",
          environment: "production",
          serviceName: "my-app",
        },
      ],
    },
  };

  test("renders waterfall with span tree", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/GetTrace", mockTrace);
    await page.goto("/traces/abc123def456");

    // Summary bar
    await expect(page.locator(".trace-summary-value").first()).toBeVisible();
    await expect(page.locator(".trace-summary-item").filter({ hasText: "Duration" }).locator(".trace-summary-value")).toHaveText("1500ms");
    await expect(page.locator("text=$0.0125")).toBeVisible();

    // Waterfall rows
    await expect(page.locator(".waterfall-row")).toHaveCount(3);

    // Span kind badges
    await expect(page.locator(".waterfall-kind").filter({ hasText: "AGENT" })).toBeVisible();
    await expect(page.locator(".waterfall-kind").filter({ hasText: "LLM" })).toBeVisible();
    await expect(page.locator(".waterfall-kind").filter({ hasText: "TOOL" })).toBeVisible();
  });

  test("shows span detail on click", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/GetTrace", mockTrace);
    await page.goto("/traces/abc123def456");

    // Click the LLM span
    await page.locator(".waterfall-row").nth(2).click();

    // Detail panel should appear
    await expect(page.locator(".span-detail")).toBeVisible();
    await expect(page.locator(".span-detail h3")).toContainText("gpt-4o-mini");

    // GenAI section
    await expect(page.locator(".span-meta-value").filter({ hasText: "openai" })).toBeVisible();
    await expect(page.locator("text=1,500")).toBeVisible(); // input tokens
  });

  test("shows prompt and completion content", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/GetTrace", mockTrace);
    await page.goto("/traces/abc123def456");

    // Click the LLM span
    await page.locator(".waterfall-row").nth(2).click();

    // Prompt & completion
    await expect(page.locator("text=Hello, how are you?")).toBeVisible();
    await expect(page.locator("text=I'm doing great, thanks for asking!")).toBeVisible();
  });

  test("shows back button that navigates to traces list", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/GetTrace", mockTrace);
    await page.goto("/traces/abc123def456");

    await expect(page.locator("text=← Back")).toBeVisible();
  });

  test("shows error when backend is down", async ({ page }) => {
    await page.route(`${API_BASE}/**`, (route) => route.abort());
    await page.goto("/traces/nonexistent");
    await expect(
      page.locator(".card-title").filter({ hasText: /unavailable|error/i })
    ).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Projects — with mocked ConnectRPC
// ──────────────────────────────────────────

test.describe("Projects", () => {
  test("shows empty state when no projects", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.ProjectService/ListProjects", {
      projects: [],
    });

    await page.goto("/projects");
    await expect(page.locator(".empty-state-title")).toHaveText("No projects yet");
  });

  test("shows New Project button", async ({ page }) => {
    await page.goto("/projects");
    await expect(page.locator(".btn-primary")).toHaveText("+ New Project");
  });
});

// ──────────────────────────────────────────
// Settings
// ──────────────────────────────────────────

test.describe("Settings", () => {
  test("shows backend connection info", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator(".main-header h1")).toHaveText("Settings");
    await expect(page.locator("text=http://localhost:8181")).toBeVisible();
    await expect(page.locator("text=ConnectRPC v2")).toBeVisible();
  });

  test("shows connected status when backend responds", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "100",
      totalSpans: "500",
      totalLlmCalls: "100",
      totalInputTokens: "0",
      totalOutputTokens: "0",
      totalCostUsd: 0,
      avgLatencyMs: 0,
      errorRate: 0,
      costOverTime: [],
      tokensOverTime: [],
    });

    await page.goto("/settings");
    await expect(page.locator("text=Connected")).toBeVisible();
    await expect(page.locator(".health-dot")).toBeVisible();
  });

  test("shows offline status when backend is down", async ({ page }) => {
    // Abort all backend requests to simulate offline
    await page.route(`${API_BASE}/**`, (route) => route.abort());
    await page.goto("/settings");
    await expect(page.locator("text=Offline")).toBeVisible({ timeout: 10000 });
  });

  test("shows configured providers", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator(".settings-provider-chip").filter({ hasText: "OpenAI" })).toBeVisible();
    await expect(page.locator(".settings-provider-chip").filter({ hasText: "Google (Gemini)" })).toBeVisible();
    await expect(page.locator(".settings-provider-chip").filter({ hasText: "Anthropic" })).toBeVisible();
  });

  test("shows storage backend", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator(".badge-info").filter({ hasText: "DuckDB" })).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Costs
// ──────────────────────────────────────────

test.describe("Costs", () => {
  test("renders cost page with empty model breakdown", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "0",
      totalSpans: "0",
      totalLlmCalls: "0",
      totalInputTokens: "0",
      totalOutputTokens: "0",
      totalCostUsd: 0,
      avgLatencyMs: 0,
      errorRate: 0,
      costOverTime: [],
      tokensOverTime: [],
    });
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetModelBreakdown", {
      models: [],
    });

    await page.goto("/costs");
    await expect(page.locator(".main-header h1")).toHaveText("Costs");
    await expect(page.locator(".card").filter({ hasText: "Total Cost" })).toBeVisible();
    await expect(page.locator(".empty-state-title")).toHaveText("No cost data yet");
  });

  test("renders model breakdown table with data", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "50",
      totalSpans: "150",
      totalLlmCalls: "50",
      totalInputTokens: "8000",
      totalOutputTokens: "4000",
      totalCostUsd: 2.75,
      avgLatencyMs: 200,
      errorRate: 0,
      costOverTime: [],
      tokensOverTime: [],
    });
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetModelBreakdown", {
      models: [
        { model: "gpt-4o", provider: "openai", callCount: 30, inputTokens: 5000, outputTokens: 2500, costUsd: 2.0, avgLatencyMs: 250 },
        { model: "gemini-2.5-pro", provider: "google", callCount: 20, inputTokens: 3000, outputTokens: 1500, costUsd: 0.75, avgLatencyMs: 150 },
      ],
    });

    await page.goto("/costs");
    // Summary card
    await expect(
      page.locator(".card").filter({ hasText: "Total Cost" }).locator(".card-value")
    ).toHaveText("$2.7500");
    // Model table — sorted by cost, gpt-4o first
    const rows = page.locator("tbody tr");
    await expect(rows).toHaveCount(2);
    await expect(rows.first().locator("td").first()).toContainText("gpt-4o");
    await expect(rows.nth(1).locator("td").first()).toContainText("gemini-2.5-pro");
  });

  test("time range selector switches period", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "10",
      totalSpans: "30",
      totalLlmCalls: "10",
      totalInputTokens: "1000",
      totalOutputTokens: "500",
      totalCostUsd: 0.5,
      avgLatencyMs: 100,
      errorRate: 0,
      costOverTime: [],
      tokensOverTime: [],
    });
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetModelBreakdown", {
      models: [],
    });

    await page.goto("/costs");
    // Default is 7d
    await expect(
      page.locator(".card").filter({ hasText: "Total Cost" }).locator(".card-subtitle")
    ).toHaveText("Last 7 days");

    // Switch to 24h
    await page.locator(".time-range-btn").filter({ hasText: "24h" }).click();
    await expect(
      page.locator(".card").filter({ hasText: "Total Cost" }).locator(".card-subtitle")
    ).toHaveText("Last 24 hours");
  });
});

// ──────────────────────────────────────────
// Dashboard — time range
// ──────────────────────────────────────────

test.describe("Dashboard time range", () => {
  test("time range selector switches period label", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetUsageSummary", {
      totalTraces: "5",
      totalSpans: "15",
      totalLlmCalls: "5",
      totalInputTokens: "500",
      totalOutputTokens: "250",
      totalCostUsd: 0.1,
      avgLatencyMs: 50,
      errorRate: 0,
      tracesOverTime: [],
      costOverTime: [],
      tokensOverTime: [],
    });
    await mockConnectRPC(page, "/candela.v1.TraceService/ListTraces", {
      traces: [],
      pagination: { totalCount: 0, nextPageToken: "" },
    });

    await page.goto("/");
    // Default is 24h
    await expect(
      page.locator(".card").filter({ hasText: "Total Traces" }).locator(".card-subtitle")
    ).toHaveText("Last 24 hours");

    // Switch to 30d
    await page.locator(".time-range-btn").filter({ hasText: "30d" }).click();
    await expect(
      page.locator(".card").filter({ hasText: "Total Traces" }).locator(".card-subtitle")
    ).toHaveText("Last 30 days");
  });
});

// ──────────────────────────────────────────
// Trace Detail — toggle deselect
// ──────────────────────────────────────────

test.describe("Trace detail interactions", () => {
  const mockTrace = {
    trace: {
      traceId: "toggle-test-123",
      startTime: "2024-03-31T00:00:00Z",
      endTime: "2024-03-31T00:00:01Z",
      duration: "1s",
      projectId: "proj-1",
      environment: "dev",
      spanCount: 1,
      totalTokens: "100",
      totalCostUsd: 0.001,
      rootSpanName: "test_op",
      spans: [
        {
          spanId: "span-only",
          traceId: "toggle-test-123",
          parentSpanId: "",
          name: "test_op",
          kind: "SPAN_KIND_LLM",
          status: "SPAN_STATUS_OK",
          statusMessage: "",
          startTime: "2024-03-31T00:00:00Z",
          endTime: "2024-03-31T00:00:01Z",
          duration: "1s",
          genAi: { model: "gpt-4o", provider: "openai", inputTokens: "50", outputTokens: "50", totalTokens: "100", costUsd: 0.001, temperature: 0 },
          attributes: [],
          events: [],
          projectId: "proj-1",
          environment: "dev",
          serviceName: "test",
        },
      ],
    },
  };

  test("clicking a span then clicking again deselects it", async ({ page }) => {
    await mockConnectRPC(page, "/candela.v1.TraceService/GetTrace", mockTrace);
    await page.goto("/traces/toggle-test-123");

    // Click to select
    await page.locator(".waterfall-row").first().click();
    await expect(page.locator(".span-detail")).toBeVisible();

    // Click again to deselect
    await page.locator(".waterfall-row").first().click();
    await expect(page.locator(".span-detail")).not.toBeVisible();
  });
});

// ──────────────────────────────────────────
// Responsive sidebar
// ──────────────────────────────────────────

test.describe("Responsive sidebar", () => {
  test("sidebar collapses on narrow viewport", async ({ page }) => {
    await page.setViewportSize({ width: 600, height: 800 });
    await page.goto("/");

    // Sidebar text should be hidden
    const sidebar = page.locator(".sidebar");
    await expect(sidebar).toBeVisible();

    // Logo text should be hidden at narrow widths
    const logoText = page.locator(".sidebar-logo");
    await expect(logoText).not.toBeVisible();

    // Nav items should still be clickable
    await page.locator(".nav-item").filter({ hasText: "" }).first().click();
  });
});
