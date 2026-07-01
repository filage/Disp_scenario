"use client";

import { useMemo, useState } from "react";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { actionDescription } from "@/features/scenarios/action-description";
import { formatDuration, formatIssueType } from "@/lib/display";

export type GraphNode = {
  id: string;
  label: string;
  type: string;
  severity: string;
  frequency: number;
  confidence: number;
  issueTypes?: string[];
  relatedScenarioIds?: string[];
  examples?: {
    screen?: string;
    target?: string;
    issueType?: string;
    timestampMs?: number;
  }[];
};

export type GraphEdge = {
  id: string;
  from: string;
  to: string;
  frequency: number;
  averageTimeToNextMs: number;
};

export type ScenarioGraph = {
  nodes: GraphNode[];
  edges: GraphEdge[];
};

const allGroups = "Все группы";
const allIssues = "Все типы";

export function ScenarioMapWorkbench({
  graph,
  groups,
}: {
  graph: ScenarioGraph;
  groups: ScenarioGroup[];
}) {
  const [group, setGroup] = useState(allGroups);
  const [issue, setIssue] = useState(allIssues);
  const [minimumConfidence, setMinimumConfidence] = useState(0.8);
  const [sort, setSort] = useState<"frequency" | "confidence" | "action">(
    "frequency",
  );
  const [zoom, setZoom] = useState(1);
  const [selectedId, setSelectedId] = useState("");
  const issues = useMemo(
    () => [
      allIssues,
      ...new Set(
        graph.nodes.flatMap((node) => node.issueTypes ?? []).filter(Boolean),
      ),
    ],
    [graph.nodes],
  );
  const filtered = useMemo(() => {
    const selectedGroup = groups.find((item) => item.name === group);
    const allowedActions = selectedGroup
      ? new Set(selectedGroup.actionSequence)
      : null;
    const nodes = graph.nodes
      .filter((node) => {
        if (allowedActions && !allowedActions.has(node.id)) return false;
        if (issue !== allIssues && !node.issueTypes?.includes(issue)) {
          return false;
        }
        return node.confidence >= minimumConfidence;
      })
      .toSorted((left, right) => {
        if (sort === "confidence") {
          return right.confidence - left.confidence;
        }
        if (sort === "action") return left.label.localeCompare(right.label);
        return right.frequency - left.frequency;
      });
    const nodeIDs = new Set(nodes.map((node) => node.id));
    return {
      nodes,
      edges: graph.edges
        .filter((edge) => nodeIDs.has(edge.from) && nodeIDs.has(edge.to))
        .toSorted((left, right) => right.frequency - left.frequency),
    };
  }, [graph, group, groups, issue, minimumConfidence, sort]);
  const selected =
    filtered.nodes.find((node) => node.id === selectedId) ?? null;
  const layout = useMemo(
    () => buildLayout(filtered.nodes, filtered.edges),
    [filtered.edges, filtered.nodes],
  );
  const width = 960 / zoom;
  const height = 620 / zoom;
  const viewBox = `${(960 - width) / 2} ${(620 - height) / 2} ${width} ${height}`;

  return (
    <>
      <div className="mt-6 grid gap-3 border border-line bg-panel p-4 md:grid-cols-2 xl:grid-cols-4">
        <Select
          label="Сортировка"
          value={sort}
          onChange={(value) =>
            setSort(value as "frequency" | "confidence" | "action")
          }
          options={[
            ["frequency", "Частота"],
            ["confidence", "Уверенность"],
            ["action", "Действие"],
          ]}
        />
        <Select
          label="Группа"
          value={group}
          onChange={setGroup}
          options={[
            [allGroups, allGroups],
            ...groups.map((item) => [item.name, item.name] as const),
          ]}
        />
        <Select
          label="Тип проблемы"
          value={issue}
          onChange={setIssue}
          options={issues.map((item) => [item, item] as const)}
        />
        <Select
          label="Уверенность"
          value={String(minimumConfidence)}
          onChange={(value) => setMinimumConfidence(Number(value))}
          options={[
            ["0.8", "Высокая ≥80%"],
            ["0.6", "Средняя ≥60%"],
            ["0", "Любая"],
          ]}
        />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <section className="relative min-h-[38rem] overflow-hidden border border-line bg-panel-raised">
          <div className="pointer-events-none absolute left-3 top-3 z-10 border border-line bg-background/95 px-3 py-2 font-mono text-[10px] uppercase text-muted">
            Поток слева направо · наконечник указывает следующий шаг
          </div>
          <div className="absolute right-3 top-3 z-10 flex border border-line bg-background">
            <ZoomButton
              onClick={() => setZoom((value) => Math.min(1.8, value + 0.15))}
            >
              +
            </ZoomButton>
            <ZoomButton
              onClick={() => setZoom((value) => Math.max(0.65, value - 0.15))}
            >
              −
            </ZoomButton>
            <ZoomButton onClick={() => setZoom(1)}>⌖</ZoomButton>
          </div>
          <svg
            role="img"
            aria-label="Граф действий сценариев"
            viewBox={viewBox}
            className="h-[38rem] w-full"
          >
            <defs>
              <marker
                id="arrow"
                markerWidth="10"
                markerHeight="10"
                refX="8"
                refY="3"
                orient="auto"
                markerUnits="strokeWidth"
              >
                <path d="M0,0 L0,6 L9,3 z" fill="var(--color-accent)" />
              </marker>
            </defs>
            {filtered.edges.map((edge) => {
              const from = layout.get(edge.from);
              const to = layout.get(edge.to);
              if (!from || !to || edge.from === edge.to) return null;
              const focused =
                !selected ||
                selected.id === edge.from ||
                selected.id === edge.to;
              const segment = trimEdge(from, to);
              return (
                <g key={edge.id} opacity={focused ? 1 : 0.18}>
                  <line
                    x1={segment.x1}
                    y1={segment.y1}
                    x2={segment.x2}
                    y2={segment.y2}
                    stroke={
                      focused && selected
                        ? "var(--color-accent)"
                        : "var(--color-line)"
                    }
                    strokeWidth={Math.min(4, 1.25 + edge.frequency * 0.6)}
                    markerEnd="url(#arrow)"
                  />
                  <text
                    x={(from.x + to.x) / 2}
                    y={(from.y + to.y) / 2 - 8}
                    fill="var(--color-muted)"
                    fontSize="11"
                    textAnchor="middle"
                  >
                    {edge.frequency}× ·{" "}
                    {formatDuration(edge.averageTimeToNextMs)}
                  </text>
                </g>
              );
            })}
            {filtered.nodes.map((node) => {
              const point = layout.get(node.id);
              if (!point) return null;
              const description = actionDescription(node.id);
              const focused =
                !selected ||
                selected.id === node.id ||
                filtered.edges.some(
                  (edge) =>
                    (edge.from === selected.id && edge.to === node.id) ||
                    (edge.to === selected.id && edge.from === node.id),
                );
              return (
                <g
                  key={node.id}
                  role="button"
                  tabIndex={0}
                  aria-label={`${node.label}. ${description}. Нажмите повторно, чтобы показать весь граф.`}
                  onClick={() =>
                    setSelectedId((current) =>
                      current === node.id ? "" : node.id,
                    )
                  }
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      setSelectedId((current) =>
                        current === node.id ? "" : node.id,
                      );
                    }
                  }}
                  className="cursor-pointer"
                  opacity={focused ? 1 : 0.25}
                >
                  <rect
                    x={point.x - 94}
                    y={point.y - 41}
                    width="188"
                    height="82"
                    rx="3"
                    fill={
                      selected?.id === node.id
                        ? "color-mix(in srgb, var(--color-accent) 18%, var(--color-panel))"
                        : "var(--color-panel)"
                    }
                    stroke={
                      selected?.id === node.id
                        ? "var(--color-accent)"
                        : "var(--color-line)"
                    }
                  />
                  <text
                    x={point.x}
                    y={point.y - 15}
                    fill="var(--color-foreground)"
                    fontSize="12"
                    fontWeight="600"
                    textAnchor="middle"
                  >
                    {node.label}
                  </text>
                  <text
                    x={point.x}
                    y={point.y + 5}
                    fill="var(--color-foreground)"
                    fontSize="10"
                    textAnchor="middle"
                  >
                    {description}
                  </text>
                  <text
                    x={point.x}
                    y={point.y + 25}
                    fill="var(--color-muted)"
                    fontSize="10"
                    textAnchor="middle"
                  >
                    {node.frequency}× · {Math.round(node.confidence * 100)}%
                  </text>
                </g>
              );
            })}
          </svg>
          {!filtered.nodes.length ? (
            <div className="absolute inset-0 grid place-items-center text-sm text-muted">
              Нет узлов для выбранных фильтров.
            </div>
          ) : null}
        </section>

        <aside className="border border-line bg-panel p-5">
          <p className="font-mono text-[10px] uppercase text-accent">
            Инспектор графа
          </p>
          {selected ? (
            <>
              <h2 className="mt-3 text-lg font-semibold">{selected.label}</h2>
              <p className="mt-1 text-sm text-foreground">
                {actionDescription(selected.id)}
              </p>
              <p className="mt-2 text-xs text-muted">
                {formatNodeType(selected.type)} · повторов: {selected.frequency}{" "}
                · уверенность {Math.round(selected.confidence * 100)}%
              </p>
              <div className="mt-4 flex flex-wrap gap-1">
                {(selected.issueTypes ?? []).map((item) => (
                  <span
                    key={item}
                    className="border border-warning/50 px-2 py-1 text-[10px] text-warning"
                  >
                    {formatIssueType(item)}
                  </span>
                ))}
              </div>
              <h3 className="mt-6 font-mono text-[10px] uppercase text-muted">
                Связанные переходы
              </h3>
              <div className="mt-2 grid gap-2">
                {filtered.edges
                  .filter(
                    (edge) =>
                      edge.from === selected.id || edge.to === selected.id,
                  )
                  .map((edge) => (
                    <div key={edge.id} className="border-l border-line pl-3">
                      <p className="mb-1 font-mono text-[9px] uppercase tracking-wide text-accent">
                        {edge.from === selected.id ? "Исходящий" : "Входящий"}
                      </p>
                      <p className="text-xs leading-5">
                        {labelForNode(filtered.nodes, edge.from)} →{" "}
                        {labelForNode(filtered.nodes, edge.to)}
                      </p>
                      <p className="mt-1 font-mono text-[10px] text-muted">
                        {edge.frequency}× ·{" "}
                        {formatDuration(edge.averageTimeToNextMs)}
                      </p>
                    </div>
                  ))}
              </div>
              <h3 className="mt-6 font-mono text-[10px] uppercase text-muted">
                Примеры
              </h3>
              <div className="mt-2 grid gap-2">
                {(selected.examples ?? []).slice(0, 5).map((example, index) => (
                  <div
                    key={`${example.timestampMs}-${index}`}
                    className="border-l border-line pl-3 text-xs"
                  >
                    <p>
                      {example.screen} → {example.target}
                    </p>
                    <p className="mt-1 text-[10px] text-muted">
                      {formatDuration(example.timestampMs ?? 0)} ·{" "}
                      {example.issueType
                        ? formatIssueType(example.issueType)
                        : "без проблемы"}
                    </p>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="mt-3 text-sm leading-6 text-muted">
              Выберите узел, чтобы увидеть связанные переходы, типы проблем и
              примеры из событий. Повторное нажатие на выбранный узел вернёт
              весь граф.
            </p>
          )}
        </aside>
      </div>
    </>
  );
}

function Select({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: readonly (readonly [string, string])[];
}) {
  return (
    <label className="grid gap-2 text-[10px] uppercase text-muted">
      {label}
      <select
        value={value}
        onChange={(change) => onChange(change.target.value)}
        className="min-w-0 border border-line bg-background px-3 py-2 text-xs normal-case text-foreground"
      >
        {options.map(([id, name]) => (
          <option key={id} value={id}>
            {name}
          </option>
        ))}
      </select>
    </label>
  );
}

function ZoomButton({
  children,
  onClick,
}: {
  children: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="grid size-9 place-items-center border-r border-line text-sm text-muted last:border-r-0 hover:text-accent"
    >
      {children}
    </button>
  );
}

function buildLayout(nodes: GraphNode[], edges: GraphEdge[]) {
  const result = new Map<string, { x: number; y: number }>();
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const incoming = new Map(nodes.map((node) => [node.id, 0]));
  const outgoing = new Map(nodes.map((node) => [node.id, [] as string[]]));
  for (const edge of edges) {
    if (
      edge.from === edge.to ||
      !nodeIDs.has(edge.from) ||
      !nodeIDs.has(edge.to)
    )
      continue;
    outgoing.get(edge.from)?.push(edge.to);
    incoming.set(edge.to, (incoming.get(edge.to) ?? 0) + 1);
  }
  const rank = new Map(nodes.map((node) => [node.id, 0]));
  const queue = nodes
    .filter((node) => incoming.get(node.id) === 0)
    .map((node) => node.id);
  const visited = new Set<string>();
  while (queue.length) {
    const id = queue.shift();
    if (!id || visited.has(id)) continue;
    visited.add(id);
    for (const target of outgoing.get(id) ?? []) {
      rank.set(
        target,
        Math.max(rank.get(target) ?? 0, (rank.get(id) ?? 0) + 1),
      );
      incoming.set(target, (incoming.get(target) ?? 1) - 1);
      if (incoming.get(target) === 0) queue.push(target);
    }
  }
  for (const node of nodes) {
    if (!visited.has(node.id)) rank.set(node.id, fallbackRank(node.id));
  }
  const rankValues = [...new Set(rank.values())].toSorted(
    (left, right) => left - right,
  );
  const columnCount = Math.min(4, rankValues.length);
  const columnByRank = new Map(
    rankValues.map((value, index) => [
      value,
      Math.floor((index * columnCount) / rankValues.length),
    ]),
  );
  const columns = new Map<number, GraphNode[]>();
  for (const node of nodes) {
    const column = columnByRank.get(rank.get(node.id) ?? 0) ?? 0;
    columns.set(column, [...(columns.get(column) ?? []), node]);
  }
  for (const [column, columnNodes] of columns) {
    columnNodes.forEach((node, row) => {
      result.set(node.id, {
        x: ((column + 1) * 960) / (columns.size + 1),
        y: ((row + 1) * 620) / (columnNodes.length + 1),
      });
    });
  }
  return result;
}

function fallbackRank(action: string) {
  const ranks: Record<string, number> = {
    OPEN_ORDER: 0,
    INSPECT_ISSUE: 1,
    CHECK: 2,
    OPEN_DRIVER_ASSIGNMENT: 2,
    OPEN_FIELD_EDITOR: 2,
    EDIT_FIELD: 2,
    SELECT_DRIVER: 3,
    CHANGE_FIELD_VALUE: 3,
    SEND_TO_SELECTED_DRIVER: 3,
    ASSIGN_DRIVER: 3,
    SAVE: 3,
    TAKE_ACTION: 4,
    RESOLVE_ISSUE: 5,
    NAVIGATE: 6,
  };
  return ranks[action] ?? 3;
}

function trimEdge(
  from: { x: number; y: number },
  to: { x: number; y: number },
) {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const distance = Math.hypot(dx, dy) || 1;
  const unitX = dx / distance;
  const unitY = dy / distance;
  const offset = Math.min(
    Math.abs(unitX) > 0 ? 94 / Math.abs(unitX) : Number.POSITIVE_INFINITY,
    Math.abs(unitY) > 0 ? 41 / Math.abs(unitY) : Number.POSITIVE_INFINITY,
  );
  return {
    x1: from.x + unitX * offset,
    y1: from.y + unitY * offset,
    x2: to.x - unitX * offset,
    y2: to.y - unitY * offset,
  };
}

function labelForNode(nodes: GraphNode[], id: string) {
  return nodes.find((node) => node.id === id)?.label ?? id;
}

function formatNodeType(value: string) {
  const labels: Record<string, string> = {
    action: "Действие",
    issue: "Проблема",
    screen: "Экран",
    target: "Цель",
  };
  return labels[value] ?? value;
}
