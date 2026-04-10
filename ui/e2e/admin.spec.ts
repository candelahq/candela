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

async function mockConnectRPCError(
  page: import("@playwright/test").Page,
  servicePath: string,
  code: string,
  message: string,
) {
  await page.route(`${API_BASE}${servicePath}`, async (route) => {
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ code, message }),
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
// Admin Route Guard
// ──────────────────────────────────────────

test.describe("Admin Route Guard", () => {
  test("non-admin sees access denied on /admin/users", async ({ page }) => {
    await mockDevUser(page);
    await page.goto("/admin/users");
    await expect(page.locator("h2").filter({ hasText: "Access Denied" })).toBeVisible();
    await expect(page.locator("#users-table")).not.toBeVisible();
  });

  test("non-admin sees access denied on /admin/budgets", async ({ page }) => {
    await mockDevUser(page);
    await page.goto("/admin/budgets");
    await expect(page.locator("h2").filter({ hasText: "Access Denied" })).toBeVisible();
  });

  test("non-admin sees access denied on /admin/audit", async ({ page }) => {
    await mockDevUser(page);
    await page.goto("/admin/audit");
    await expect(page.locator("h2").filter({ hasText: "Access Denied" })).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Admin Users Page
// ──────────────────────────────────────────

test.describe("Admin Users Page", () => {
  test("renders user table for admin", async ({ page }) => {
    await mockAdminUser(page);
    await mockConnectRPC(page, "/candela.v1.UserService/ListUsers", {
      users: [
        {
          id: "u1",
          email: "alice@test.com",
          displayName: "Alice",
          role: 1,
          status: 2,
          lastSeenAt: { seconds: "1712000000" },
        },
        {
          id: "u2",
          email: "bob@test.com",
          displayName: "Bob",
          role: 2,
          status: 3,
        },
      ],
      pagination: { totalCount: 2 },
    });

    await page.goto("/admin/users");
    await expect(page.locator("#users-table")).toBeVisible();
    // Wait for the table to fully render with mock data
    await expect(page.locator("#users-table tbody tr")).toHaveCount(2, { timeout: 10000 });
    await expect(page.locator("text=alice@test.com")).toBeVisible();
    await expect(page.locator("text=bob@test.com")).toBeVisible();
  });

  test("shows create user modal", async ({ page }) => {
    await mockAdminUser(page);
    await mockConnectRPC(page, "/candela.v1.UserService/ListUsers", {
      users: [],
      pagination: { totalCount: 0 },
    });

    await page.goto("/admin/users");
    await page.click("#create-user-btn");
    await expect(page.locator(".modal")).toBeVisible();
    await expect(page.locator("#create-email")).toBeVisible();
    await expect(page.locator("#create-role")).toBeVisible();
  });

  test("shows status badges correctly", async ({ page }) => {
    await mockAdminUser(page);
    await mockConnectRPC(page, "/candela.v1.UserService/ListUsers", {
      users: [
        { id: "u1", email: "a@t.com", displayName: "A", role: 2, status: 1 },
        { id: "u2", email: "b@t.com", displayName: "B", role: 2, status: 2 },
        { id: "u3", email: "c@t.com", displayName: "C", role: 2, status: 3 },
      ],
      pagination: { totalCount: 3 },
    });

    await page.goto("/admin/users");
    await expect(page.locator(".status-provisioned")).toBeVisible();
    await expect(page.locator(".status-active")).toBeVisible();
    await expect(page.locator(".status-inactive")).toBeVisible();
  });

  test("deactivate button visible for active users", async ({ page }) => {
    await mockAdminUser(page);
    await mockConnectRPC(page, "/candela.v1.UserService/ListUsers", {
      users: [
        { id: "u1", email: "a@t.com", displayName: "A", role: 2, status: 2 },
      ],
      pagination: { totalCount: 1 },
    });

    await page.goto("/admin/users");
    await expect(page.locator(".btn-danger").filter({ hasText: "Deactivate" })).toBeVisible();
  });

  test("reactivate button visible for inactive users", async ({ page }) => {
    await mockAdminUser(page);
    await mockConnectRPC(page, "/candela.v1.UserService/ListUsers", {
      users: [
        { id: "u1", email: "a@t.com", displayName: "A", role: 2, status: 3 },
      ],
      pagination: { totalCount: 1 },
    });

    await page.goto("/admin/users");
    await expect(page.locator(".btn-success").filter({ hasText: "Reactivate" })).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Admin Budgets Page
// ──────────────────────────────────────────

test.describe("Admin Budgets Page", () => {
  test("renders budget explainer for admin", async ({ page }) => {
    await mockAdminUser(page);
    await page.goto("/admin/budgets");
    await expect(page.locator("text=Budget Enforcement")).toBeVisible();
    await expect(page.locator("text=Waterfall")).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Admin Audit Page
// ──────────────────────────────────────────

test.describe("Admin Audit Page", () => {
  test("renders audit page for admin", async ({ page }) => {
    await mockAdminUser(page);
    await page.goto("/admin/audit");
    await expect(page.locator(".admin-page-title").filter({ hasText: "Audit Log" })).toBeVisible();
  });
});

// ──────────────────────────────────────────
// Sidebar Admin Section
// ──────────────────────────────────────────

test.describe("Sidebar Admin Visibility", () => {
  test("admin sees Admin section in sidebar", async ({ page }) => {
    await mockAdminUser(page);
    await page.goto("/");
    await expect(page.locator(".nav-section-label").filter({ hasText: "Admin" })).toBeVisible();
  });

  test("developer does NOT see Admin section in sidebar", async ({ page }) => {
    await mockDevUser(page);
    await page.goto("/");
    await expect(page.locator(".nav-section-label").filter({ hasText: "Admin" })).not.toBeVisible();
  });
});
