import { DataState } from "@/components/data-state";
import { MutationButton } from "@/components/mutation-button";
import { PageFrame } from "@/components/page-frame";
import { TimelineWorkbench } from "@/features/events/timeline-workbench";
import type { ActionEvent, AnalysisBundle } from "@/features/events/types";
import { apiData, listRecordings, publicApiUrl } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function TimelinePage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordings = await listRecordings().catch(() => []);
  const recordingId = params.recordingId ?? recordings[0]?.id;
  const bundle = recordingId
    ? await apiData<AnalysisBundle>(
        `/v1/recordings/${recordingId}/analysis`,
      ).catch(() => ({
        events: [] as ActionEvent[],
        rawEvents: [],
        dataQualityIssues: [],
        scenarios: { instances: [] },
      }))
    : {
        events: [],
        rawEvents: [],
        dataQualityIssues: [],
        scenarios: { instances: [] },
      };

  return (
    <PageFrame
      eyebrow="Канонические события"
      title="Таймлайн"
      description="Индекс, детальная таблица и QA-редактор нормализованных событий с привязкой к сырым наблюдениям."
    >
      {recordingId ? (
        <div className="mt-6 flex flex-wrap gap-2">
          <MutationButton
            path={`/v1/recordings/${recordingId}/rebuild`}
            tone="accent"
          >
            Пересобрать
          </MutationButton>
          <MutationButton path={`/v1/recordings/${recordingId}/renormalize`}>
            Нормализовать сырые события
          </MutationButton>
        </div>
      ) : null}
      <DataState empty={bundle.events.length === 0}>
        {recordingId ? (
          <TimelineWorkbench
            recordingId={recordingId}
            events={bundle.events}
            rawEvents={bundle.rawEvents}
            scenarioInstances={bundle.scenarios?.instances ?? []}
            exportHref={publicApiUrl(
              `/v1/recordings/${recordingId}/exports/timeline.csv`,
            )}
          />
        ) : null}
      </DataState>
    </PageFrame>
  );
}
