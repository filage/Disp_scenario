import type { ScenarioGroup } from "@/features/scenarios/types";
import { formatIssueType, formatScenarioStatus } from "@/lib/display";

export function ScenarioMetricsTable({
  groups,
  limit,
}: {
  groups: ScenarioGroup[];
  limit?: number;
}) {
  const rows = typeof limit === "number" ? groups.slice(0, limit) : groups;
  if (!rows.length) {
    return (
      <p className="p-6 text-sm text-muted">
        Группы сценариев появятся после анализа записей.
      </p>
    );
  }
  return (
    <div className="min-w-0 overflow-x-auto">
      <table className="w-full min-w-[70rem] text-left text-xs">
        <thead className="bg-panel-raised font-mono uppercase text-muted">
          <tr>
            <th className="px-3 py-2">Группа</th>
            <th className="px-3 py-2">Тип проблемы</th>
            <th className="px-3 py-2">Сигнатура</th>
            <th className="px-3 py-2">Частота</th>
            <th className="px-3 py-2">Сред / мед / p95</th>
            <th className="px-3 py-2">Ручные проверки</th>
            <th className="px-3 py-2">Уверенность</th>
            <th className="px-3 py-2">Автоматизация</th>
            <th className="px-3 py-2">Статус</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((group) => (
            <tr key={group.id} className="border-t border-line">
              <td className="px-3 py-3">
                <strong className="block text-foreground">{group.name}</strong>
                <span className="font-mono text-[10px] text-muted">
                  {group.code ?? group.id}
                </span>
              </td>
              <td className="px-3 py-3">{formatIssueType(group.issueType)}</td>
              <td className="max-w-72 truncate px-3 py-3 font-mono">
                {group.signature}
              </td>
              <td className="px-3 py-3 font-mono">{group.frequency}</td>
              <td className="px-3 py-3 font-mono">
                {formatMs(group.averageDurationMs)} /{" "}
                {formatMs(group.medianDurationMs)} /{" "}
                {formatMs(group.p95DurationMs)}
              </td>
              <td className="px-3 py-3 font-mono">
                {group.manualCheckCount}
              </td>
              <td className="px-3 py-3 font-mono">
                {Math.round((group.confidenceAverage ?? 0) * 100)}%
              </td>
              <td className="px-3 py-3 font-mono text-accent">
                {Math.round((group.automationScore ?? 0) * 100)}%
              </td>
              <td className="px-3 py-3">
                {formatScenarioStatus(group.status)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatMs(milliseconds = 0) {
  const totalSeconds = Math.max(0, Math.round(milliseconds / 1000));
  return `${Math.floor(totalSeconds / 60)}:${String(totalSeconds % 60).padStart(2, "0")}`;
}
