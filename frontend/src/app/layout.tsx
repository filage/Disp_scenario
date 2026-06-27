import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "DispScenario Analyst",
  description: "Video-first dispatcher scenario analytics",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body className="antialiased">{children}</body>
    </html>
  );
}
