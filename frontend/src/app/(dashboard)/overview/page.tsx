import { MetricCard } from "@/components/metric-card";
import { PageFrame } from "@/components/page-frame";
import { StatusBadge } from "@/components/data-state";
import { ScenarioMetricsTable } from "@/features/scenarios/scenario-metrics-table";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { getSystemSnapshot } from "@/lib/api";
import { formatDuration } from "@/lib/display";
import {
  type AnalysisRun,
  apiData,
  listRuns,
  type Recording,
} from "@/lib/data";

export const dynamic = "force-dynamic";

type ScenarioInstance = { id: string };

type ProjectBundle = {
  events?: { confidence: number; qaStatus?: string }[];
  dataQualityIssues?: { resolved: boolean }[];
  scenarios?: {
    instances?: ScenarioInstance[];
    templates?: ScenarioGroup[];
  };
};

export default async function OverviewPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const [snapshot, runs, project] = await Promise.all([
    getSystemSnapshot().catch(() => ({
      health: null,
      recordings: [] as Recording[],
      unavailable: true,
    })),
    listRuns(params.recordingId).catch(() => [] as AnalysisRun[]),
    apiData<ProjectBundle>(
      params.recordingId
        ? `/v1/recordings/${params.recordingId}/analysis`
        : "/v1/project/analysis",
    ).catch(() => ({}) as ProjectBundle),
  ]);
  const recordings = snapshot.recordings ?? [];
  const events = project.events ?? [];
  const instances = project.scenarios?.instances ?? [];
  const groups = project.scenarios?.templates ?? [];
  const qualityIssues = (project.dataQualityIssues ?? []).filter(
    (issue) => !issue.resolved,
  );
  const candidates = groups.reduce(
    (sum, group) => sum + (group.automationCandidates?.length ?? 0),
    0,
  );
  const confidence = events.length
    ? Math.round(
        (events.reduce((sum, event) => sum + (event.confidence ?? 0), 0) /
          events.length) *
          100,
      )
    : 0;
  const reviewedEvents = events.filter((event) =>
    ["confirmed", "edited", "reviewed"].includes(event.qaStatus ?? ""),
  ).length;
  const activeRuns = runs.filter((run) =>
    ["QUEUED", "PROCESSING", "NORMALIZING"].includes(run.status),
  ).length;

  return (
    <PageFrame
      eyebrow="Обзор аналитики"
      title="Обзор аналитики"
      description="Сводка по записям, сценариям, качеству данных и кандидатам на автоматизацию."
    >
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <MetricCard
          label="Проанализировано"
          value={recordings.filter((item) => item.status === "ANALYZED").length}
          detail="Записей со статусом «проанализировано»"
        />
        <MetricCard
          label="Группы"
          value={groups.length}
          detail="Шаблоны сценариев проекта"
        />
        <MetricCard
          label="Экземпляры"
          value={instances.length}
          detail="Распознанные выполнения сценариев"
        />
        <MetricCard
          label="Кандидаты"
          value={candidates}
          detail="Оценка не ниже 30%"
          tone="accent"
        />
        <MetricCard
          label="Оценка модели"
          value={`${confidence}%`}
          detail={
            confidence >= 99
              ? `Некалиброванная самооценка по ${events.length} событиям — не равна точности`
              : `Средняя самооценка по ${events.length} нормализованным событиям`
          }
          tone={confidence >= 99 ? "danger" : "default"}
        />
        <MetricCard
          label="Проверено человеком"
          value={`${reviewedEvents}/${events.length}`}
          detail={`${qualityIssues.length} открытых замечаний качества`}
          tone={qualityIssues.length ? "danger" : "accent"}
        />
      </section>

      <section className="mt-6 grid gap-4 xl:grid-cols-3">
        <ChartPanel title="Самые частые сценарии">
          <RankedScenarios groups={groups} mode="frequency" />
        </ChartPanel>
        <ChartPanel title="Где уходит больше времени">
          <RankedScenarios groups={groups} mode="duration" />
        </ChartPanel>
        <article className="border border-line bg-panel p-5">
          <h2 className="text-sm font-semibold">Недавние запуски</h2>
          <div className="mt-4 divide-y divide-line">
            {runs.slice(0, 5).map((run) => (
              <div
                key={run.id}
                className="grid grid-cols-[1fr_auto] gap-3 py-3 text-xs"
              >
                <span>
                  <strong className="block font-mono">
                    {run.id.slice(0, 8)}
                  </strong>
                  <span className="text-muted">
                    {run.provider}/{run.model ?? "модель"} · {run.promptVersion}
                  </span>
                </span>
                <StatusBadge value={run.status} />
              </div>
            ))}
            {!runs.length ? (
              <p className="py-6 text-sm text-muted">Запусков пока нет.</p>
            ) : null}
          </div>
        </article>
      </section>

      <section className="mt-6 border border-line bg-panel">
        <div className="flex flex-wrap items-start justify-between gap-3 p-5">
          <div>
            <h2 className="text-base font-semibold">Повторяющиеся сценарии</h2>
            <p className="mt-1 max-w-3xl text-xs leading-5 text-muted">
              Сводка по всем выбранным записям. Потенциал автоматизации —
              расчётный приоритет, а не процент уже выполненной автоматизации.
              Для каждого сценария показаны сигнатура, среднее, медиана, p95 и
              оценка модели.
            </p>
          </div>
          <span className="text-xs text-muted">Область: весь проект</span>
        </div>
        <ScenarioMetricsTable groups={groups} limit={8} compact />
      </section>

      <section className="mt-6 grid gap-3 sm:grid-cols-3">
        <MetricCard
          label="API"
          value={snapshot.health?.status ?? "недоступно"}
          detail="Readiness основного Go API"
          tone={snapshot.unavailable ? "danger" : "accent"}
        />
        <MetricCard
          label="Очередь"
          value={activeRuns}
          detail="В очереди / обработка / нормализация"
        />
        <MetricCard
          label="Всего записей"
          value={recordings.length}
          detail="Включая загруженные и ошибочные"
        />
      </section>
    </PageFrame>
  );
}

