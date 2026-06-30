import { PageFrame } from "@/components/page-frame";
import {
  SettingsEditor,
  type GeminiCredential,
  type SettingsData,
} from "@/features/settings/settings-editor";
import { apiData } from "@/lib/data";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const [settings, geminiCredential] = await Promise.all([
    apiData<SettingsData>("/v1/settings").catch(() => ({
      versions: {},
      knownScenarios: [],
      boundaryRules: [],
      actionCatalog: [],
      dataQualityFlags: [],
    })),
    apiData<GeminiCredential>("/v1/settings/gemini-credential").catch(() => ({
      provider: "gemini",
      configured: false,
    })),
  ]);

  return (
    <PageFrame
      eyebrow="Runtime policy"
      title="Настройки системы"
      description="Редактирование известных сценариев и правил границ, версии промптов, каталог канонических действий и флаги качества данных."
    >
      <SettingsEditor settings={settings} geminiCredential={geminiCredential} />
    </PageFrame>
  );
}
