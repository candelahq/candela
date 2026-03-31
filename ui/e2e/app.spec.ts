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
