"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  ACTION_OPTIONS,
  type ActionEvent,
  type RawEvent,
} from "@/features/events/types";
import { formatClock, formatQAStatus } from "@/lib/display";

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";

export function EventEditor({
  recordingId,
  event,
  rawEvents,
  variant = "light",
}: {
  recordingId: string;
  event: ActionEvent;
  rawEvents: RawEvent[];
  variant?: "light" | "dark";
}) {
  const router = useRouter();
  const [draft, setDraft] = useState({
    canonicalAction: event.canonicalAction,
    target: event.target,
    issueType: event.issueType ?? "",
    qaComment: event.qaComment ?? "",
  });
  const [state, setState] = useState<"idle" | "saving" | "saved" | "error">(
    "idle",
  );
  const relatedRaw = rawEvents.filter(
    (raw) =>
      event.sourceRawEventIds?.includes(raw.id) ||
      Math.abs(raw.timestampMs - event.timestampMs) <= 1500,
  );
  const dark = variant === "dark";
  const rawDisplay = relatedRaw[0] ?? {
    timestampMs: event.timestampMs,
    screen: event.screen,
    target: event.target,
    canonicalAction: event.canonicalAction,
    issueType: event.issueType,
    sourceRawEventIds: event.sourceRawEventIds,
    qualityFlags: event.qualityFlags,
  };
  const fieldClass = dark
    ? "border border-[#475569] bg-[#0f172a] px-3 py-2 text-sm text-white"
    : "border border-line bg-background px-3 py-2 text-sm text-foreground";

  async function save(
    extra: Record<string, unknown>,
    includeDraft = true,
  ) {
    setState("saving");
    const response = await fetch(
      `${apiURL}/v1/recordings/${recordingId}/events/${event.id}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...(includeDraft ? draft : {}),
          ...extra,
        }),
      },
    );
    if (!response.ok) {
      setState("error");
      return;
    }
    setState("saved");
    router.refresh();
  }

  return (
    <aside
      className={
        dark
          ? "grid min-h-[42rem] grid-rows-[auto_minmax(10rem,1fr)_auto_auto] border border-[#334155] bg-[#1e293b] text-[#f8fafc]"
          : "border border-line bg-panel-raised p-5"
      }
    >
      <div
        className={
          dark
            ? "flex items-start justify-between gap-4 border-b border-[#334155] px-4 py-3"
            : "flex items-start justify-between gap-4"
        }
      >
        <div>
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-accent">
            {dark ? "Сырые данные Gemini" : "Инспектор события"}
          </p>
          <p className="mt-2 font-mono text-xs text-muted">
            {formatClock(event.timestampMs)} · v{event.version}
          </p>
        </div>
        <span className="font-mono text-[10px] uppercase text-muted">
          {formatQAStatus(event.qaStatus)}
        </span>
      </div>

      {dark ? (
        <pre className="max-h-[22rem] overflow-auto whitespace-pre-wrap p-4 font-mono text-[11px] leading-5 text-[#dbeafe]">
          {JSON.stringify(rawDisplay, null, 2)}
        </pre>
      ) : null}

      <div
        className={
          dark
            ? "grid gap-3 border-t border-[#334155] bg-[#111827] p-4 md:grid-cols-2"
            : "mt-5 grid gap-4"
        }
      >
        <label className={`grid gap-2 text-xs ${dark ? "text-[#cbd5e1]" : "text-muted"}`}>
          Каноническое действие
          <select
            value={draft.canonicalAction}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                canonicalAction: change.target.value,
              }))
            }
            className={fieldClass}
          >
            {ACTION_OPTIONS.map((action) => (
              <option key={action}>{action}</option>
            ))}
          </select>
        </label>
        <label className={`grid gap-2 text-xs ${dark ? "text-[#cbd5e1]" : "text-muted"}`}>
          Цель
          <input
            value={draft.target}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                target: change.target.value,
              }))
            }
            className={fieldClass}
          />
        </label>
        <label className={`grid gap-2 text-xs ${dark ? "text-[#cbd5e1]" : "text-muted"}`}>
          Тип проблемы
          <input
            value={draft.issueType}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                issueType: change.target.value,
              }))
            }
            className={fieldClass}
          />
        </label>
        <label className={`grid gap-2 text-xs ${dark ? "text-[#cbd5e1] md:col-span-2" : "text-muted"}`}>
          Комментарий QA
          <textarea
            value={draft.qaComment}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                qaComment: change.target.value,
              }))
            }
            rows={3}
            className={`resize-y ${fieldClass}`}
          />
        </label>
      </div>

      <div
        className={
          dark
            ? "flex flex-wrap gap-2 border-t border-[#334155] bg-[#111827] p-4"
            : "mt-5 flex flex-wrap gap-2"
        }
      >
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() => save({ qaStatus: "confirmed" })}
          className={
            dark
              ? "border border-[#475569] bg-[#15803d] px-3 py-2 text-[10px] uppercase text-white disabled:opacity-50"
              : "border border-accent px-3 py-2 text-[10px] uppercase text-accent disabled:opacity-50"
          }
        >
          Подтвердить
        </button>
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() =>
            save({ qaStatus: "edited", saveGroundTruth: true })
          }
          className="border border-warning px-3 py-2 text-[10px] uppercase text-warning disabled:opacity-50"
        >
          Сохранить эталон
        </button>
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() =>
            save(
              {
                qaStatus: "edited",
                qualityFlags: [
                  ...new Set([
                    ...(event.qualityFlags ?? []),
                    "AMBIGUOUS_BOUNDARY",
                  ]),
                ],
              },
              false,
            )
          }
          className="border border-warning px-3 py-2 text-[10px] uppercase text-warning disabled:opacity-50"
        >
          Спорно
        </button>
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() =>
            save(
              {
                qaStatus: "ignored",
                qualityFlags: [
                  ...new Set([
                    ...(event.qualityFlags ?? []),
                    "IGNORED_NOISE",
                  ]),
                ],
              },
              false,
            )
          }
          className="border border-danger px-3 py-2 text-[10px] uppercase text-danger disabled:opacity-50"
        >
          Отметить как шум
        </button>
      </div>
      {state === "saved" ? (
        <p className="mt-3 text-xs text-accent">Сохранено и пересобрано.</p>
      ) : null}
      {state === "error" ? (
        <p className="mt-3 text-xs text-danger">Не удалось сохранить изменения.</p>
      ) : null}

      {!dark ? <div className="mt-6 border-t border-line pt-4">
        <p className="font-mono text-[10px] uppercase text-muted">
          Исходные наблюдения
        </p>
        <div className="mt-3 grid gap-2">
          {relatedRaw.length ? (
            relatedRaw.map((raw) => (
              <article key={raw.id} className="border-l border-line pl-3">
                <p className="text-xs text-foreground">
                  {raw.visibleText ?? raw.target ?? raw.eventTypeGuess}
                </p>
                <p className="mt-1 text-[11px] leading-5 text-muted">
                  {raw.stateChange ?? raw.screen}
                </p>
              </article>
            ))
          ) : (
            <p className="text-xs text-muted">Связанных сырых наблюдений нет.</p>
          )}
        </div>
      </div> : null}
    </aside>
  );
}
