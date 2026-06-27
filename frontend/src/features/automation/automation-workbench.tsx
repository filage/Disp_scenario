"use client";

import { useMemo, useState } from "react";
import type { ScenarioGroup } from "@/features/scenarios/types";
import {
  formatScenarioStatus,
  formatTechnicalMetricName,
} from "@/lib/display";

export function AutomationWorkbench({
  groups,
  recordingCount,
}: {
  groups: ScenarioGroup[];
  recordingCount: number;
}) {
  const candidates = useMemo(
    () =>
      groups
        .flatMap((group) =>
          (group.automationCandidates ?? []).map((candidate) => ({
            ...candidate,
            groupName: group.name,
            groupCode: group.code,
          })),
        )
        .toSorted((a, b) => b.score - a.score),
    [groups],
  );
  const [selectedId, setSelectedId] = useState(candidates[0]?.id ?? "");
  const selected =
    candidates.find((candidate) => candidate.id === selectedId) ??
    candidates[0] ??
    null;
  const totalImpact = candidates.reduce(
    (sum, candidate) =>
      sum + Number(candidate.breakdown?.durationImpactMs ?? 0),
    0,
  );

  return (
    <>
      <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Summary label="Кандидаты" value={String(candidates.length)} />
        <Summary label="Группы сценариев" value={String(groups.length)} />
        <Summary label="Область" value={formatRecordingCount(recordingCount)} />
        <Summary label="Оценка эффекта" value={formatImpact(totalImpact)} />
      </div>

      {candidates.length ? (
        <div className="mt-4 grid min-w-0 gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.2fr)]">
          <section className="min-w-0 border border-line bg-panel p-4">
            <h2 className="font-mono text-[10px] uppercase text-accent">
              Ранжированные кандидаты
            </h2>
            <div className="mt-3 grid gap-2">
              {candidates.map((candidate) => (
                <button
                  key={candidate.id}
                  type="button"
                  onClick={() => setSelectedId(candidate.id)}
                  className={`grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-4 border p-4 text-left ${
                    selected?.id === candidate.id
                      ? "border-accent bg-accent/10"
                      : "border-line hover:bg-panel-raised"
                  }`}
                >
                  <span className="min-w-0">
                    <span className="font-mono text-[10px] uppercase text-muted">
                      {candidate.groupCode ?? candidate.groupName}
                    </span>
                    <strong className="mt-1 block break-words text-sm">
                      {candidate.title}
                    </strong>
                    <span className="mt-2 block break-words text-xs leading-5 text-muted">
                      {candidate.impact}
                    </span>
                  </span>
                  <span className="font-mono text-2xl text-accent">
                    {Math.round(candidate.score * 100)}
                  </span>
                </button>
              ))}
            </div>
          </section>

          <section className="min-w-0 border border-line bg-panel-raised p-5">
            {selected ? (
              <>
                <p className="font-mono text-[10px] uppercase text-accent">
                  Расчёт оценки
                </p>
                <h2 className="mt-3 break-words text-xl font-semibold">
                  {selected.title}
                </h2>
                <p className="mt-3 max-w-3xl break-words text-sm leading-7 text-muted">
                  {selected.rationale}
                </p>
                <div className="mt-5 flex flex-wrap gap-2">
                  {(selected.affectedSteps ?? []).map((step) => (
                    <span
                      key={step}
                      className="border border-line px-2 py-1 font-mono text-[10px]"
                    >
                      {step}
                    </span>
                  ))}
                </div>
                <div className="mt-6 grid gap-3 sm:grid-cols-2">
                  {Object.entries(selected.breakdown ?? {})
                    .filter(([, value]) => typeof value !== "object")
                    .map(([name, value]) => (
                      <div key={name} className="border border-line p-4">
                        <p className="text-xs text-muted">
                          {formatTechnicalMetricName(name)}
                        </p>
                        <p className="mt-2 font-mono text-xl">
                          {formatFactor(name, value)}
                        </p>
                      </div>
                    ))}
                </div>
                {selected.breakdown?.factors ? (
                  <BreakdownTable
                    factors={selected.breakdown.factors}
                    weights={selected.breakdown.weights ?? {}}
                  />
                ) : null}
                <div className="mt-6 border-t border-line pt-4 text-xs text-muted">
                  Уверенность {Math.round(selected.confidence * 100)}% · статус{" "}
                  {formatScenarioStatus(selected.status)}. Оценка объясняет приоритет и не запускает
                  автоматизацию автоматически.
                </div>
              </>
            ) : null}
          </section>
        </div>
      ) : (
        <div className="mt-4 border border-line bg-panel p-8 text-sm text-muted">
          Для текущей выборки нет кандидатов выше порога. Группы и их
          оценка автоматизации остаётся доступна на странице сценариев.
        </div>
      )}
    </>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <article className="border border-line bg-panel p-4">
      <p className="font-mono text-[10px] uppercase text-muted">{label}</p>
      <p className="mt-2 text-xl font-semibold">{value}</p>
    </article>
  );
}

function formatImpact(milliseconds: number) {
  const minutes = Math.round(milliseconds / 60000);
  if (minutes < 1) return "< 1 мин";
  if (minutes < 60) return `${minutes} мин`;
  return `${Math.floor(minutes / 60)} ч ${minutes % 60} мин`;
}

function formatRecordingCount(count: number) {
  const remainder100 = count % 100;
  const remainder10 = count % 10;
  if (remainder100 >= 11 && remainder100 <= 14) return `${count} записей`;
  if (remainder10 === 1) return `${count} запись`;
  if (remainder10 >= 2 && remainder10 <= 4) return `${count} записи`;
  return `${count} записей`;
}

function formatFactor(name: string, value: unknown) {
  if (typeof value === "string") return value;
  if (typeof value !== "number") return "—";
  if (name.toLowerCase().includes("ms")) return formatImpact(value);
  if (Math.abs(value) <= 1) return `${Math.round(value * 100)}%`;
  return String(value);
}

function BreakdownTable({
  factors,
  weights,
}: {
  factors: Record<string, number>;
  weights: Record<string, number>;
}) {
  return (
    <div className="mt-6 overflow-x-auto border border-line">
      <table className="w-full min-w-[34rem] text-left text-xs">
        <thead className="bg-panel font-mono uppercase text-muted">
          <tr>
            <th className="px-3 py-2">Фактор</th>
            <th className="px-3 py-2">Значение</th>
            <th className="px-3 py-2">Вес</th>
            <th className="px-3 py-2">Вклад</th>
          </tr>
        </thead>
        <tbody>
          {Object.entries(factors).map(([name, value]) => {
            const weight = Number(weights[name] ?? 0);
            return (
              <tr key={name} className="border-t border-line">
                <td className="px-3 py-2">{formatTechnicalMetricName(name)}</td>
                <td className="px-3 py-2 font-mono">
                  {Math.round(value * 100)}%
                </td>
                <td className="px-3 py-2 font-mono">
                  {Math.round(weight * 100)}%
                </td>
                <td className="px-3 py-2 font-mono">
                  {Math.round(value * weight * 100)}%
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
