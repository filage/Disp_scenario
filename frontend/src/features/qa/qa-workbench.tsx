"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { MutationButton } from "@/components/mutation-button";
import { EventEditor } from "@/features/events/event-editor";
import {
  ScenarioBoundaryDivider,
  createScenarioBoundaryLookup,
} from "@/features/events/scenario-boundaries";
import type {
  ActionEvent,
  QualityIssue,
  RawEvent,
  ScenarioInstance,
} from "@/features/events/types";
import { CompleteQA, ResolveIssue } from "@/features/qa/qa-actions";
import {
  formatClock,
  formatQualityIssueMessage,
  formatQualityIssueType,
} from "@/lib/display";

export function QAWorkbench({
  recordingId,
  events,
  rawEvents,
  issues,
  scenarioInstances = [],
  runId,
  playbackHref,
  evidenceBaseHref,
}: {
  recordingId: string;
  events: ActionEvent[];
  rawEvents: RawEvent[];
  issues: QualityIssue[];
  scenarioInstances?: ScenarioInstance[];
  runId?: string;
  playbackHref: string;
  evidenceBaseHref: string;
}) {
  const playerRef = useRef<HTMLVideoElement>(null);
  const [selectedId, setSelectedId] = useState(
    issues.find((issue) => !issue.resolved)?.actionEventId ??
      events[0]?.id ??
      "",
  );
  const selected =
    events.find((event) => event.id === selectedId) ?? events[0] ?? null;
  const selectedTimestampMs = selected?.timestampMs;
  const openIssues = issues.filter((issue) => !issue.resolved);
  const scenarioBoundaries = useMemo(
    () => createScenarioBoundaryLookup(events, scenarioInstances),
    [events, scenarioInstances],
  );

  useEffect(() => {
    const player = playerRef.current;
    if (!player || selectedTimestampMs === undefined) return;
    const targetTime = Math.max(0, selectedTimestampMs / 1000);
    const seek = () => {
      player.currentTime = targetTime;
    };
    if (player.readyState >= HTMLMediaElement.HAVE_METADATA) {
      seek();
      return;
    }
    player.addEventListener("loadedmetadata", seek, { once: true });
    return () => player.removeEventListener("loadedmetadata", seek);
  }, [selectedId, selectedTimestampMs]);

  return (
    <div className="mt-6 grid gap-4">
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-4 border border-line bg-panel p-4">
        <div className="min-w-0">
          <p className="text-xs font-medium text-accent">
            Разбор сессии{runId ? `: ${runId.slice(0, 8)}` : ""}
          </p>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-muted">
            Сверьте событие с видео и кадром, исправьте поля при необходимости,
            затем подтвердите результат. Открытых замечаний: {openIssues.length}
            .
          </p>
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Link
            href={`/runs?recordingId=${recordingId}`}
            className="border border-line px-3 py-2 text-xs text-muted transition-colors hover:border-accent hover:text-accent active:translate-y-px"
          >
            История запусков
          </Link>
          <CompleteQA
            recordingId={recordingId}
            openIssueCount={openIssues.length}
          />
        </div>
      </div>

      <details className="border border-line bg-panel px-4 py-3">
        <summary className="cursor-pointer text-xs font-medium text-muted hover:text-foreground">
          Сервисные операции анализа
        </summary>
        <p className="mt-3 max-w-3xl text-xs leading-5 text-muted">
          Эти действия нужны при изменении правил или диагностике. Для обычной
          проверки записи их запускать не требуется.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <MutationButton path={`/v1/recordings/${recordingId}/rebuild`}>
            Пересобрать отчёты из текущих событий
          </MutationButton>
          <MutationButton path={`/v1/recordings/${recordingId}/renormalize`}>
            Повторить нормализацию raw-событий
          </MutationButton>
          <MutationButton
            path={`/v1/recordings/${recordingId}/boundary-review`}
          >
            Сверить границы через Gemini
          </MutationButton>
        </div>
      </details>

      <section className="grid min-w-0 gap-4 border border-line bg-panel p-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(18rem,1fr)]">
        {playbackHref ? (
          <video
            ref={playerRef}
            data-testid="qa-evidence-video"
            src={playbackHref}
            controls
            preload="metadata"
            className="aspect-video w-full border border-line bg-black object-contain"
          />
        ) : (
          <div className="grid aspect-video place-items-center border border-line bg-black text-xs text-muted">
            Видео недоступно
          </div>
        )}
        <div className="min-w-0">
          <p className="font-mono text-[10px] uppercase text-accent">
            Кадр-доказательство
          </p>
          {selected ? (
            <>
              {/* Evidence is produced lazily by the Go API and cached in S3. */}
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={`${evidenceBaseHref}/${selected.timestampMs}`}
                alt={`Кадр ${formatClock(selected.timestampMs)}`}
                className="mt-3 aspect-video w-full border border-line bg-black object-contain"
              />
              <div className="mt-3 flex items-baseline justify-between gap-3">
                <strong className="text-sm">{selected.canonicalAction}</strong>
                <span className="font-mono text-[10px] text-muted">
                  {formatClock(selected.timestampMs)}
                </span>
              </div>
              <p className="mt-1 text-xs text-muted">
                {selected.screen} → {selected.target}
              </p>
            </>
          ) : null}
        </div>
      </section>

      <section className="min-w-0 border border-line bg-panel">
        <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
          <h2 className="text-sm font-semibold">Фрагменты доказательств</h2>
          <span className="font-mono text-[10px] text-muted">
            {selected ? formatClock(selected.timestampMs) : "нет событий"}
          </span>
        </div>
        <div
          data-testid="qa-evidence-fragments"
          className="flex flex-wrap gap-2 p-3"
        >
          {events.map((event) => {
            const active = selected?.id === event.id;
            const hasProblem = (event.qualityFlags ?? []).length > 0;
            return (
              <div key={event.id} className="contents">
                {scenarioBoundaries.startsByEventId
                  .get(event.id)
                  ?.map((mark) => (
                    <div key={mark.key} className="w-full shrink-0">
                      <ScenarioBoundaryDivider mark={mark} />
                    </div>
                  ))}
                <button
                  type="button"
                  data-testid="qa-evidence-fragment"
                  data-event-id={event.id}
                  data-timestamp-ms={event.timestampMs}
                  aria-pressed={active}
                  aria-label={`Перейти к фрагменту ${formatClock(event.timestampMs)}`}
                  onClick={() => setSelectedId(event.id)}
                  className={`w-48 overflow-hidden border text-left transition-colors active:translate-y-px ${
                    active
                      ? "border-accent bg-accent/10"
                      : hasProblem
                        ? "border-warning/60 bg-warning/5 hover:border-warning"
                        : "border-line hover:border-accent/60"
                  }`}
                >
                  {/* Evidence is produced lazily by the Go API and cached in S3. */}
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={`${evidenceBaseHref}/${event.timestampMs}`}
                    alt=""
                    loading="lazy"
                    className="aspect-video w-full border-b border-line bg-black object-cover"
                  />
                  <span className="block px-3 pt-2 font-mono text-[10px] text-accent">
                    {formatClock(event.timestampMs)}
                  </span>
                  <strong className="block truncate px-3 pt-1 text-xs">
                    {event.canonicalAction}
                  </strong>
                  <span className="block truncate px-3 pb-3 pt-1 text-[11px] text-muted">
                    {event.target || event.screen || "без цели"}
                  </span>
                </button>
                {scenarioBoundaries.endsByEventId.get(event.id)?.map((mark) => (
                  <div key={mark.key} className="w-full shrink-0">
                    <ScenarioBoundaryDivider mark={mark} />
                  </div>
                ))}
              </div>
            );
          })}
        </div>
      </section>

      <div className="grid min-w-0 gap-4 2xl:grid-cols-[18rem_minmax(22rem,1fr)_20rem]">
        <section className="min-w-0 border border-line bg-panel p-4">
          <h2 className="font-mono text-[10px] uppercase text-accent">
            Системные события
          </h2>
          <div className="mt-3 max-h-[42rem] overflow-y-auto">
            {events.map((event) => (
              <div key={event.id}>
                {scenarioBoundaries.startsByEventId
                  .get(event.id)
                  ?.map((mark) => (
                    <ScenarioBoundaryDivider
                      key={mark.key}
                      mark={mark}
                      compact
                    />
                  ))}
                <button
                  type="button"
                  onClick={() => setSelectedId(event.id)}
                  className={`w-full border-b border-line px-2 py-3 text-left ${
                    selected?.id === event.id
                      ? "bg-accent/10"
                      : "hover:bg-panel-raised"
                  }`}
                >
                  <span className="font-mono text-[10px] text-accent">
                    {formatClock(event.timestampMs)}
                  </span>
                  <strong className="mt-1 block text-xs">
                    {event.canonicalAction}
                  </strong>
                  <span className="mt-1 block truncate text-[11px] text-muted">
                    {event.target}
                  </span>
                </button>
                {scenarioBoundaries.endsByEventId.get(event.id)?.map((mark) => (
                  <ScenarioBoundaryDivider key={mark.key} mark={mark} compact />
                ))}
              </div>
            ))}
          </div>
        </section>

        {selected ? (
          <EventEditor
            key={`${selected.id}-${selected.version}`}
            recordingId={recordingId}
            event={selected}
            rawEvents={rawEvents}
          />
        ) : null}

        <section className="grid min-w-0 content-start gap-4">
          <div className="min-w-0 border border-line bg-panel p-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="font-mono text-[10px] uppercase text-accent">
                Проблемы качества
              </h2>
              <span className="font-mono text-[10px] text-muted">
                Открыто: {openIssues.length}
              </span>
            </div>
            <div className="mt-3 grid max-h-[32rem] gap-2 overflow-y-auto pr-1">
              {!issues.length ? (
                <p className="border border-dashed border-line p-4 text-xs leading-5 text-muted">
                  Автоматических замечаний нет. Вы всё равно можете проверить
                  выбранные события вручную.
                </p>
              ) : null}
              {issues.map((issue) => (
                <article
                  key={issue.id}
                  className={`grid gap-3 border p-3 md:grid-cols-[1fr_auto] ${
                    issue.resolved
                      ? "border-line text-muted"
                      : "border-warning/50"
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => {
                      const event =
                        events.find(
                          (item) => item.id === issue.actionEventId,
                        ) ??
                        events.find(
                          (item) =>
                            Math.abs(item.timestampMs - issue.timestampMs) <=
                            1500,
                        );
                      if (event) setSelectedId(event.id);
                    }}
                    className="text-left"
                  >
                    <span className="font-mono text-[10px] uppercase">
                      {issue.severity} · {formatClock(issue.timestampMs)}
                    </span>
                    <strong className="mt-1 block text-xs">
                      {formatQualityIssueType(issue.type)}
                    </strong>
                    <span className="mt-1 block text-[11px] leading-5 text-muted">
                      {formatQualityIssueMessage(issue.message)}
                    </span>
                  </button>
                  <ResolveIssue
                    recordingId={recordingId}
                    issueId={issue.id}
                    resolved={issue.resolved}
                  />
                </article>
              ))}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
