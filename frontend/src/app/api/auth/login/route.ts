import { createHash, randomBytes } from "node:crypto";
import { NextResponse } from "next/server";
import { oidcConfiguration, secureAuthCookies } from "@/lib/oidc";

function base64Url(value: Buffer): string {
  return value.toString("base64url");
}

export async function GET() {
  const configuration = await oidcConfiguration();
  const state = base64Url(randomBytes(24));
  const nonce = base64Url(randomBytes(24));
  const verifier = base64Url(randomBytes(48));
  const challenge = base64Url(createHash("sha256").update(verifier).digest());
  const appURL = process.env.APP_URL ?? "http://localhost:3000";
  const parameters = new URLSearchParams({
    client_id: process.env.OIDC_CLIENT_ID ?? "",
    response_type: "code",
    scope: "openid profile email",
    redirect_uri: `${appURL}/api/auth/callback`,
    state,
    nonce,
    code_challenge: challenge,
    code_challenge_method: "S256",
  });
  const response = NextResponse.redirect(
    `${configuration.authorization_endpoint}?${parameters}`,
  );
  const options = {
    httpOnly: true,
    sameSite: "lax" as const,
    secure: secureAuthCookies(),
    path: "/",
    maxAge: 600,
  };
  response.cookies.set("oidc_state", state, options);
  response.cookies.set("oidc_nonce", nonce, options);
  response.cookies.set("oidc_verifier", verifier, options);
  return response;
}
