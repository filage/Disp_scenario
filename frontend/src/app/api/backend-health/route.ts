import { backendApiUrl } from "@/lib/backend-fetch";

export async function GET() {
  const started = Date.now();
  try {
    const response = await fetch(`${backendApiUrl}/health`, {
      cache: "no-store",
      signal: AbortSignal.timeout(30_000),
      headers: { Accept: "application/json" },
    });
    const contentType = response.headers.get("content-type") ?? "";
    const body = contentType.includes("application/json")
      ? await response.json().catch(() => null)
      : await response.text().catch(() => null);
    return Response.json(
      {
        backendApiUrl,
        status: response.status,
        elapsedMs: Date.now() - started,
        contentType,
        body,
      },
      { status: response.ok ? 200 : 502 },
    );
  } catch (error) {
    return Response.json(
      {
        backendApiUrl,
        status: 0,
        elapsedMs: Date.now() - started,
        error: error instanceof Error ? error.message : String(error),
      },
      { status: 502 },
    );
  }
}
