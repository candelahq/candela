import { test, expect } from "@playwright/test";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8181";

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

async function mockDevUser(page: import("@playwright/test").Page) {
  await mockConnectRPC(page, "/candela.v1.UserService/GetCurrentUser", {
    user: {
      id: "dev-1",
      email: "dev@test.com",
      displayName: "Dev User",
      role: 1,
      status: 2,
    },
    budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
    activeGrants: [],
  });
}

// ──────────────────────────────────────────
// Today Budget View (/today)
// ──────────────────────────────────────────

test.describe("Today Budget View", () => {
  test("renders page header with today's date", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "0",
      totalInputTokens: "0",
      totalOutputTokens: "0",
      totalCostUsd: 0,
      avgLatencyMs: 0,
      models: [],
      budget: { limitUsd: 50, spentUsd: 0, periodType: 1 },
      totalRemainingUsd: 50,
    });

    await page.goto("/today");
    await expect(page.locator("h1")).toHaveText("Today");
    // Date string should be visible (e.g. "Saturday, April 26, 2026")
    await expect(page.locator(".today-date")).toBeVisible();
  });

  test("shows budget ring gauge with spent/limit/remaining", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "42",
      totalInputTokens: "10000",
      totalOutputTokens: "5000",
      totalCostUsd: 1.25,
      avgLatencyMs: 340,
      models: [
        { model: "gpt-4o", provider: "openai", callCount: "30", inputTokens: "7000", outputTokens: "3000", costUsd: 1.0, avgLatencyMs: 300 },
        { model: "claude-3-sonnet", provider: "anthropic", callCount: "12", inputTokens: "3000", outputTokens: "2000", costUsd: 0.25, avgLatencyMs: 400 },
      ],
      budget: { limitUsd: 50, spentUsd: 12.5, periodType: 1 },
      totalRemainingUsd: 37.5,
    });

    await page.goto("/today");

    // Budget ring should show percentage
    await expect(page.locator(".today-ring-pct")).toHaveText("25%");

    // Spent / Limit / Left stats
    await expect(page.locator(".today-ring-stat").filter({ hasText: "Spent" })).toContainText("$12.50");
    await expect(page.locator(".today-ring-stat").filter({ hasText: "Limit" })).toContainText("$50.00");
    await expect(page.locator(".today-ring-stat").filter({ hasText: "Left" })).toContainText("$37.50");
  });

  test("shows quick stats cards with today's data", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "42",
      totalInputTokens: "10000",
      totalOutputTokens: "5000",
      totalCostUsd: 1.25,
      avgLatencyMs: 340,
      models: [],
      budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
      totalRemainingUsd: 45,
    });

    await page.goto("/today");

    // Requests card
    await expect(
      page.locator(".today-stat-card").filter({ hasText: "Requests" }).locator(".card-value")
    ).toHaveText("42");

    // Cost card
    await expect(
      page.locator(".today-stat-card").filter({ hasText: "Cost" }).locator(".card-value")
    ).toHaveText("$1.2500");

    // Latency card
    await expect(
      page.locator(".today-stat-card").filter({ hasText: "Avg Latency" }).locator(".card-value")
    ).toHaveText("340ms");
  });

  test("shows per-model token breakdown sorted by cost", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "42",
      totalInputTokens: "10000",
      totalOutputTokens: "5000",
      totalCostUsd: 1.25,
      avgLatencyMs: 340,
      models: [
        { model: "gpt-4o", provider: "openai", callCount: "30", inputTokens: "7000", outputTokens: "3000", costUsd: 1.0, avgLatencyMs: 300 },
        { model: "claude-3-sonnet", provider: "anthropic", callCount: "12", inputTokens: "3000", outputTokens: "2000", costUsd: 0.25, avgLatencyMs: 400 },
      ],
      budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
      totalRemainingUsd: 45,
    });

    await page.goto("/today");

    // Model names should be visible, sorted by cost desc (gpt-4o first)
    const models = page.locator(".today-model-row");
    await expect(models).toHaveCount(2);
    await expect(models.first()).toContainText("gpt-4o");
    await expect(models.nth(1)).toContainText("claude-3-sonnet");

    // Token legend
    await expect(page.locator(".today-legend-item").filter({ hasText: "Input" })).toBeVisible();
    await expect(page.locator(".today-legend-item").filter({ hasText: "Output" })).toBeVisible();
  });

  test("shows empty state when no activity today", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "0",
      totalInputTokens: "0",
      totalOutputTokens: "0",
      totalCostUsd: 0,
      avgLatencyMs: 0,
      models: [],
      budget: { limitUsd: 50, spentUsd: 0, periodType: 1 },
      totalRemainingUsd: 50,
    });

    await page.goto("/today");
    await expect(page.locator(".empty-state-title")).toHaveText("No activity yet today");
  });

  test("shows no-budget message when budget is not configured", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "10",
      totalInputTokens: "1000",
      totalOutputTokens: "500",
      totalCostUsd: 0.05,
      avgLatencyMs: 200,
      models: [],
      // No budget field
      totalRemainingUsd: 0,
    });

    await page.goto("/today");
    await expect(page.locator(".today-no-budget-title")).toHaveText("No daily budget configured");
  });

  test("shows alert when budget is nearly exhausted (>=90%)", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "200",
      totalInputTokens: "80000",
      totalOutputTokens: "40000",
      totalCostUsd: 47.5,
      avgLatencyMs: 300,
      models: [],
      budget: { limitUsd: 50, spentUsd: 47.5, periodType: 1 },
      totalRemainingUsd: 2.5,
    });

    await page.goto("/today");
    await expect(page.locator(".today-hero-alert")).toContainText("Approaching daily budget limit");
  });

  test("shows exhausted alert when budget is at 100%", async ({ page }) => {
    await mockDevUser(page);
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "300",
      totalInputTokens: "100000",
      totalOutputTokens: "50000",
      totalCostUsd: 52.0,
      avgLatencyMs: 350,
      models: [],
      budget: { limitUsd: 50, spentUsd: 52, periodType: 1 },
      totalRemainingUsd: 0,
    });

    await page.goto("/today");
    await expect(page.locator(".today-hero-alert")).toContainText("Daily budget exhausted");
  });

  test("shows error banner when backend is down", async ({ page }) => {
    await page.route(`${API_BASE}/**`, (route) => route.abort());
    await page.goto("/today");
    await expect(
      page.locator(".card-title").filter({ hasText: "Connection Error" })
    ).toBeVisible();
  });

  test("refresh button triggers data reload", async ({ page }) => {
    await mockDevUser(page);

    let callCount = 0;
    await page.route(`${API_BASE}/candela.v1.DashboardService/GetMyUsage`, async (route) => {
      callCount++;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          totalCalls: String(callCount * 10),
          totalInputTokens: "1000",
          totalOutputTokens: "500",
          totalCostUsd: 0.5,
          avgLatencyMs: 200,
          models: [],
          budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
          totalRemainingUsd: 45,
        }),
      });
    });

    await page.goto("/today");
    // Wait for page to settle — read whatever initial value landed
    const requestsCard = page.locator(".today-stat-card").filter({ hasText: "Requests" }).locator(".card-value");
    await expect(requestsCard).not.toHaveText("—");
    const initialValue = await requestsCard.textContent();

    // Click refresh and wait for the network call to complete
    const responsePromise = page.waitForResponse(
      (res) => res.url().includes("/candela.v1.DashboardService/GetMyUsage") && res.status() === 200
    );
    await page.locator("button").filter({ hasText: "🔄" }).click();
    await responsePromise;

    // Value should have changed from whatever it was
    await expect(requestsCard).not.toHaveText(initialValue!);
  });
});

// ──────────────────────────────────────────
// Sidebar — Today navigation
// ──────────────────────────────────────────

test.describe("Today navigation", () => {
  test("sidebar shows Today link and navigates to /today", async ({ page }) => {
    await page.goto("/");
    const todayLink = page.locator(".nav-item").filter({ hasText: "Today" });
    await expect(todayLink).toBeVisible();

    await todayLink.click();
    await expect(page).toHaveURL("/today");
    await expect(page.locator("h1")).toHaveText("Today");
  });

  test("Today nav item is highlighted when active", async ({ page }) => {
    await page.goto("/today");
    const todayNav = page.locator(".nav-item").filter({ hasText: "Today" });
    await expect(todayNav).toHaveClass(/active/);
  });
});
