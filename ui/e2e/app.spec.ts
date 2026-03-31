import { test, expect } from "@playwright/test";

const API_BASE = "http://localhost:8080";

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
    await expect(page.locator(".sidebar-env")).toContainText("localhost:8080");
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
    // No mock → fetch to :8080 will fail with network error
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
    await expect(page.locator("text=http://localhost:8080")).toBeVisible();
    await expect(page.locator("text=ConnectRPC")).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Costs
// ──────────────────────────────────────────

test.describe("Costs", () => {
  test("renders cost page with placeholder stats", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.locator(".main-header h1")).toHaveText("Costs");
    await expect(page.locator(".card").filter({ hasText: "Today" })).toBeVisible();
    await expect(page.locator(".empty-state-title")).toHaveText("No cost data yet");
  });
});
