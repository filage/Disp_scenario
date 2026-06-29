export function GET() {
  return new Response("Authentication required", {
    status: 401,
    headers: {
      "WWW-Authenticate": 'Basic realm="DispScenario", charset="UTF-8"',
      "Cache-Control": "no-store",
    },
  });
}
