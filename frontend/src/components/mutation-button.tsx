"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

const apiUrl =
  process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";

export function MutationButton({
  path,
  method = "POST",
  body,
  children,
  tone = "neutral",
}: {
  path: string;
  method?: "POST" | "PATCH" | "DELETE";
  body?: Record<string, unknown>;
  children: React.ReactNode;
  tone?: "neutral" | "accent" | "danger";
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const color =
    tone === "accent"
      ? "border-accent text-accent"
      : tone === "danger"
        ? "border-danger text-danger"
        : "border-line text-muted";
  return (
    <button
      type="button"
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        await fetch(`${apiUrl}${path}`, {
          method,
          headers: body ? { "Content-Type": "application/json" } : undefined,
          body: body ? JSON.stringify(body) : undefined,
        });
        setBusy(false);
        router.refresh();
      }}
      className={`border px-3 py-1.5 text-[10px] uppercase disabled:opacity-40 ${color}`}
    >
      {busy ? "Выполняется…" : children}
    </button>
  );
}
