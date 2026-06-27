import type { ActionEvent, ScenarioInstance } from "./types";
import { formatClock, formatIssueType } from "../../lib/display";

export type ScenarioBoundaryMark = {
  key: string;
  kind: "start" | "end";
  title: string;
  detail: string;
  timestampMs: number;
};

export type ScenarioBoundaryLookup = {
  startsByEventId: Map<string, ScenarioBoundaryMark[]>;
  endsByEventId: Map<string, ScenarioBoundaryMark[]>;
};

export function createScenarioBoundaryLookup(
  events: ActionEvent[],
  instances: ScenarioInstance[] = [],
): ScenarioBoundaryLookup {
  const startsByEventId = new Map<string, ScenarioBoundaryMark[]>();
  const endsByEventId = new Map<string, ScenarioBoundaryMark[]>();
  const eventsById = new Map(events.map((event) => [event.id, event]));

  for (const instance of instances) {
    const scenarioEvents = (instance.eventIds ?? [])
      .map((id) => eventsById.get(id))
      .filter((event): event is ActionEvent => Boolean(event));
    const start =
      scenarioEvents[0] ?? closestEvent(events, instance.startedAtMs);
    const end =
      scenarioEvents[scenarioEvents.length - 1] ??
      closestEvent(events, instance.endedAtMs);

    if (start) {
      pushMark(startsByEventId, start.id, {
        key: `${instance.id}-start`,
        kind: "start",
        title: "Начало сценария",
        detail: scenarioDetail(instance),
        timestampMs: instance.startedAtMs,
      });
    }

    if (end) {
      pushMark(endsByEventId, end.id, {
        key: `${instance.id}-end`,
        kind: "end",
        title: "Конец сценария",
        detail: scenarioDetail(instance),
        timestampMs: instance.endedAtMs,
      });
    }
  }

  return { startsByEventId, endsByEventId };
}

export function ScenarioBoundaryDivider({
  mark,
  compact = false,
}: {
  mark: ScenarioBoundaryMark;
  compact?: boolean;
}) {
  const tone =
    mark.kind === "start"
      ? "border-success/55 bg-success/5 text-success"
      : "border-accent/55 bg-accent/5 text-accent";

  return (
    <div
      data-testid="scenario-boundary"
      data-boundary-kind={mark.kind}
      className={`flex min-w-0 items-center gap-2 border-y ${tone} ${
        compact ? "px-2 py-1.5" : "px-3 py-2"
      }`}
    >
      <span className="h-5 border-l border-current" aria-hidden="true" />
      <span className="h-px min-w-6 flex-1 bg-current/35" aria-hidden="true" />
      <span className="min-w-0 text-center font-mono text-[10px] uppercase tracking-normal">
        {mark.title}
        <span className="ml-2 text-muted">{formatClock(mark.timestampMs)}</span>
        <span className="ml-2 normal-case text-muted">{mark.detail}</span>
      </span>
      <span className="h-px min-w-6 flex-1 bg-current/35" aria-hidden="true" />
      <span className="h-5 border-l border-current" aria-hidden="true" />
    </div>
  );
}

export function ScenarioBoundaryTableRow({
  mark,
  colSpan,
}: {
  mark: ScenarioBoundaryMark;
  colSpan: number;
}) {
  return (
    <tr>
      <td colSpan={colSpan} className="p-0">
        <ScenarioBoundaryDivider mark={mark} compact />
      </td>
    </tr>
  );
}

function closestEvent(events: ActionEvent[], timestampMs: number) {
  if (!events.length) return null;
  return events.reduce((closest, event) =>
    Math.abs(event.timestampMs - timestampMs) <
    Math.abs(closest.timestampMs - timestampMs)
      ? event
      : closest,
  );
}

function pushMark(
  target: Map<string, ScenarioBoundaryMark[]>,
  eventId: string,
  mark: ScenarioBoundaryMark,
) {
  const marks = target.get(eventId) ?? [];
  marks.push(mark);
  target.set(eventId, marks);
}

function scenarioDetail(instance: ScenarioInstance) {
  const label = formatIssueType(instance.issueType);
  const target = instance.orderId || instance.entityId;
  return target ? `${label} · ${target}` : label;
}
