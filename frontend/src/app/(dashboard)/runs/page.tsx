import { MutationButton } from "@/components/mutation-button";
import { PageFrame } from "@/components/page-frame";
import { StatusBadge } from "@/components/data-state";
import { listRecordings, listRuns } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function RunsPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordings = await listRecordings().catch(() => []);
  const recordingId = params.recordingId ?? recordings[0]?.id;
  const runs = await listRuns(recordingId).catch(() => []);

  return (
    <PageFrame
      eyebrow="Выполнение"
      title="Запуски анализа"
      description="Воспроизводимость обработки: провайдер и модель, версии, сырой ответ, временные метки, повторы, отмена и ошибки."
    >
      <div className="mt-6 overflow-x-auto border border-line bg-panel">
        <table className="w-full min-w-[84rem] text-left text-xs">
          <thead className="border-b border-line bg-panel-raised font-mono text-[10px] uppercase text-muted">
            <tr>
              <th className="px-4 py-3">Запуск / запись</th>
              <th className="px-4 py-3">Провайдер</th>
              <th className="px-4 py-3">Версии</th>
              <th className="px-4 py-3">Статус</th>
              <th className="px-4 py-3">Создан</th>
              <th className="px-4 py-3">Запущен</th>
              <th className="px-4 py-3">Завершён</th>
              <th
                className="px-4 py-3"
                title="Расчётная стоимость по тарифу, сохранённому на момент анализа"
              >
                Стоимость
              </th>
              <th className="px-4 py-3">Сырой ответ / ошибка</th>
              <th className="px-4 py-3">Действия</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.id} className="border-b border-line align-top">
                <td className="px-4 py-4">
                  <strong className="block font-mono">{run.id}</strong>
                  <span className="mt-1 block font-mono text-[10px] text-muted">
                    запись: {run.recordingId}
                  </span>
                </td>
                <td className="px-4 py-4">
                  {run.provider}/{run.model ?? "модель по умолчанию"}
                </td>
                <td className="px-4 py-4 font-mono text-[10px] leading-5 text-muted">
                  <span className="block">P {run.promptVersion}</span>
                  <span className="block">N {run.normalizationVersion}</span>
                  <span className="block">G {run.groupingVersion}</span>
                </td>
                <td className="px-4 py-4">
                  <StatusBadge value={run.status} />
                </td>
                <td className="px-4 py-4 text-muted">
                  {formatDate(run.createdAt)}
                </td>
                <td className="px-4 py-4 text-muted">
                  {formatDate(run.startedAt)}
                </td>
                <td className="px-4 py-4 text-muted">
                  {formatDate(run.completedAt)}
                </td>
                <td className="px-4 py-4" title={formatUsageDetails(run)}>
                  <strong className="block whitespace-nowrap font-mono text-foreground">
                    {formatEstimatedCost(run.estimatedCostUsd)}
                  </strong>
                  <span className="mt-1 block whitespace-nowrap font-mono text-[10px] text-muted">
                    {formatTokenCount(run.totalTokens)}
                  </span>
                </td>
                <td className="max-w-sm px-4 py-4">
                  <span
                    className={`font-mono text-[10px] ${
                      run.rawText ? "text-accent" : "text-muted"
                    }`}
                  >
                    сырой ответ: {run.rawText ? "сохранён" : "нет данных"}
                  </span>
                  {run.error ? (
                    <pre className="mt-2 whitespace-pre-wrap text-xs text-danger">
                      {run.error}
                    </pre>
                  ) : null}
                </td>
                <td className="px-4 py-4">
                  <div className="flex flex-wrap gap-2">
                    {["QUEUED", "PROCESSING", "NORMALIZING"].includes(
                      run.status,
                    ) ? (
                      <MutationButton
                        path={`/v1/analysis-runs/${run.id}/cancel`}
                        tone="danger"
                      >
                        Отменить
                      </MutationButton>
                    ) : null}
                    {["FAILED", "CANCELLED"].includes(run.status) ? (
                      <MutationButton
                        path={`/v1/analysis-runs/${run.id}/retry`}
                        tone="accent"
                      >
                        Повторить
                      </MutationButton>
                    ) : null}
                    {run.status === "COMPLETED" ? (
                      <MutationButton
                        path={`/v1/recordings/${run.recordingId}/analysis-runs`}
                        tone="accent"
                      >
                        Запустить повторно
                      </MutationButton>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!runs.length ? (
          <p className="p-8 text-sm text-muted">
            Запустите анализ, чтобы увидеть воспроизводимость и жизненный цикл.
          </p>
        ) : null}
      </div>
    </PageFrame>
  );
}

function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString("ru-RU") : "—";
}

function formatEstimatedCost(value?: number | null) {
  if (value == null) return "—";
  return `≈ ${new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 4,
    maximumFractionDigits: 6,
  }).format(value)}`;
}

function formatTokenCount(value?: number | null) {
  if (value == null) return "нет данных";
  return `${new Intl.NumberFormat("ru-RU").format(value)} токенов`;
}

function formatUsageDetails(run: {
  inputTokens?: number | null;
  outputTokens?: number | null;
  thinkingTokens?: number | null;
  pricingVersion?: string | null;
}) {
  const details = [
    run.inputTokens == null ? null : `вход: ${formatInteger(run.inputTokens)}`,
    run.outputTokens == null
      ? null
      : `ответ: ${formatInteger(run.outputTokens)}`,
    run.thinkingTokens == null
      ? null
      : `thinking: ${formatInteger(run.thinkingTokens)}`,
    run.pricingVersion ? `тариф: ${run.pricingVersion}` : null,
  ].filter(Boolean);
  return details.length
    ? details.join("; ")
    : "Данные об использовании недоступны";
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("ru-RU").format(value);
}
