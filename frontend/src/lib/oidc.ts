import "server-only";

type OIDCConfiguration = {
  authorization_endpoint: string;
  token_endpoint: string;
  end_session_endpoint?: string;
};

export async function oidcConfiguration(): Promise<OIDCConfiguration> {
  const issuer = process.env.OIDC_ISSUER?.replace(/\/$/, "");
  if (!issuer) throw new Error("OIDC_ISSUER is not configured");
  const response = await fetch(
    `${issuer}/.well-known/openid-configuration`,
    { next: { revalidate: 3600 } },
  );
  if (!response.ok) throw new Error("OIDC discovery failed");
  return response.json() as Promise<OIDCConfiguration>;
}

export function oidcEnabled(): boolean {
  return Boolean(process.env.OIDC_ISSUER && process.env.OIDC_CLIENT_ID);
}

export function secureAuthCookies(): boolean {
  const appURL = process.env.APP_URL ?? "http://localhost:3000";
  return appURL.startsWith("https://");
}
