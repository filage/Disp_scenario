import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  localAuthEnabled,
  sessionCookieName,
  verifySession,
} from "@/lib/session-core";

export function proxy(request: NextRequest) {
  if (!localAuthEnabled()) return NextResponse.next();
  const session = verifySession(request.cookies.get(sessionCookieName)?.value);
  if (session || request.cookies.get("id_token")?.value) return NextResponse.next();
  if (request.nextUrl.pathname.startsWith("/api/backend")) {
    return NextResponse.json({ error: "authentication required" }, { status: 401 });
  }
  return NextResponse.redirect(new URL("/login", request.url));
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|robots.txt|login|api/auth|api/session|api/health|api/backend-health).*)",
  ],
};
