import "server-only";

export const backendApiUrl =
  process.env.API_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  "http://localhost:8787";

const wakeDelaysMS = [0, 1_000, 2_000, 3_000, 4_000, 5_000, 5_000];
let readyUntil = 0;

function sleep(milliseconds: number) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

export async function ensureBackendReady() {
  if (Date.now() < readyUntil) return;
  let lastError: unknown;
  for (let attempt = 0; attempt < wakeDelaysMS.length; attempt += 1) {
    if (wakeDelaysMS[attempt]) await sleep(wakeDelaysMS[attempt]);
    try {
      const response = await fetch(`${backendApiUrl}/health`, {
        cache: "no-store",
        signal: AbortSignal.timeout(10_000),
        headers: { Accept: "application/json" },
      });
      if (response.ok) {
        await response.body?.cancel();
        readyUntil = Date.now() + 30_000;
        return;
      }
      await response.body?.cancel();
      lastError = new Error(`health returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
  }
  console.error("[backend-ready] API did not become ready", {
    error: String(lastError),
  });
  throw new Error("backend is unavailable", { cause: lastError });
}

export async function fetchBackend(path: string, init: RequestInit = {}) {
  await ensureBackendReady();
  const method = (init.method ?? "GET").toUpperCase();
  const attempts = method === "GET" || method === "HEAD" ? 3 : 1;
  let lastError: unknown;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      const response = await fetch(`${backendApiUrl}${path}`, {
        ...init,
        cache: "no-store",
        signal: AbortSignal.timeout(30_000),
      });
      if (![502, 503, 504].includes(response.status) || attempt === attempts - 1) {
        return response;
      }
      await response.body?.cancel();
      lastError = new Error(`backend returned ${response.status}`);
    } catch (error) {
      lastError = error;
      if (attempt === attempts - 1) break;
    }
    readyUntil = 0;
    await sleep(1_000 * (attempt + 1));
    await ensureBackendReady();
  }
  throw new Error("backend request failed", { cause: lastError });
}
