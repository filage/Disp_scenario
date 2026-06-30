import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import {
  localAuthEnabled,
  sessionCookieName,
  verifySession,
  type AppSession,
} from "@/lib/session-core";

export async function currentSession(): Promise<AppSession | null> {
  const store = await cookies();
  const local = verifySession(store.get(sessionCookieName)?.value);
  if (local) return local;
  const idToken = store.get("id_token")?.value;
  if (idToken) {
    try {
      const [, payload] = idToken.split(".");
      const claims = JSON.parse(
        Buffer.from(payload, "base64url").toString("utf8"),
      ) as { sub?: string; exp?: number };
      if (
        claims.sub &&
        (!claims.exp || claims.exp > Math.floor(Date.now() / 1000))
      ) {
        return {
          subject: claims.sub,
          expiresAt: claims.exp ?? Number.MAX_SAFE_INTEGER,
        };
      }
    } catch {
      return null;
    }
  }
  if (!localAuthEnabled() && !process.env.OIDC_ISSUER) {
    return { subject: "local-user", expiresAt: Number.MAX_SAFE_INTEGER };
  }
  return null;
}

export async function requireSession() {
  const session = await currentSession();
  if (!session) redirect("/login");
  return session;
}
