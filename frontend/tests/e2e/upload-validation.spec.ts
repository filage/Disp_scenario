import { expect, test, type Page } from "@playwright/test";
import { loginAsDemo } from "./auth";

const apiURL = process.env.E2E_API_URL ?? "/api/backend";

test.describe("recording upload validation", () => {
  test.beforeEach(async ({ page }) => {
    await loginAsDemo(page);
  });

  test("unsupported file upload is rejected without creating a recording", async ({
    page,
  }) => {
    const originalName = `e2e-unsupported-${Date.now()}.txt`;
    const before = await listRecordings(page);

    await page.goto("/recordings");
    await page.locator('input[type="file"]').setInputFiles({
      name: originalName,
      mimeType: "text/plain",
      buffer: Buffer.from("not a supported video file"),
    });

    await expect(page.getByText("Не удалось создать upload")).toBeVisible();

    await expect
      .poll(async () => {
        const after = await listRecordings(page);
        return {
          countChanged: after.length !== before.length,
          unsupportedExists: after.some(
            (recording) => recording.originalName === originalName,
          ),
        };
      })
      .toEqual({ countChanged: false, unsupportedExists: false });

    const response = await page.request.post(`${apiURL}/v1/recordings/uploads`, {
      data: {
        originalName,
        mimeType: "text/plain",
        sizeBytes: 26,
      },
    });
    expect(response.status()).toBe(400);
    await expect(response.text()).resolves.toContain(
      "only video/webm and video/mp4 are supported",
    );
  });
});

async function listRecordings(page: Page): Promise<Recording[]> {
  const response = await page.request.get(`${apiURL}/v1/recordings`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { items: Recording[] };
  return body.items;
}

type Recording = {
  id: string;
  originalName: string;
};
