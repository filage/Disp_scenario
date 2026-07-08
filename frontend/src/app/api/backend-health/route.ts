import { backendApiUrl } from "@/lib/backend-fetch";

function localApiConfigured() {
  const required = [
    "DATABASE_URL",
    "S3_ENDPOINT",
    "S3_PUBLIC_ENDPOINT",
    "S3_ACCESS_KEY",
    "S3_SECRET_KEY",
    "S3_BUCKET",
    "S3_REGION",
  ];
  const hasRequired = required.every((name) => Boolean(process.env[name]));
  const hasCredentialSecret = Boolean(
    process.env.CREDENTIALS_ENCRYPTION_KEY ||
      process.env.API_SHARED_SECRET ||
      process.env.GEMINI_API_KEY,
  );
  return hasRequired && hasCredentialSecret;
}

const diagnostics = {
  backendMode: process.env.INTERNAL_API_URL ? "local" : "external",
  localApiConfigured: localApiConfigured(),
  renderCommit: process.env.RENDER_GIT_COMMIT?.slice(0, 7) ?? null,
};

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
        ...diagnostics,
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
        ...diagnostics,
        backendApiUrl,
        status: 0,
        elapsedMs: Date.now() - started,
        error: error instanceof Error ? error.message : String(error),
      },
      { status: 502 },
    );
  }
}
