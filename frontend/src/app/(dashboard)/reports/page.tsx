import { DataState } from "@/components/data-state";
import { PageFrame } from "@/components/page-frame";
import { ScenarioMetricsTable } from "@/features/scenarios/scenario-metrics-table";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { apiData, publicApiUrl } from "@/lib/data";
import { Download, FileJson } from "lucide-react";

export const dynamic = "force-dynamic";

type Report = {
  id: string;
  summary: string;
  observations: string[];
  recommendations: string[];
  metrics: Record<string, unknown>;
  graphSummary?: Record<string, unknown>;
  model: string;
  provider?: string;
  promptVersion?: string;
  normalizationVersion?: string;
  groupingVersion?: string;
};

type AnalysisBundle = {
  report: Report | null;
  scenarios?: { templates?: ScenarioGroup[] };
};

export default async function ReportsPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordingId = params.recordingId;
  const bundle: AnalysisBundle = await apiData<AnalysisBundle>(
    recordingId
      ? `/v1/recordings/${recordingId}/analysis`
      : "/v1/project/analysis",
  ).catch(() => ({ report: null }));
  const report = bundle.report;
  const groups = bundle.scenarios?.templates ?? [];

  return (
    <PageFrame
      eyebrow="Поддержка решений"
      title="Отчет по анализу сценариев"
      description="Экспортируемая сводка, наблюдения, рекомендации, метаданные снимка и метрики сценариев."
    >
      <DataState empty={!report}>
        {report ? (
          <section className="mt-6 grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
            <ReportPanel title="Сводка AI-аналитика">
              <p className="text-sm font-semibold leading-6">
                {report.summary}
              </p>
              <ReportList title="Наблюдения" items={report.observations} />
              <ReportList title="Рекомендации" items={report.recommendations} />

              <h3 className="mt-7 text-sm font-semibold">
                Метаданные снимка отчёта
              </h3>
              <dl className="mt-3 grid gap-x-8 gap-y-2 text-xs sm:grid-cols-[11rem_minmax(0,1fr)]">
                <Metadata label="Провайдер" value={report.provider} />
                <Metadata label="Модель" value={report.model} />
                <Metadata label="Версия промпта" value={report.promptVersion} />
                <Metadata
                  label="Версия нормализации"
                  value={report.normalizationVersion}
                />
                <Metadata
                  label="Версия группировки"
                  value={report.groupingVersion}
                />
              </dl>

              <details className="mt-6 border-t border-line pt-4 text-xs">
                <summary className="cursor-pointer font-semibold">
                  Технические метрики
                </summary>
                <dl className="mt-4 grid gap-x-8 gap-y-2 sm:grid-cols-[11rem_minmax(0,1fr)]">
                  {Object.entries(report.metrics ?? {}).map(([key, value]) => (
                    <Metadata key={key} label={key} value={value} />
                  ))}
                  {Object.entries(report.graphSummary ?? {}).map(
                    ([key, value]) => (
                      <Metadata
                        key={`graph-${key}`}
                        label={`graph.${key}`}
                        value={value}
                      />
                    ),
                  )}
                </dl>
              </details>
            </ReportPanel>

            <ReportPanel title="Экспорт">
              <div className="grid gap-2">
                {recordingId
                  ? (
                      [
                        ["timeline", "json", "Таймлайн JSON", FileJson],
                        ["timeline", "csv", "Таймлайн CSV", Download],
                        ["report", "json", "Отчет JSON", FileJson],
                        ["report", "csv", "Отчет CSV", Download],
                      ] as const
                    ).map(([kind, format, label, Icon]) => (
                      <a
                        key={`${kind}-${format}`}
                        href={publicApiUrl(
                          `/v1/recordings/${recordingId}/exports/${kind}.${format}`,
                        )}
                        className="flex min-h-10 items-center gap-2 border border-line px-3 text-sm text-foreground transition-colors hover:border-accent hover:text-accent active:translate-y-px"
                      >
                        <Icon aria-hidden="true" size={16} strokeWidth={1.75} />
                        {label}
                      </a>
                    ))
                  : null}
              </div>
            </ReportPanel>
          </section>
        ) : null}
      </DataState>

      <section className="mt-6 border border-line bg-panel">
        <div className="p-5">
          <h2 className="text-sm font-semibold">Метрики сценариев</h2>
        </div>
        <ScenarioMetricsTable groups={groups} />
      </section>
    </PageFrame>
  );
}

function ReportPanel({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="border border-line bg-panel">
      <header className="border-b border-line px-5 py-3">
        <h2 className="text-sm font-semibold">{title}</h2>
      </header>
      <div className="p-5">{children}</div>
    </section>
  );
}

function ReportList({ title, items }: { title: string; items: string[] }) {
  return (
    <>
      <h3 className="mt-7 text-sm font-semibold">{title}</h3>
      <div className="mt-2 space-y-2 text-sm leading-6 text-muted">
        {(items ?? []).map((item, index) => (
          <p key={`${title}-${index}`}>{item}</p>
        ))}
      </div>
    </>
  );
}

function Metadata({ label, value }: { label: string; value: unknown }) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0 break-words font-mono">{formatMetric(value)}</dd>
    </>
  );
}

function formatMetric(value: unknown) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
