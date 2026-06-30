import { timingSafeEqual } from "node:crypto";
import { NextResponse } from "next/server";
import {
  createSession,
  sessionCookieName,
  sessionCookieOptions,
} from "@/lib/session-core";

function matches(left: string, right: string) {
  const leftBytes = Buffer.from(left);
  const rightBytes = Buffer.from(right);
  return (
    leftBytes.length === rightBytes.length &&
    timingSafeEqual(leftBytes, rightBytes)
  );
}

export async function POST(request: Request) {
  const expectedUsername = process.env.DEMO_USERNAME;
  const expectedPassword = process.env.DEMO_PASSWORD;
  if (!expectedUsername || !expectedPassword) {
    return NextResponse.json(
      { error: "Вход по логину не настроен." },
      { status: 503 },
    );
  }
  let input: { username?: string; password?: string };
  try {
    input = (await request.json()) as typeof input;
  } catch {
    return NextResponse.json({ error: "Некорректный запрос." }, { status: 400 });
  }
  if (
    !matches(input.username ?? "", expectedUsername) ||
    !matches(input.password ?? "", expectedPassword)
  ) {
    return NextResponse.json(
      { error: "Неверный логин или пароль." },
      { status: 401 },
    );
  }
  const response = NextResponse.json({ ok: true });
  response.cookies.set(
    sessionCookieName,
    createSession(expectedUsername),
    sessionCookieOptions,
  );
  return response;
}
