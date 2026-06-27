import { PageFrame } from "@/components/page-frame";
import { AutomationWorkbench } from "@/features/automation/automation-workbench";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { apiData, listRecordings } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function AutomationPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const [data, recordings] = await Promise.all([
    params.recordingId
      ? apiData<{ scenarios?: { templates?: ScenarioGroup[] } }>(
          `/v1/recordings/${params.recordingId}/analysis`,
        )
          .then((bundle) => ({
            templates: bundle.scenarios?.templates ?? [],
          }))
          .catch(() => ({ templates: [] }))
      : apiData<{ templates: ScenarioGroup[] }>("/v1/scenarios").catch(() => ({
          templates: [],
        })),
    listRecordings().catch(() => []),
  ]);

  return (
    <PageFrame
      eyebrow="Opportunity scoring"
      title="Кандидаты на автоматизацию"
      description="Приоритеты по частоте, повторяемости, времени, ручным проверкам и снижению ошибок. Кандидаты не исполняются автоматически."
    >
      <AutomationWorkbench
        groups={data.templates}
        recordingCount={params.recordingId ? 1 : recordings.length}
      />
    </PageFrame>
  );
}
