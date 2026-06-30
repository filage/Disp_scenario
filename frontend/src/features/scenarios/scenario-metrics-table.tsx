import type { ScenarioGroup } from "@/features/scenarios/types";
import {
  formatDuration,
  formatIssueType,
  formatScenarioStatus,
} from "@/lib/display";

export function ScenarioMetricsTable({
  groups,
  limit,
  compact = false,
}: {
  groups: ScenarioGroup[];
  limit?: number;
  compact?: boolean;
}) {
  const rows = typeof limit === "number" ? groups.slice(0, limit) : groups;
  if (!rows.length) {
    return (
      <p className="p-6 text-sm text-muted">
        Группы сценариев появятся после анализа записей.
      </p>
    );
  }
  if (compact) {
    return (
      <div className="min-w-0 overflow-x-auto">
        <table className="w-full min-w-[44rem] text-left text-sm">
          <thead className="border-y border-line bg-panel-raised text-xs font-medium text-muted">
            <tr>
              <th className="px-4 py-3">Сценарий</th>
              <th className="px-4 py-3">Повторений</th>
              <th className="px-4 py-3">Среднее время</th>
              <th className="px-4 py-3">Ручные проверки</th>
              <th className="px-4 py-3">Потенциал автоматизации</th>
              <th className="px-4 py-3">Статус</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((group) => (
              <tr key={group.id} className="border-b border-line">
                <td className="px-4 py-4">
                  <strong className="block text-foreground">
                    {group.name}
                  </strong>
                  <span className="mt-1 block text-xs text-muted">
                    {formatIssueType(group.issueType)}
                  </span>
                </td>
                <td className="px-4 py-4 font-mono">{group.frequency}</td>
                <td className="px-4 py-4">
                  {formatDuration(group.averageDurationMs)}
                </td>
                <td className="px-4 py-4 font-mono">
                  {group.manualCheckCount}
                </td>
                <td className="px-4 py-4">
                  <div className="flex items-center gap-3">
                    <div className="h-1.5 w-20 overflow-hidden rounded-full bg-line/60">
                      <div
                        className="h-full rounded-full bg-accent"
                        style={{
                          width: `${Math.round((group.automationScore ?? 0) * 100)}%`,
                        }}
                      />
                    </div>
                    <span className="font-mono text-xs text-accent">
                      {Math.round((group.automationScore ?? 0) * 100)}%
                    </span>
                  </div>
                </td>
                <td className="px-4 py-4">
                  {formatScenarioStatus(group.status)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
              <td className="px-3 py-3 font-mono">{group.manualCheckCount}</td>
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
