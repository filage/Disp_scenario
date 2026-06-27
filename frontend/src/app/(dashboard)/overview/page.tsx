import { MetricCard } from "@/components/metric-card";
import { PageFrame } from "@/components/page-frame";
import { StatusBadge } from "@/components/data-state";
import { ScenarioMetricsTable } from "@/features/scenarios/scenario-metrics-table";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { getSystemSnapshot } from "@/lib/api";
import {
  type AnalysisRun,
  apiData,
  listRuns,
  type Recording,
} from "@/lib/data";

export const dynamic = "force-dynamic";

type ScenarioInstance = {
  id: string;
  startedAtMs: number;
  endedAtMs: number;
};

type ProjectBundle = {
  events?: { confidence: number }[];
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
    ).catch(
      () => ({} as ProjectBundle),
    ),
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
  const activeRuns = runs.filter((run) =>
    ["QUEUED", "PROCESSING", "NORMALIZING"].includes(run.status),
  ).length;

  return (
    <PageFrame
      eyebrow="Обзор аналитики"
      title="Обзор аналитики"
      description="Сводка по записям, сценариям, качеству данных и кандидатам на автоматизацию."
    >
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
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
          label="Уверенность"
          value={`${confidence}%`}
          detail="Средняя по нормализованным событиям"
        />
        <MetricCard
          label="Предупреждения"
          value={qualityIssues.length}
          detail="Неразрешённые проблемы качества"
          tone={qualityIssues.length ? "danger" : "default"}
        />
      </section>

      <section className="mt-6 grid gap-4 xl:grid-cols-3">
        <ChartPanel title="Сценарии со временем">
          <MiniBars items={buildScenarioTrend(instances)} />
        </ChartPanel>
        <ChartPanel title="Средняя длительность">
          <MiniBars items={buildDurationBars(groups)} />
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
        <div className="flex items-center justify-between p-5">
          <h2 className="text-sm font-semibold">Топ узких мест</h2>
          <span className="font-mono text-[10px] uppercase text-muted">
            область: проект
          </span>
        </div>
        <ScenarioMetricsTable groups={groups} limit={8} />
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

function MiniBars({
  items,
}: {
  items: { label: string; value: number; height: number }[];
}) {
  if (!items.length) {
    return <p className="mt-8 text-sm text-muted">Данных пока нет.</p>;
  }
  return (
    <div className="mt-6 flex h-40 items-end gap-2">
      {items.map((item, index) => (
        <div
          key={`${item.label}-${index}`}
          className="group relative min-w-0 flex-1"
          title={`${item.label}: ${item.value}`}
        >
          <div
            className="min-h-2 bg-accent/75 transition-colors group-hover:bg-accent"
            style={{ height: `${item.height * 1.4}px` }}
          />
          <span className="mt-2 block truncate font-mono text-[9px] text-muted">
            {item.label}
          </span>
        </div>
      ))}
    </div>
  );
}

function buildScenarioTrend(instances: ScenarioInstance[]) {
  const safe = instances.filter((item) => Number.isFinite(item.startedAtMs));
  if (!safe.length) return [];
  const minimum = Math.min(...safe.map((item) => item.startedAtMs));
  const maximum = Math.max(
    ...safe.map((item) =>
      Number.isFinite(item.endedAtMs) ? item.endedAtMs : item.startedAtMs,
    ),
  );
  const bucketCount = Math.min(7, Math.max(1, safe.length));
  const bucketSize = Math.max(1, (maximum - minimum + 1) / bucketCount);
  const buckets = Array.from({ length: bucketCount }, (_, index) => ({
    label: `${formatMinutes(minimum + bucketSize * index)}–${formatMinutes(minimum + bucketSize * (index + 1))}`,
    value: 0,
  }));
  for (const instance of safe) {
    const index = Math.min(
      bucketCount - 1,
      Math.floor((instance.startedAtMs - minimum) / bucketSize),
    );
    buckets[index].value += 1;
  }
  return scaleItems(buckets);
}

function buildDurationBars(groups: ScenarioGroup[]) {
  return scaleItems(
    groups
      .map((group) => ({
        label: group.code ?? group.name,
        value: group.averageDurationMs || group.medianDurationMs || 0,
      }))
      .filter((item) => item.value > 0)
      .sort((a, b) => b.value - a.value)
      .slice(0, 7),
  );
}

function scaleItems(items: { label: string; value: number }[]) {
  const maximum = Math.max(...items.map((item) => item.value), 0);
  if (!maximum) return [];
  return items.map((item) => ({
    ...item,
    height: Math.max(8, Math.round((item.value / maximum) * 100)),
  }));
}

function formatMinutes(milliseconds: number) {
  return `${Math.max(0, Math.round(milliseconds / 60000))} мин`;
}
