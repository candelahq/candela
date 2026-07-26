import { test, expect } from "@playwright/test";

// ──────────────────────────────────────────
// Error Boundaries & 404 Pages
// ──────────────────────────────────────────

test.describe("Error Handling", () => {
  test("shows 404 page for unknown routes", async ({ page }) => {
    await page.goto("/this-route-does-not-exist");
    // The root not-found.tsx should render
    await expect(page.getByText("Page not found")).toBeVisible();
    await expect(page.getByRole("link", { name: "Go home" })).toBeVisible();
  });

  test("404 page has working home link", async ({ page }) => {
    await page.goto("/this-route-does-not-exist");
    await expect(page.getByText("Page not found")).toBeVisible();
    const homeLink = page.getByRole("link", { name: "Go home" });
    await expect(homeLink).toHaveAttribute("href", "/");
  });

});

