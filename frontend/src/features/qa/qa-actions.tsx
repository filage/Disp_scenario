"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";

export function ResolveIssue({
  recordingId,
  issueId,
  resolved,
}: {
  recordingId: string;
  issueId: string;
  resolved: boolean;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  return (
    <button
      type="button"
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        await fetch(
          `${apiUrl}/v1/recordings/${recordingId}/quality-issues/${issueId}`,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ resolved: !resolved }),
          },
        );
        setBusy(false);
        router.refresh();
      }}
      className="border border-line px-3 py-1.5 text-[10px] uppercase text-muted hover:text-accent"
    >
      {resolved ? "Открыть снова" : "Закрыть"}
    </button>
  );
}

export function CompleteQA({
  recordingId,
  openIssueCount,
}: {
  recordingId: string;
  openIssueCount: number;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  return (
    <button
      type="button"
      disabled={busy || openIssueCount === 0}
      onClick={async () => {
        setBusy(true);
        await fetch(`${apiUrl}/v1/recordings/${recordingId}/qa/complete`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        });
        setBusy(false);
        router.refresh();
      }}
      className="border border-accent px-4 py-2 text-xs font-medium text-accent hover:bg-accent-soft disabled:cursor-not-allowed disabled:opacity-50"
    >
      {busy
        ? "Закрываем замечания…"
        : openIssueCount > 0
          ? `Закрыть все замечания (${openIssueCount})`
          : "Замечания закрыты"}
    </button>
  );
}
