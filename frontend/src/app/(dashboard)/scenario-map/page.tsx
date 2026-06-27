import { DataState } from "@/components/data-state";
import { PageFrame } from "@/components/page-frame";
import {
  ScenarioMapWorkbench,
  type ScenarioGraph,
} from "@/features/scenarios/scenario-map-workbench";
import type { ScenarioGroup } from "@/features/scenarios/types";
import { apiData } from "@/lib/data";

export const dynamic = "force-dynamic";

const emptyGraph: ScenarioGraph = { nodes: [], edges: [] };

type AnalysisPayload = {
  graph?: ScenarioGraph;
  scenarios?: { templates?: ScenarioGroup[] };
};

export default async function ScenarioMapPage({
  searchParams,
}: {
  searchParams: Promise<{ recordingId?: string }>;
}) {
  const params = await searchParams;
  const recordingId = params.recordingId ?? "";
  const analysis = recordingId
    ? await apiData<AnalysisPayload>(
        `/v1/recordings/${recordingId}/analysis`,
      ).catch(() => ({ graph: emptyGraph, scenarios: { templates: [] } }))
    : await apiData<AnalysisPayload>("/v1/project/analysis").catch(() => ({
        graph: emptyGraph,
        scenarios: { templates: [] },
      }));

  return (
    <PageFrame
      eyebrow="Топология процесса"
      title="Карта сценариев"
      description="Фильтруемый граф действий выбранной записи или всего проекта с направленным потоком, масштабированием, фокусом и инспектором переходов."
    >
      <DataState empty={!(analysis.graph ?? emptyGraph).nodes.length}>
        <ScenarioMapWorkbench
          graph={analysis.graph ?? emptyGraph}
          groups={analysis.scenarios?.templates ?? []}
        />
      </DataState>
    </PageFrame>
  );
}
