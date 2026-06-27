import { DataState } from "@/components/data-state";
import { PageFrame } from "@/components/page-frame";
import type { ActionEvent, AnalysisBundle } from "@/features/events/types";
import { QAWorkbench } from "@/features/qa/qa-workbench";
import { apiData, listRecordings, listRuns, publicApiUrl } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function QAPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordings = await listRecordings().catch(() => []);
  const recordingId = params.recordingId ?? recordings[0]?.id;
  const [bundle, playback, runs] = recordingId
    ? await Promise.all([
        apiData<AnalysisBundle>(`/v1/recordings/${recordingId}/analysis`).catch(
          () => ({
            events: [] as ActionEvent[],
            rawEvents: [],
            dataQualityIssues: [],
            scenarios: { instances: [] },
          }),
        ),
        apiData<{ url: string }>(
          `/v1/recordings/${recordingId}/playback`,
        ).catch(() => null),
        listRuns(recordingId).catch(() => []),
      ])
    : [
        {
          events: [],
          rawEvents: [],
          dataQualityIssues: [],
          scenarios: { instances: [] },
        },
        null,
        [],
      ];

  return (
    <PageFrame
      eyebrow="Evidence review"
      title={`Разбор сессии: ${(runs[0]?.id ?? "TRK-882-NL").slice(0, 8)}`}
      description="Видео, кадры-доказательства, сырые наблюдения, редактирование канонических событий, контроль качества и пересборка производных артефактов."
    >
      <DataState empty={!recordingId || bundle.events.length === 0}>
        {recordingId ? (
          <QAWorkbench
            recordingId={recordingId}
            events={bundle.events}
            rawEvents={bundle.rawEvents}
            issues={bundle.dataQualityIssues}
            scenarioInstances={bundle.scenarios?.instances ?? []}
            runId={runs[0]?.id}
            playbackHref={playback?.url ?? ""}
            evidenceBaseHref={publicApiUrl(
              `/v1/recordings/${recordingId}/evidence`,
            )}
          />
        ) : null}
      </DataState>
    </PageFrame>
  );
}
