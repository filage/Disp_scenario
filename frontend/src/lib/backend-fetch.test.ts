import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

describe("backend cold-start handling", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("waits for health before sending a mutation exactly once", async () => {
    vi.useFakeTimers();
    vi.stubEnv("API_URL", "http://api.test");
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("", { status: 503 }))
      .mockResolvedValueOnce(new Response('{"status":"ok"}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"configured":true}', { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const { fetchBackend } = await import("./backend-fetch");

    const request = fetchBackend("/v1/settings/gemini-credential", {
      method: "PUT",
      body: JSON.stringify({ apiKey: "test-key" }),
    });
    await vi.runAllTimersAsync();
    const response = await request;

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "PUT")).toHaveLength(1);
  });

  it("retries a read after a transient gateway response", async () => {
    vi.useFakeTimers();
    vi.stubEnv("API_URL", "http://api.test");
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response('{"status":"ok"}', { status: 200 }))
      .mockResolvedValueOnce(new Response("", { status: 502 }))
      .mockResolvedValueOnce(new Response('{"status":"ok"}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"items":[1,2]}', { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const { fetchBackend } = await import("./backend-fetch");

    const request = fetchBackend("/v1/recordings");
    await vi.runAllTimersAsync();
    const response = await request;

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("does not block requests when health reports a degraded live backend", async () => {
    vi.useFakeTimers();
    vi.stubEnv("API_URL", "http://api.test");
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        Response.json(
          { status: "degraded", dependencies: { s3: "error" } },
          { status: 503 },
        ),
      )
      .mockResolvedValueOnce(new Response('{"configured":true}', { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const { fetchBackend } = await import("./backend-fetch");

    const response = await fetchBackend("/v1/settings/gemini-credential", {
      method: "PUT",
      body: JSON.stringify({ apiKey: "test-key" }),
    });

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls.at(1)?.[0]).toBe(
      "http://api.test/v1/settings/gemini-credential",
    );
  });

  it("does not use a relative public API URL for server-side backend fetches", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_URL", "/api/backend");
    vi.stubEnv("RENDER_EXTERNAL_HOSTNAME", "disp-scenario-web.onrender.com");
    const { backendApiUrl } = await import("./backend-fetch");

    expect(backendApiUrl).toBe("https://disp-scenario-api.onrender.com");
  });

  it("prefers the Render sibling API URL over a stale runtime API_URL", async () => {
    vi.stubEnv("API_URL", "http://localhost:8787");
    vi.stubEnv("RENDER_EXTERNAL_HOSTNAME", "disp-scenario-web.onrender.com");
    const { backendApiUrl } = await import("./backend-fetch");

    expect(backendApiUrl).toBe("https://disp-scenario-api.onrender.com");
  });
});
