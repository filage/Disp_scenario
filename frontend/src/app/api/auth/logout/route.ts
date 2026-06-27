import { NextResponse } from "next/server";

export async function GET() {
  const response = NextResponse.redirect(
    `${process.env.APP_URL ?? "http://localhost:3000"}/overview`,
  );
  response.cookies.delete("id_token");
  return response;
}
