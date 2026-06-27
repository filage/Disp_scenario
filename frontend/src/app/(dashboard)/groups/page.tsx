import { DataState } from "@/components/data-state";
import { PageFrame } from "@/components/page-frame";
import { GroupBrowser } from "@/features/scenarios/group-browser";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { apiData } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function GroupsPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const data = params.recordingId
    ? await apiData<{ scenarios?: { templates?: ScenarioGroup[] } }>(
        `/v1/recordings/${params.recordingId}/analysis`,
      )
        .then((bundle) => ({
          templates: bundle.scenarios?.templates ?? [],
        }))
        .catch(() => ({ templates: [] }))
    : await apiData<{ templates: ScenarioGroup[] }>("/v1/scenarios").catch(
        () => ({ templates: [] }),
      );

  return (
    <PageFrame
      eyebrow="Сигнатуры"
      title="Группы сценариев"
      description="Поиск, фильтрация кандидатов и сравнение сигнатур, частоты, длительности, ручных действий и оценки автоматизации."
    >
      <DataState empty={data.templates.length === 0}>
        <GroupBrowser groups={data.templates} />
      </DataState>
    </PageFrame>
  );
}
