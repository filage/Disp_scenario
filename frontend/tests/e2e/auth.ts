import type { Page } from "@playwright/test";

export async function loginAsDemo(page: Page) {
  const response = await page.request.post("/api/session/login", {
    data: {
      username: process.env.E2E_DEMO_USERNAME ?? "demo",
      password: process.env.E2E_DEMO_PASSWORD ?? "demo",
    },
  });
  if (!response.ok()) {
    throw new Error(`E2E demo login failed with HTTP ${response.status()}`);
  }
}
