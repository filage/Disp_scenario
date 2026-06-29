import { timingSafeEqual } from "node:crypto";
import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

function matches(left: string, right: string) {
  const leftBytes = Buffer.from(left);
  const rightBytes = Buffer.from(right);
  return (
    leftBytes.length === rightBytes.length &&
    timingSafeEqual(leftBytes, rightBytes)
  );
}

export function proxy(request: NextRequest) {
  const username = process.env.DEMO_USERNAME;
  const password = process.env.DEMO_PASSWORD;
  if (!username || !password) return NextResponse.next();

  const expected = `Basic ${Buffer.from(`${username}:${password}`).toString("base64")}`;
  const provided = request.headers.get("authorization") ?? "";
  if (matches(provided, expected)) return NextResponse.next();

  return NextResponse.rewrite(new URL("/api/auth-required", request.url));
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|robots.txt|api/auth-required|api/health).*)",
  ],
};
