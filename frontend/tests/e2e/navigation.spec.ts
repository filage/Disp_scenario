import { expect, test } from "@playwright/test";
import { loginAsDemo } from "./auth";

const routes = [
  "/overview",
  "/recordings",
  "/runs",
  "/timeline",
  "/guide",
  "/scenario-map",
  "/groups",
  "/qa",
  "/automation",
  "/reports",
  "/settings",
];

test.beforeEach(async ({ page }) => {
  await loginAsDemo(page);
});

for (const route of routes) {
  test(`${route} renders application shell`, async ({ page }) => {
    await page.goto(route);
    await expect(
      page.getByText("DispScenario", { exact: false }).first(),
    ).toBeVisible();
    await expect(page.locator("main")).toBeVisible();
  });
}
