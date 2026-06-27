import { NextRequest, NextResponse } from "next/server";
import { oidcConfiguration, secureAuthCookies } from "@/lib/oidc";

export async function GET(request: NextRequest) {
  const code = request.nextUrl.searchParams.get("code");
  const state = request.nextUrl.searchParams.get("state");
  const expectedState = request.cookies.get("oidc_state")?.value;
  const verifier = request.cookies.get("oidc_verifier")?.value;
  const expectedNonce = request.cookies.get("oidc_nonce")?.value;
  if (!code || !state || state !== expectedState || !verifier || !expectedNonce) {
    return NextResponse.json(
      { error: "invalid OIDC callback state" },
      { status: 400 },
    );
  }
  const configuration = await oidcConfiguration();
  const appURL = process.env.APP_URL ?? "http://localhost:3000";
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    client_id: process.env.OIDC_CLIENT_ID ?? "",
    redirect_uri: `${appURL}/api/auth/callback`,
    code_verifier: verifier,
  });
  if (process.env.OIDC_CLIENT_SECRET) {
    body.set("client_secret", process.env.OIDC_CLIENT_SECRET);
  }
  const tokenResponse = await fetch(configuration.token_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
    cache: "no-store",
  });
  if (!tokenResponse.ok) {
    return NextResponse.json(
      { error: "OIDC token exchange failed" },
      { status: 502 },
    );
  }
  const tokens = (await tokenResponse.json()) as {
    id_token?: string;
    expires_in?: number;
  };
  if (!tokens.id_token) {
    return NextResponse.json(
      { error: "OIDC provider returned no id_token" },
      { status: 502 },
    );
  }
  const tokenParts = tokens.id_token.split(".");
  if (tokenParts.length !== 3) {
    return NextResponse.json({ error: "invalid id_token" }, { status: 502 });
  }
  let claims: { nonce?: string };
  try {
    claims = JSON.parse(
      Buffer.from(tokenParts[1], "base64url").toString("utf8"),
    ) as { nonce?: string };
  } catch {
    return NextResponse.json({ error: "invalid id_token" }, { status: 502 });
  }
  if (claims.nonce !== expectedNonce) {
    return NextResponse.json(
      { error: "OIDC nonce validation failed" },
      { status: 400 },
    );
  }
  const response = NextResponse.redirect(`${appURL}/overview`);
  response.cookies.set("id_token", tokens.id_token, {
    httpOnly: true,
    sameSite: "lax",
    secure: secureAuthCookies(),
    path: "/",
    maxAge: tokens.expires_in ?? 3600,
  });
  for (const name of ["oidc_state", "oidc_nonce", "oidc_verifier"]) {
    response.cookies.delete(name);
  }
  return response;
}
