import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { fetchBackend } from "@/lib/backend-fetch";
import { currentSession } from "@/lib/session";

async function proxy(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> },
) {
  const session = await currentSession();
  if (!session) {
    return NextResponse.json({ error: "authentication required" }, { status: 401 });
  }
  const { path } = await context.params;
  const query = request.nextUrl.search;
  const token = (await cookies()).get("id_token")?.value;
  const headers = new Headers(request.headers);
  headers.delete("host");
  headers.delete("content-length");
  headers.delete("cookie");
  headers.delete("authorization");
  headers.delete("x-api-shared-secret");
  headers.delete("x-app-user");
  const sharedSecret = process.env.API_SHARED_SECRET;
  if (sharedSecret) headers.set("X-API-Shared-Secret", sharedSecret);
  headers.set("X-App-User", session.subject);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const hasBody = !["GET", "HEAD"].includes(request.method);
  let response: Response;
  try {
    response = await fetchBackend(`/${path.join("/")}${query}`, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      redirect: "manual",
    });
  } catch (error) {
    console.error("[backend-proxy] request failed", {
      method: request.method,
      path: `/${path.join("/")}`,
      error: String(error),
    });
    return NextResponse.json(
      {
        error: {
          code: "API_WAKING",
          message: "Основной API запускается. Повторите запрос через несколько секунд.",
        },
      },
      { status: 503 },
    );
  }
  const responseHeaders = new Headers();
  for (const name of [
    "content-type",
    "content-disposition",
    "cache-control",
    "etag",
    "location",
    "x-correlation-id",
  ]) {
    const value = response.headers.get(name);
    if (value) responseHeaders.set(name, value);
  }
  return new NextResponse(response.body, {
    status: response.status,
    headers: responseHeaders,
  });
}

export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
export const PUT = proxy;
export const DELETE = proxy;