function ChartPanel({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <article className="border border-line bg-panel p-5">
      <h2 className="text-sm font-semibold">{title}</h2>
      {children}
    </article>
  );
}

function RankedScenarios({
  groups,
  mode,
}: {
  groups: ScenarioGroup[];
  mode: "frequency" | "duration";
}) {
  const rows = [...groups]
    .filter((group) =>
      mode === "frequency" ? group.frequency > 0 : group.averageDurationMs > 0,
    )
    .sort((left, right) =>
      mode === "frequency"
        ? right.frequency - left.frequency
        : right.averageDurationMs - left.averageDurationMs,
    )
    .slice(0, 5);
  const maximum = Math.max(
    ...rows.map((group) =>
      mode === "frequency" ? group.frequency : group.averageDurationMs,
    ),
    1,
  );
  if (!rows.length)
    return <p className="mt-8 text-sm text-muted">Данных пока нет.</p>;
  return (
    <ol className="mt-5 grid gap-4">
      {rows.map((group) => {
        const value =
          mode === "frequency" ? group.frequency : group.averageDurationMs;
        return (
          <li key={group.id}>
            <div className="flex items-baseline justify-between gap-3 text-xs">
              <span className="truncate font-medium">{group.name}</span>
              <span className="shrink-0 text-muted">
                {mode === "frequency" ? `${value} раз` : formatDuration(value)}
              </span>
            </div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-line/50">
              <div
                className="h-full rounded-full bg-accent"
                style={{ width: `${Math.max(6, (value / maximum) * 100)}%` }}
              />
            </div>
          </li>
        );
      })}
    </ol>
  );
}
