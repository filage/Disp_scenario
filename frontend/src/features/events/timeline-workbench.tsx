"use client";

import { Fragment, useMemo, useState } from "react";
import { EventEditor } from "@/features/events/event-editor";
import { timedEventNarrative } from "@/features/events/event-narrative";
import {
  ScenarioBoundaryDivider,
  ScenarioBoundaryTableRow,
  createScenarioBoundaryLookup,
} from "@/features/events/scenario-boundaries";
import type {
  ActionEvent,
  RawEvent,
  ScenarioInstance,
} from "@/features/events/types";
import { formatClock, formatIssueType, formatQAStatus } from "@/lib/display";

export function TimelineWorkbench({
  recordingId,
  events,
  rawEvents,
  scenarioInstances = [],
  exportHref,
}: {
  recordingId: string;
  events: ActionEvent[];
  rawEvents: RawEvent[];
  scenarioInstances?: ScenarioInstance[];
  exportHref: string;
}) {
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState(events[0]?.id ?? "");
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return events;
    return events.filter((event) =>
      [
        event.canonicalAction,
        event.screen,
        event.target,
        event.issueType,
        event.orderId,
        String(event.timestampMs),
      ].some((value) =>
        String(value ?? "")
          .toLowerCase()
          .includes(needle),
      ),
    );
  }, [events, query]);
  const selected =
    events.find((event) => event.id === selectedId) ?? filtered[0] ?? null;
  const scenarioBoundaries = useMemo(
    () => createScenarioBoundaryLookup(events, scenarioInstances),
    [events, scenarioInstances],
  );

  return (
    <div className="mt-6 grid gap-4 2xl:grid-cols-[17rem_minmax(0,1fr)_23rem]">
      <section className="border border-line bg-panel p-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="font-mono text-[10px] uppercase text-accent">
            Индекс событий
          </h2>
          <span className="font-mono text-[10px] text-muted">
            {filtered.length}/{events.length}
          </span>
        </div>
        <input
          value={query}
          onChange={(change) => setQuery(change.target.value)}
          placeholder="Время, действие, цель…"
          className="mt-4 w-full border border-line bg-background px-3 py-2 text-xs"
        />
        <div className="mt-3 max-h-[36rem] overflow-y-auto">
          {filtered.map((event) => (
            <div key={event.id}>
              {scenarioBoundaries.startsByEventId.get(event.id)?.map((mark) => (
                <ScenarioBoundaryDivider key={mark.key} mark={mark} compact />
              ))}
              <button
                type="button"
                onClick={() => setSelectedId(event.id)}
                className={`grid w-full grid-cols-[3.5rem_1fr] gap-3 border-b border-line px-2 py-3 text-left ${
                  selected?.id === event.id
                    ? "bg-accent/10 text-foreground"
                    : "text-muted hover:text-foreground"
                }`}
              >
                <span className="font-mono text-[10px] text-accent">
                  {formatClock(event.timestampMs)}
                </span>
                <span>
                  <strong className="block text-xs font-medium">
                    {event.canonicalAction}
                  </strong>
                  <span className="mt-1 block truncate text-[11px]">
                    {event.target || event.screen}
                  </span>
                </span>
              </button>
              {scenarioBoundaries.endsByEventId.get(event.id)?.map((mark) => (
                <ScenarioBoundaryDivider key={mark.key} mark={mark} compact />
              ))}
            </div>
          ))}
        </div>
      </section>

      <section className="min-w-0 border border-line bg-panel p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-mono text-[10px] uppercase text-accent">
            Канонический таймлайн
          </h2>
          <a
            href={exportHref}
            className="border border-line px-3 py-2 text-[10px] uppercase text-muted hover:text-accent"
          >
            Экспорт CSV
          </a>
        </div>
        <section className="mt-4 border border-line bg-background p-4">
          <div className="flex items-center justify-between gap-3">
            <h3 className="font-mono text-[10px] uppercase text-accent">
              Ход выполнения
            </h3>
            <span className="font-mono text-[10px] text-muted">
              {events.length} шагов
            </span>
          </div>
          <ol className="mt-3 grid max-h-48 gap-2 overflow-y-auto pr-1 text-xs leading-5 text-[#26364f]">
            {events.map((event) => (
              <li key={event.id} className="border-l border-line pl-3">
                {timedEventNarrative(event)}
              </li>
            ))}
          </ol>
        </section>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full min-w-[64rem] text-left text-xs">
            <thead className="font-mono text-[10px] uppercase text-muted">
              <tr className="border-b border-line">
                <th className="px-3 py-3">Время</th>
                <th className="px-3 py-3">Действие</th>
                <th className="px-3 py-3">Экран</th>
                <th className="px-3 py-3">Цель</th>
                <th className="px-3 py-3">Тип проблемы</th>
                <th className="px-3 py-3">Уверенность</th>
                <th className="px-3 py-3">Флаги</th>
                <th className="px-3 py-3">QA</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((event) => (
                <Fragment key={event.id}>
                  {scenarioBoundaries.startsByEventId
                    .get(event.id)
                    ?.map((mark) => (
                      <ScenarioBoundaryTableRow
                        key={mark.key}
                        mark={mark}
                        colSpan={8}
                      />
                    ))}
                  <tr
                    onClick={() => setSelectedId(event.id)}
                    className={`cursor-pointer border-b border-line ${
                      selected?.id === event.id
                        ? "bg-accent/10"
                        : "hover:bg-panel-raised"
                    }`}
                  >
                    <td className="px-3 py-3 font-mono text-accent">
                      {formatClock(event.timestampMs)}
                    </td>
                    <td className="px-3 py-3 font-medium">
                      {event.canonicalAction}
                    </td>
                    <td className="px-3 py-3 text-muted">{event.screen}</td>
                    <td className="px-3 py-3 text-muted">
                      {event.target || event.entityId || event.orderId || "—"}
                    </td>
                    <td className="px-3 py-3 text-muted">
                      {formatIssueType(event.issueType)}
                    </td>
                    <td className="px-3 py-3 font-mono">
                      {Math.round(event.confidence * 100)}%
                    </td>
                    <td className="px-3 py-3">
                      <FlagList flags={event.qualityFlags ?? []} />
                    </td>
                    <td className="px-3 py-3 text-muted">
                      {formatQAStatus(event.qaStatus)}
                    </td>
                  </tr>
                  {scenarioBoundaries.endsByEventId
                    .get(event.id)
                    ?.map((mark) => (
                      <ScenarioBoundaryTableRow
                        key={mark.key}
                        mark={mark}
                        colSpan={8}
                      />
                    ))}
                </Fragment>
              ))}
            </tbody>
          </table>
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
    </div>
  );
}

function FlagList({ flags }: { flags: string[] }) {
  if (!flags.length) return <span className="text-muted">—</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {flags.map((flag) => (
        <span
          key={flag}
          className="border border-warning/50 px-1.5 py-0.5 font-mono text-[9px] text-warning"
        >
          {flag}
        </span>
      ))}
    </div>
  );
}
