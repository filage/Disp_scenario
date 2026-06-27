import { expect, test, type ConsoleMessage } from "@playwright/test";

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

const viewports = [
  { name: "mobile", width: 390, height: 900 },
  { name: "desktop", width: 1440, height: 1000 },
];

for (const viewport of viewports) {
  test.describe(`${viewport.name} UI parity guard`, () => {
    for (const route of routes) {
      test(`${route} keeps legacy shell without page-level horizontal overflow`, async ({
        page,
      }) => {
        const consoleErrors: ConsoleMessage[] = [];
        page.on("console", (message) => {
          if (message.type() === "error") {
            consoleErrors.push(message);
          }
        });

        await page.setViewportSize({
          width: viewport.width,
          height: viewport.height,
        });
        await page.goto(route);
        await page.waitForLoadState("networkidle");

        await expect(page.getByText("DispScenario", { exact: false }).first()).toBeVisible();
        await expect(page.locator("main")).toBeVisible();

        const layout = await page.evaluate(() => ({
          clientWidth: document.documentElement.clientWidth,
          scrollWidth: document.documentElement.scrollWidth,
          title: document.querySelector("h1")?.textContent?.trim() ?? "",
        }));

        expect(layout.title, `${route} should expose a page heading`).not.toBe("");
        expect(
          layout.scrollWidth,
          `${route} should not expand the document horizontally at ${viewport.width}px`,
        ).toBeLessThanOrEqual(layout.clientWidth + 1);
        expect(
          consoleErrors.map((message) => message.text()),
          `${route} should not emit browser console errors`,
        ).toEqual([]);
      });
    }
  });
}
