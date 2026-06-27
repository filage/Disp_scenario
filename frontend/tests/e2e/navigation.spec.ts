import { expect, test } from "@playwright/test";

const routes = [
  "/overview",
  "/recordings",
  "/runs",
  "/timeline",
  "/scenario-map",
  "/groups",
  "/qa",
  "/automation",
  "/reports",
  "/settings",
];

for (const route of routes) {
  test(`${route} renders application shell`, async ({ page }) => {
    await page.goto(route);
    await expect(page.getByText("DispScenario", { exact: false }).first()).toBeVisible();
    await expect(page.locator("main")).toBeVisible();
  });
}
