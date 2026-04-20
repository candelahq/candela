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

// Mock GetCurrentUser to return an admin user.
async function mockAdminUser(page: import("@playwright/test").Page) {
  await mockConnectRPC(page, "/candela.v1.UserService/GetCurrentUser", {
    user: {
      id: "admin-1",
      email: "admin@test.com",
      displayName: "Admin User",
      role: 2, // ADMIN
      status: 2, // ACTIVE
    },
    budget: { limitUsd: 100, spentUsd: 10, periodType: 1 },
    activeGrants: [],
  });
}

// Mock GetCurrentUser to return a developer user.
async function mockDevUser(page: import("@playwright/test").Page) {
  await mockConnectRPC(page, "/candela.v1.UserService/GetCurrentUser", {
    user: {
      id: "dev-1",
      email: "dev@test.com",
      displayName: "Dev User",
      role: 1, // DEVELOPER
      status: 2, // ACTIVE
    },
    budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
    activeGrants: [],
  });
}

// ──────────────────────────────────────────
// Personal Usage Dashboard (/usage)
// ──────────────────────────────────────────

test.describe("Usage Dashboard", () => {
  test("renders personal usage and budget gauge for dev", async ({ page }) => {
    await mockDevUser(page);

    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      totalCalls: "150",
      totalInputTokens: "50000",
      totalOutputTokens: "25000",
      totalCostUsd: 1.25,
      avgLatencyMs: 450,
      models: [
        { model: "gpt-4o", provider: "openai", callCount: "100", costUsd: 1.0 },
        { model: "claude-3-sonnet", provider: "anthropic", callCount: "50", costUsd: 0.25 }
      ],
      budget: { limitUsd: 50, spentUsd: 5, periodType: 1 },
      totalRemainingUsd: 45
    });

    await page.goto("/usage");

    // Check header
    await expect(page.locator("h1")).toContainText("My Usage");

    // Check Budget Gauge
    await expect(page.locator("text=Personal Budget")).toBeVisible();
    await expect(page.locator("text=$5.00")).toBeVisible(); // Spent
    await expect(page.locator("text=$45.00")).toBeVisible(); // Remaining

    // Check Stats Grid
    await expect(page.locator("text=150")).toBeVisible(); // Total Calls
    await expect(page.locator("text=$1.2500")).toBeVisible(); // Cost

    // Check Table
    await expect(page.locator("text=gpt-4o")).toBeVisible();
    await expect(page.locator("text=claude-3-sonnet")).toBeVisible();
  });

  test("shows global budget alert on home page when over budget", async ({ page }) => {
    await mockDevUser(page);

    // Mock user spending 95% of budget ($47.50 / $50.0)
    await mockConnectRPC(page, "/candela.v1.DashboardService/GetMyUsage", {
      budget: { limitUsd: 50, spentUsd: 47.5, periodType: 1 },
      totalRemainingUsd: 2.5
    });

    await page.goto("/");

    // Check Global Alert presence
    await expect(page.locator(".budget-alert-banner")).toBeVisible();
    await expect(page.locator("text=You've used 95% of your budget")).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Team Leaderboard (/admin/leaderboard)
// ──────────────────────────────────────────

test.describe("Team Leaderboard", () => {
  test("non-admin sees access denied on /admin/leaderboard", async ({ page }) => {
    await mockDevUser(page);
    await page.goto("/admin/leaderboard");

    // The page uses redirect() which might show home or generic error in mock environment,
    // but based on our implementation, it redirects if not admin.
    // In our test, next/navigation handles this.
    // We expect to NOT see the leaderboard title.
    await expect(page.locator("h1").filter({ hasText: "Team Leaderboard" })).not.toBeVisible();
  });

  test("admin sees leaderboard and ranked users", async ({ page }) => {
    await mockAdminUser(page);

    await mockConnectRPC(page, "/candela.v1.DashboardService/GetTeamLeaderboard", {
      users: [
        {
          userId: "u1",
          user_id: "u1",
          email: "alice@test.com",
          displayName: "Alice",
          display_name: "Alice",
          callCount: 500,
          call_count: 500,
          totalTokens: 200000,
          total_tokens: 200000,
          costUsd: 15.5,
          cost_usd: 15.5,
          avgLatencyMs: 320,
          avg_latency_ms: 320,
          topModel: "gpt-4o",
          top_model: "gpt-4o"
        },
        {
          userId: "u2",
          user_id: "u2",
          email: "bob@test.com",
          displayName: "Bob",
          display_name: "Bob",
          callCount: 300,
          call_count: 300,
          totalTokens: 100000,
          total_tokens: 100000,
          costUsd: 8.2,
          cost_usd: 8.2,
          avgLatencyMs: 450,
          avg_latency_ms: 450,
          topModel: "claude-3-opus",
          top_model: "claude-3-opus"
        }
      ]
    });

    await page.goto("/admin/leaderboard");

    // Header and Subtitle (Standard Admin UI)
    await expect(page.locator(".admin-page-title")).toContainText("Team Leaderboard");
    await expect(page.locator(".admin-page-subtitle")).toBeVisible();

    // Check Rankings Table
    const table = page.locator("table");
    await expect(table).toBeVisible();

    // First Row (Alice)
    const row1 = table.locator("tbody tr").first();
    await expect(row1.locator("text=#1")).toBeVisible();
    await expect(row1.locator("text=/Alice/i")).toBeVisible();
    await expect(row1.locator("text=$15.50")).toBeVisible();
    await expect(row1.locator("text=gpt-4o")).toBeVisible();

    // Second Row (Bob)
    const row2 = table.locator("tbody tr").nth(1);
    await expect(row2.locator("text=#2")).toBeVisible();
    await expect(row2.locator("text=/Bob/i")).toBeVisible();
    await expect(row2.locator("text=$8.20")).toBeVisible();
  });
});
