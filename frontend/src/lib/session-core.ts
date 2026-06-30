import { createHmac, timingSafeEqual } from "node:crypto";

export const sessionCookieName = "app_session";
const sessionLifetimeSeconds = 60 * 60 * 24 * 7;

export type AppSession = {
  subject: string;
  expiresAt: number;
};

function sessionSecret() {
  return process.env.AUTH_SESSION_SECRET ?? process.env.API_SHARED_SECRET ?? "";
}

function signature(payload: string) {
  return createHmac("sha256", sessionSecret()).update(payload).digest("base64url");
}

export function localAuthEnabled() {
  return Boolean(
    process.env.DEMO_USERNAME && process.env.DEMO_PASSWORD && sessionSecret(),
  );
}

export function createSession(subject: string): string {
  const payload = Buffer.from(
    JSON.stringify({
      subject,
      expiresAt: Math.floor(Date.now() / 1000) + sessionLifetimeSeconds,
    } satisfies AppSession),
  ).toString("base64url");
  return `${payload}.${signature(payload)}`;
}

export function verifySession(value?: string): AppSession | null {
  if (!value || !sessionSecret()) return null;
  const [payload, providedSignature, extra] = value.split(".");
  if (!payload || !providedSignature || extra) return null;
  const expectedSignature = signature(payload);
  const provided = Buffer.from(providedSignature);
  const expected = Buffer.from(expectedSignature);
  if (
    provided.length !== expected.length ||
    !timingSafeEqual(provided, expected)
  ) {
    return null;
  }
  try {
    const session = JSON.parse(
      Buffer.from(payload, "base64url").toString("utf8"),
    ) as AppSession;
    if (
      !session.subject ||
      !session.expiresAt ||
      session.expiresAt <= Math.floor(Date.now() / 1000)
    ) {
      return null;
    }
    return session;
  } catch {
    return null;
  }
}

export const sessionCookieOptions = {
  httpOnly: true,
  sameSite: "lax" as const,
  secure: process.env.NODE_ENV === "production",
  path: "/",
  maxAge: sessionLifetimeSeconds,
};
