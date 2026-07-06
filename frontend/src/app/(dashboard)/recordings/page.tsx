import { PageFrame } from "@/components/page-frame";
import { RecordingBrowser } from "@/features/recordings/recording-browser";
import { listRecordings, publicApiUrl } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function RecordingsPage() {
  const { recordings, recordingsError } = await listRecordings()
    .then((items) => ({ recordings: items, recordingsError: "" }))
    .catch((error: unknown) => ({
      recordings: [],
      recordingsError:
        error instanceof Error ? error.message : "Recordings API unavailable",
    }));

  return (
    <PageFrame
      eyebrow="Source media"
      title="Управление записями"
      description="Загрузка и анализ видеоматериалов диспетчерских сценариев."
    >
      <RecordingBrowser
        recordings={recordings}
        recordingsError={recordingsError}
        playbackBase={publicApiUrl("/v1/recordings")}
        exportBase={publicApiUrl("/v1/recordings")}
      />
    </PageFrame>
  );
}
