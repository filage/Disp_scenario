"use client";

import { useMemo, useState } from "react";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { formatIssueType, formatScenarioStatus } from "@/lib/display";

export function GroupBrowser({ groups }: { groups: ScenarioGroup[] }) {
  const [query, setQuery] = useState("");
  const [onlyCandidates, setOnlyCandidates] = useState(false);
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return groups.filter((group) => {
      if (onlyCandidates && group.status !== "candidate") return false;
      if (!needle) return true;
      return [group.name, group.issueType, group.signature, group.code].some(
        (value) => String(value ?? "").toLowerCase().includes(needle),
      );
    });
  }, [groups, onlyCandidates, query]);

  return (
    <>
      <div className="mt-6 flex flex-wrap items-center gap-3 border border-line bg-panel p-4">
        <input
          value={query}
          onChange={(change) => setQuery(change.target.value)}
          placeholder="Поиск по сигнатуре, коду или типу…"
          className="min-w-64 flex-1 border border-line bg-background px-3 py-2 text-sm"
        />
        <button
          type="button"
          aria-pressed={onlyCandidates}
          onClick={() => setOnlyCandidates((current) => !current)}
          className={`border px-3 py-2 text-[10px] uppercase ${
            onlyCandidates
              ? "border-accent bg-accent/10 text-accent"
              : "border-line text-muted"
          }`}
        >
          Только кандидаты
        </button>
        <span className="font-mono text-[10px] text-muted">
          {filtered.length}/{groups.length}
        </span>
      </div>
      <div className="mt-4 overflow-x-auto border border-line bg-panel">
        <table className="w-full min-w-[64rem] text-left text-xs">
          <thead className="font-mono text-[10px] uppercase text-muted">
            <tr className="border-b border-line">
              <th className="px-4 py-3">Сценарий</th>
              <th className="px-4 py-3">Сигнатура</th>
              <th className="px-4 py-3">Частота</th>
              <th className="px-4 py-3">Медиана / P95</th>
              <th className="px-4 py-3">Ручные действия</th>
              <th className="px-4 py-3">Уверенность</th>
              <th className="px-4 py-3">Автоматизация</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((group) => (
              <tr key={group.id} className="border-b border-line align-top">
                <td className="px-4 py-4">
                  <span className="font-mono text-[10px] text-accent">
                    {group.code ?? formatScenarioStatus(group.status)}
                  </span>
                  <strong className="mt-1 block text-sm">{group.name}</strong>
                  <span className="mt-1 block text-muted">
                    {formatIssueType(group.issueType)}
                  </span>
                </td>
                <td className="max-w-md px-4 py-4">
                  <p className="font-mono text-[10px] leading-5 text-muted">
                    {group.signature}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {(group.actionSequence ?? []).map((action) => (
                      <span
                        key={action}
                        className="border border-line px-1.5 py-1 font-mono text-[9px]"
                      >
                        {action}
                      </span>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-4 font-mono text-lg">
                  {group.frequency}
                </td>
                <td className="px-4 py-4 font-mono">
                  {(group.medianDurationMs / 1000).toFixed(1)}s /{" "}
                  {(group.p95DurationMs / 1000).toFixed(1)}s
                </td>
                <td className="px-4 py-4 font-mono">
                  {group.manualCheckCount +
                    (group.repeatedActionCount ?? 0)}
                </td>
                <td className="px-4 py-4 font-mono">
                  {Math.round(group.confidenceAverage * 100)}%
                </td>
                <td className="px-4 py-4">
                  <span className="font-mono text-lg text-accent">
                    {Math.round(group.automationScore * 100)}
                  </span>
                  <span className="block text-[10px] uppercase text-muted">
                    оценка
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
