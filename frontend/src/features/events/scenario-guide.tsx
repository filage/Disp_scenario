import { CheckCircle2, ImageIcon } from "lucide-react";
import type { ActionEvent, ScenarioInstance } from "@/features/events/types";
import { eventNarrative } from "@/features/events/event-narrative";
import { formatClock, formatDuration, formatIssueType } from "@/lib/display";

export function ScenarioGuide({
  events,
  scenarios,
  evidenceBaseHref,
}: {
  events: ActionEvent[];
  scenarios: ScenarioInstance[];
  evidenceBaseHref: string;
}) {
  const ordered = [...events].sort(
    (left, right) => left.timestampMs - right.timestampMs,
  );
  const duration = ordered.length
    ? ordered[ordered.length - 1].timestampMs - ordered[0].timestampMs
    : 0;

  return (
    <div className="mt-6">
      <section className="grid gap-px overflow-hidden border border-line bg-line md:grid-cols-[1.5fr_1fr_1fr]">
        <GuideMetric
          label="Что показано"
          value={`${ordered.length} шагов`}
          detail="Действия пользователя в порядке выполнения"
        />
        <GuideMetric
          label="Время выполнения"
          value={formatDuration(duration)}
          detail="От первого до последнего распознанного действия"
        />
        <GuideMetric
          label="Сценарии"
          value={scenarios.length}
          detail="Распознанные системой участки записи"
        />
      </section>

      <section className="mt-6 border-t border-line">
        {ordered.map((event, index) => {
          const scenario = scenarios.find((item) =>
            item.eventIds?.includes(event.id),
          );
          const showFrame = shouldShowFrame(event.canonicalAction);
          return (
            <article
              key={event.id}
              className="grid gap-4 border-b border-line py-6 md:grid-cols-[4rem_minmax(0,1fr)] xl:grid-cols-[4rem_minmax(20rem,0.8fr)_minmax(22rem,1.2fr)]"
            >
              <div className="flex items-start gap-3 md:block">
                <span className="grid size-9 place-items-center rounded-full border border-accent/40 bg-accent-soft font-mono text-xs font-semibold text-accent">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <span className="mt-2 block font-mono text-xs text-muted">
                  {formatClock(event.timestampMs)}
                </span>
              </div>

              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <CheckCircle2 size={16} className="text-success" />
                  <span className="text-xs font-medium text-muted">
                    {scenario
                      ? formatIssueType(scenario.issueType)
                      : "Подготовка к сценарию"}
                  </span>
                </div>
                <h2 className="mt-3 text-lg font-semibold leading-7">
                  {eventNarrative(event)}
                </h2>
                <p className="mt-2 text-sm leading-6 text-muted">
                  Экран: {event.screen || "не определён"}
                  {event.target ? ` · Объект: ${event.target}` : ""}
                </p>
                {event.qaComment ? (
                  <p className="mt-3 border-l-2 border-accent pl-3 text-sm leading-6 text-muted">
                    {event.qaComment}
                  </p>
                ) : null}
              </div>

              <div className="min-w-0 md:col-start-2 xl:col-start-auto">
                {showFrame ? (
                  <figure className="overflow-hidden rounded-md border border-line bg-panel">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={`${evidenceBaseHref}/${event.timestampMs}`}
                      alt={`Кадр шага ${index + 1}: ${eventNarrative(event)}`}
                      loading="lazy"
                      className="aspect-video w-full bg-[#111827] object-contain"
                    />
                    <figcaption className="flex items-center gap-2 px-3 py-2 text-xs text-muted">
                      <ImageIcon size={14} /> Кадр из записи на{" "}
                      {formatClock(event.timestampMs)}
                    </figcaption>
                  </figure>
                ) : (
                  <div className="flex min-h-28 items-center rounded-md border border-dashed border-line bg-panel-raised px-4 text-sm leading-6 text-muted">
                    Для этого шага достаточно текстового описания. Ключевые
                    действия ниже сопровождаются кадрами.
                  </div>
                )}
              </div>
            </article>
          );
        })}
      </section>
    </div>
  );
}

function GuideMetric({
  label,
  value,
  detail,
}: {
  label: string;
  value: string | number;
  detail: string;
}) {
  return (
    <div className="bg-panel p-5">
      <p className="text-xs font-medium text-muted">{label}</p>
      <p className="mt-2 text-2xl font-semibold tracking-tight">{value}</p>
      <p className="mt-1 text-xs leading-5 text-muted">{detail}</p>
    </div>
  );
}

function shouldShowFrame(action: string) {
  return new Set([
    "OPEN_ORDER",
    "TAKE_ACTION",
    "OPEN_DRIVER_ASSIGNMENT",
    "SELECT_DRIVER",
    "SEND_TO_SELECTED_DRIVER",
    "OPEN_FIELD_EDITOR",
    "CHANGE_FIELD_VALUE",
    "SAVE",
    "RESOLVE_ISSUE",
  ]).has(action);
}
