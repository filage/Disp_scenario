import { PageFrame } from "@/components/page-frame";
import { RecordingBrowser } from "@/features/recordings/recording-browser";
import { listRecordings, publicApiUrl } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function RecordingsPage() {
  const recordings = await listRecordings().catch(() => []);

  return (
    <PageFrame
      eyebrow="Source media"
      title="Управление записями"
      description="Загрузка и анализ видеоматериалов диспетчерских сценариев."
    >
      <RecordingBrowser
        recordings={recordings}
        playbackBase={publicApiUrl("/v1/recordings")}
        exportBase={publicApiUrl("/v1/recordings")}
      />
    </PageFrame>
  );
}
