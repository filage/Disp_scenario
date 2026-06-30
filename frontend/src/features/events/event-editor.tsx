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
}: {
  recordingId: string;
  event: ActionEvent;
  rawEvents: RawEvent[];
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
  const fieldClass =
    "border border-line bg-background px-3 py-2 text-sm text-foreground";

  async function save(extra: Record<string, unknown>, includeDraft = true) {
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
    <aside className="border border-line bg-panel-raised p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-medium text-accent">
            Проверка выбранного события
          </p>
          <p className="mt-2 font-mono text-xs text-muted">
            {formatClock(event.timestampMs)} · v{event.version}
          </p>
        </div>
        <span className="text-xs text-muted">
          {formatQAStatus(event.qaStatus)}
        </span>
      </div>

      <div className="mt-5 grid gap-4">
        <label className="grid gap-2 text-xs text-muted">
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
        <label className="grid gap-2 text-xs text-muted">
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
        <label className="grid gap-2 text-xs text-muted">
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
        <label className="grid gap-2 text-xs text-muted">
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

      <p className="mt-5 text-xs leading-5 text-muted">
        Подтвердите корректное событие, сохраните исправленный эталон для
        будущей оценки модели либо пометьте неоднозначную границу или шум.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() => save({ qaStatus: "confirmed" })}
          className="border border-accent px-3 py-2 text-xs font-medium text-accent hover:bg-accent-soft disabled:opacity-50"
        >
          Подтвердить событие
        </button>
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() => save({ qaStatus: "edited", saveGroundTruth: true })}
          className="border border-warning px-3 py-2 text-xs font-medium text-warning disabled:opacity-50"
        >
          Исправить и сохранить эталон
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
          className="border border-warning px-3 py-2 text-xs font-medium text-warning disabled:opacity-50"
        >
          Граница неоднозначна
        </button>
        <button
          type="button"
          disabled={state === "saving"}
          onClick={() =>
            save(
              {
                qaStatus: "ignored",
                qualityFlags: [
                  ...new Set([...(event.qualityFlags ?? []), "IGNORED_NOISE"]),
                ],
              },
              false,
            )
          }
          className="border border-danger px-3 py-2 text-xs font-medium text-danger disabled:opacity-50"
        >
          Отметить как шум
        </button>
      </div>
      {state === "saved" ? (
        <p className="mt-3 text-xs text-accent">Сохранено и пересобрано.</p>
      ) : null}
      {state === "error" ? (
        <p className="mt-3 text-xs text-danger">
          Не удалось сохранить изменения.
        </p>
      ) : null}

      <div className="mt-6 border-t border-line pt-4">
        <p className="text-xs font-medium text-muted">
          Что увидела модель в исходном видео
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
            <p className="text-xs text-muted">
              Связанных сырых наблюдений нет.
            </p>
          )}
        </div>
      </div>
    </aside>
  );
}
