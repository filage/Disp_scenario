import { DataState } from "@/components/data-state";
import { PageFrame } from "@/components/page-frame";
import { ScenarioGuide } from "@/features/events/scenario-guide";
import type { ActionEvent, AnalysisBundle } from "@/features/events/types";
import { apiData, listRecordings, publicApiUrl } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function GuidePage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordings = await listRecordings().catch(() => []);
  const recordingId = params.recordingId ?? recordings[0]?.id;
  const recording = recordings.find((item) => item.id === recordingId);
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
      eyebrow="Пользовательская инструкция"
      title="Как выполнялся сценарий"
      description={`Пошаговый разбор записи${recording?.originalName ? ` «${recording.originalName}»` : ""}: понятные действия, время и кадры ключевых моментов.`}
    >
      <DataState empty={!recordingId || bundle.events.length === 0}>
        {recordingId ? (
          <ScenarioGuide
            events={bundle.events}
            scenarios={bundle.scenarios?.instances ?? []}
            evidenceBaseHref={publicApiUrl(
              `/v1/recordings/${recordingId}/evidence`,
            )}
          />
        ) : null}
      </DataState>
    </PageFrame>
  );
}
