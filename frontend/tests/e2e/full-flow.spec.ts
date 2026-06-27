import { basename, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

const fullStack = process.env.E2E_FULL_STACK === "true";
const apiURL = process.env.E2E_API_URL ?? "http://localhost:8787";
const projectRoot = resolve(process.cwd(), "..");
const excludedTestRecording = "20260512_081010_1x92893";
const defaultRealVideo = resolve(
  process.cwd(),
  "..",
  "..",
  "Admin Portal - Google Chrome 2026-06-24 16-47-31.mp4",
);
const realVideo = resolve(process.env.E2E_REAL_VIDEO ?? defaultRealVideo);

test.describe("real Gemini analysis flow", () => {
  test.skip(
    !fullStack,
    "requires the Docker Compose stack and a real Gemini key",
  );
  test.setTimeout(10 * 60_000);

  test("real video → Gemini → timeline → QA complete → report → export", async ({
    page,
  }) => {
    expect(
      basename(realVideo).startsWith(excludedTestRecording),
      `${excludedTestRecording} is explicitly excluded from automated tests`,
    ).toBe(false);
    expect(
      existsSync(realVideo),
      `Real E2E video is missing: ${realVideo}. Set E2E_REAL_VIDEO.`,
    ).toBe(true);

    const runID = `${Date.now()}-${process.pid}`;
    const correlationID = `e2e-${runID}`;
    const uploadedName = `e2e-real-${runID}-${basename(realVideo)}`;
    let recordingID: string | undefined;

    try {
      await page.setExtraHTTPHeaders({
        "X-Correlation-ID": correlationID,
      });
      await page.goto("/recordings");
      const uploadInput = page.locator('input[type="file"]');
      await expect(uploadInput).toBeEnabled();
      await uploadInput.setInputFiles({
        name: uploadedName,
        mimeType: "video/mp4",
        buffer: readFileSync(realVideo),
      });

      recordingID = await uploadedRecordingIDByName(page, uploadedName);
      expect(recordingID).toBeTruthy();

      await page.reload();
      const row = page.getByRole("row").filter({ hasText: uploadedName });
      await expect(row).toBeVisible();
      await row.getByRole("button", { name: "Анализировать" }).click();

      const completedRun = await waitForCompletedRun(page, recordingID);
      expect(completedRun.provider).toBe("gemini");
      expect(completedRun.model).toBeTruthy();
      expect(completedRun.promptVersion).toBe("video-raw-extractor-v7");

      await page.goto("/recordings");
      await page.getByRole("row").filter({ hasText: uploadedName }).click();
      await expect(page.locator("aside video")).toHaveAttribute(
        "src",
        /^http:\/\/localhost:9000\//,
      );

      await page.goto(`/timeline?recordingId=${recordingID}`);
      await expect(page.getByText("Канонический таймлайн")).toBeVisible();
      await expect(page.locator("tbody tr").first()).toBeVisible();
      await expect(
        page
          .getByTestId("scenario-boundary")
          .filter({ hasText: "Начало сценария" })
          .first(),
      ).toBeVisible();
      await expect(
        page
          .getByTestId("scenario-boundary")
          .filter({ hasText: "Конец сценария" })
          .first(),
      ).toBeVisible();

      await page.goto(`/qa?recordingId=${recordingID}`);
      const fragments = page.getByTestId("qa-evidence-fragment");
      await expect(fragments.first()).toBeVisible();
      await expect(
        page
          .getByTestId("scenario-boundary")
          .filter({ hasText: "Начало сценария" })
          .first(),
      ).toBeVisible();
      await expect(
        page
          .getByTestId("scenario-boundary")
          .filter({ hasText: "Конец сценария" })
          .first(),
      ).toBeVisible();
      await assertEvidenceFragmentsWrap(page);
      const targetFragment = fragments.nth(1);
      const targetEventID = await targetFragment.getAttribute("data-event-id");
      const timestampMS = Number(
        await targetFragment.getAttribute("data-timestamp-ms"),
      );
      await targetFragment.click();
      await expect(targetFragment).toHaveAttribute("aria-pressed", "true");
      await expect
        .poll(async () =>
          page
            .getByTestId("qa-evidence-video")
            .evaluate((video: HTMLVideoElement) => video.currentTime),
        )
        .toBeCloseTo(timestampMS / 1000, 1);
      const beforeEdit = await fetchAnalysisBundle(page, recordingID);
      expect(beforeEdit.report?.id).toBeTruthy();
      await page.getByRole("button", { name: "Спорно" }).click();
      await expect
        .poll(async () => {
          const body = await fetchAnalysisBundle(page, recordingID);
          const eventChanged = body.events
            ?.find((event) => event.id === targetEventID)
            ?.qualityFlags?.includes("AMBIGUOUS_BOUNDARY");
          return {
            eventChanged: eventChanged === true,
            reportChanged:
              Boolean(body.report?.id) &&
              body.report?.id !== beforeEdit.report?.id,
            reportRebuilt:
              body.report?.summary?.includes("QA rebuild") === true,
          };
        })
        .toEqual({
          eventChanged: true,
          reportChanged: true,
          reportRebuilt: true,
        });
      await assertQAResolveAndComplete(page, recordingID);

      await page.goto(`/runs?recordingId=${recordingID}`);
      await expect(page.locator("tbody tr")).toHaveCount(1);
      await expect(
        page.getByRole("columnheader", { name: "Стоимость" }),
      ).toBeVisible();
      await expect(page.locator("tbody tr").first()).toContainText("$");
      await expect(
        page.getByRole("button", { name: "Запустить повторно" }),
      ).toBeVisible();

      await page.goto(`/automation?recordingId=${recordingID}`);
      await expect(page.getByText("1 запись", { exact: true })).toBeVisible();

      await page.goto(`/reports?recordingId=${recordingID}`);
      await expect(page.getByText("Провайдер").locator("..")).toContainText(
        "gemini",
      );
      await expect(
        page.getByText("Версия промпта").locator(".."),
      ).toContainText("video-raw-extractor-v7");

      const timelineExport = await page.request.get(
        `${apiURL}/v1/recordings/${recordingID}/exports/timeline.json`,
      );
      expect(timelineExport.ok()).toBe(true);
      const timeline = (await timelineExport.json()) as unknown[];
      expect(timeline.length).toBeGreaterThan(0);

      const reportExport = await page.request.get(
        `${apiURL}/v1/recordings/${recordingID}/exports/report.json`,
      );
      expect(reportExport.ok()).toBe(true);
      const reportEnvelope = (await reportExport.json()) as {
        report?: {
          provider?: string;
          promptVersion?: string;
        };
        scenarioGroups?: { id: string }[];
      };
      expect(reportEnvelope.report?.provider).toBe("gemini");
      expect(reportEnvelope.report?.promptVersion).toBe(
        "video-raw-extractor-v7",
      );
      expect(reportEnvelope.scenarioGroups).toBeDefined();

      await waitForCorrelationLogs(page, correlationID);
      const evidenceResponse = await page.request.get(
        `${apiURL}/v1/recordings/${recordingID}/evidence/${timestampMS}`,
      );
      expect(evidenceResponse.ok()).toBe(true);
      expect(countS3ObjectsForRecording(recordingID)).toBeGreaterThan(1);

      await deleteRecordingAndAssertCleanup(page, recordingID);
      recordingID = undefined;
    } finally {
      if (recordingID) {
        await deleteRecordingAndAssertCleanup(page, recordingID);
      }
    }
  });
});

async function waitForCorrelationLogs(page: Page, correlationID: string) {
  await expect
    .poll(
      async () => {
        const response = await page.request.get(
          "http://localhost:3100/loki/api/v1/query_range",
          {
            params: {
              query: `{service=~"api|worker"} |= "${correlationID}"`,
              limit: "100",
            },
          },
        );
        if (!response.ok()) return "";
        const body = (await response.json()) as {
          data?: {
            result?: {
              stream?: { service?: string };
            }[];
          };
        };
        return [
          ...new Set(
            (body.data?.result ?? [])
              .map((result) => result.stream?.service)
              .filter((service): service is string => Boolean(service)),
          ),
        ]
          .sort()
          .join(",");
      },
      { timeout: 60_000 },
    )
    .toBe("api,worker");
}

async function deleteRecordingAndAssertCleanup(
  page: Page,
  recordingID: string,
) {
  const cleanup = await page.request.delete(
    `${apiURL}/v1/recordings/${recordingID}`,
  );
  expect(cleanup.ok()).toBe(true);

  await expect
    .poll(async () => {
      const response = await page.request.get(`${apiURL}/v1/recordings`);
      const body = (await response.json()) as {
        items: { id: string }[];
      };
      return body.items.some((item) => item.id === recordingID);
    })
    .toBe(false);

  await expect
    .poll(() => countS3ObjectsForRecording(recordingID), {
      timeout: 60_000,
      intervals: [2_000, 5_000, 10_000],
    })
    .toBe(0);
}

function countS3ObjectsForRecording(recordingID: string) {
  const output = execFileSync(
    "docker",
    [
      "compose",
      "run",
      "--rm",
      "--entrypoint",
      "/bin/sh",
      "minio-init",
      "-c",
      [
        "mc alias set local http://minio:9000 analyst analyst-secret >/dev/null",
        `mc ls --recursive local/analyst-recordings/recordings/${recordingID}/`,
      ].join(" && "),
    ],
    {
      cwd: projectRoot,
      encoding: "utf8",
      env: { ...process.env, COMPOSE_PROGRESS: "quiet" },
    },
  );
  return output.split(/\r?\n/).filter((line) => /\sSTANDARD\s/.test(line))
    .length;
}

async function assertEvidenceFragmentsWrap(page: Page) {
  const layout = await page
    .getByTestId("qa-evidence-fragments")
    .evaluate((container) => {
      const fragments = Array.from(
        container.querySelectorAll('[data-testid="qa-evidence-fragment"]'),
      );
      const rowTops = new Set(
        fragments.map((fragment) =>
          Math.round(fragment.getBoundingClientRect().top),
        ),
      );

      return {
        fragmentCount: fragments.length,
        flexWrap: getComputedStyle(container).flexWrap,
        rowCount: rowTops.size,
        pageHasHorizontalOverflow:
          document.documentElement.scrollWidth >
          document.documentElement.clientWidth,
        containerHasHorizontalOverflow:
          container.scrollWidth > container.clientWidth,
      };
    });

  expect(layout.flexWrap).toBe("wrap");
  expect(layout.pageHasHorizontalOverflow).toBe(false);
  expect(layout.containerHasHorizontalOverflow).toBe(false);
  if (layout.fragmentCount > 4) {
    expect(layout.rowCount).toBeGreaterThan(1);
  }
}

async function assertQAResolveAndComplete(page: Page, recordingID: string) {
  const before = await fetchAnalysisBundle(page, recordingID);
  const openIssue = before.dataQualityIssues.find((issue) => !issue.resolved);
  expect(
    openIssue,
    "real Gemini QA flow should expose at least one open quality issue",
  ).toBeDefined();

  const resolveResponse = await page.request.patch(
    `${apiURL}/v1/recordings/${recordingID}/quality-issues/${openIssue!.id}`,
    { data: { resolved: true } },
  );
  expect(resolveResponse.ok()).toBe(true);
  await expect
    .poll(async () => {
      const bundle = await fetchAnalysisBundle(page, recordingID);
      return bundle.dataQualityIssues.find(
        (issue) => issue.id === openIssue!.id,
      )?.resolved;
    })
    .toBe(true);

  const completeResponse = await page.request.post(
    `${apiURL}/v1/recordings/${recordingID}/qa/complete`,
    { data: { issueIds: [] } },
  );
  expect(completeResponse.ok()).toBe(true);
  const completed = (await completeResponse.json()) as AnalysisBundle;
  expect(completed.dataQualityIssues.length).toBeGreaterThan(0);
  expect(completed.dataQualityIssues.every((issue) => issue.resolved)).toBe(
    true,
  );
}

async function fetchAnalysisBundle(page: Page, recordingID: string) {
  const response = await page.request.get(
    `${apiURL}/v1/recordings/${recordingID}/analysis`,
  );
  expect(response.ok()).toBe(true);
  return (await response.json()) as AnalysisBundle;
}

type AnalysisBundle = {
  events?: {
    id: string;
    qualityFlags?: string[];
  }[];
  dataQualityIssues: {
    id: string;
    resolved: boolean;
  }[];
  report?: {
    id?: string;
    summary?: string;
  } | null;
};

async function uploadedRecordingIDByName(page: Page, originalName: string) {
  let recordingID: string | undefined;
  await expect
    .poll(
      async () => {
        const response = await page.request.get(`${apiURL}/v1/recordings`);
        const body = (await response.json()) as {
          items: { id: string; originalName: string; status: string }[];
        };
        const recording = body.items.find(
          (item) => item.originalName === originalName,
        );
        recordingID = recording?.id;
        return recording?.status;
      },
      { timeout: 60_000 },
    )
    .toBe("UPLOADED");
  return recordingID;
}

async function waitForCompletedRun(page: Page, recordingID: string) {
  let completed:
    | {
        status: string;
        provider: string;
        model: string | null;
        promptVersion: string;
        error?: string | null;
      }
    | undefined;
  await expect
    .poll(
      async () => {
        const response = await page.request.get(
          `${apiURL}/v1/analysis-runs?recordingId=${recordingID}`,
        );
        const body = (await response.json()) as {
          items: {
            status: string;
            provider: string;
            model: string | null;
            promptVersion: string;
            error?: string | null;
          }[];
        };
        completed = body.items[0];
        if (completed?.status === "FAILED") {
          throw new Error(completed.error ?? "Real Gemini analysis failed");
        }
        return completed?.status;
      },
      { timeout: 8 * 60_000 },
    )
    .toBe("COMPLETED");

  if (!completed) throw new Error("Analysis run was not created");
  return completed;
}
