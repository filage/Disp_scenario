"use client";

import { BookOpen, Play, RefreshCw } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

const routeLabels: Record<string, string> = {
  "/overview": "Обзор",
  "/recordings": "Записи",
  "/runs": "Запуски анализа",
  "/timeline": "Таймлайн",
  "/guide": "Гайд по сценарию",
  "/scenario-map": "Карта сценариев",
  "/groups": "Группы сценариев",
  "/qa": "QA-проверка",
  "/automation": "Автоматизация",
  "/reports": "Отчеты",
  "/settings": "Настройки",
};
const aggregateRoutes = new Set([
  "/overview",
  "/scenario-map",
  "/groups",
  "/automation",
  "/reports",
]);

type RecordingOption = {
  id: string;
  originalName: string;
};

export function TopBar() {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [recordings, setRecordings] = useState<RecordingOption[]>([]);
  const [analyzing, setAnalyzing] = useState(false);
  const selected = searchParams.get("recordingId") ?? "";
  const allowAll = aggregateRoutes.has(pathname);
  const activeRecording =
    selected || (allowAll ? "" : (recordings[0]?.id ?? ""));

  useEffect(() => {
    let active = true;
    void fetch("/api/backend/v1/recordings", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error(`Recordings HTTP ${response.status}`);
        return (await response.json()) as { items?: RecordingOption[] };
      })
      .then((payload) => {
        if (active) setRecordings(payload.items ?? []);
      })
      .catch(() => {
        if (active) setRecordings([]);
      });
    return () => {
      active = false;
    };
  }, []);

  function selectRecording(recordingId: string) {
    const query = new URLSearchParams(searchParams.toString());
    if (recordingId) query.set("recordingId", recordingId);
    else query.delete("recordingId");
    router.push(`${pathname}${query.size ? `?${query}` : ""}`);
  }

  async function analyzeSelected() {
    if (!activeRecording || analyzing) return;
    setAnalyzing(true);
    try {
      await fetch(
        `/api/backend/v1/recordings/${activeRecording}/analysis-runs`,
        {
          method: "POST",
        },
      );
      router.refresh();
    } finally {
      setAnalyzing(false);
    }
  }

  return (
    <header className="sticky top-0 z-20 flex h-12 items-center justify-between gap-4 border-b border-line bg-panel px-4 md:px-6">
      <div className="flex min-w-0 items-center gap-4">
        <label className="flex h-8 min-w-0 items-center gap-2 rounded-md border border-[#b8c7dc] bg-[#f6f9fd] px-2">
          <span className="hidden text-[10px] font-semibold uppercase tracking-wide text-muted xl:inline">
            Запись
          </span>
          <select
            aria-label="Текущая запись"
            value={activeRecording}
            onChange={(event) => selectRecording(event.target.value)}
            className="w-[clamp(9rem,24vw,22rem)] min-w-0 bg-transparent text-sm font-semibold text-[#122038] outline-none"
          >
            {allowAll ? <option value="">Все записи</option> : null}
            {recordings.map((recording) => (
              <option key={recording.id} value={recording.id}>
                {recording.originalName}
              </option>
            ))}
          </select>
        </label>
        <span className="hidden whitespace-nowrap text-sm text-muted md:inline">
          {routeLabels[pathname] ?? "Обзор"}
        </span>
      </div>
      <div className="flex items-center gap-1 text-muted">
        <button
          type="button"
          disabled={!activeRecording}
          onClick={() => router.push(`/guide?recordingId=${activeRecording}`)}
          className="mr-2 hidden h-8 items-center gap-2 rounded-sm border border-line bg-panel px-3 text-sm hover:border-accent hover:text-accent disabled:cursor-not-allowed disabled:opacity-45 lg:flex"
        >
          <BookOpen size={15} />
          Гайд по записи
        </button>
        <button
          type="button"
          title="Обновить"
          onClick={() => router.refresh()}
          className="grid size-8 place-items-center rounded-sm hover:bg-panel-raised hover:text-accent active:translate-y-px"
        >
          <RefreshCw size={16} />
        </button>
        <button
          type="button"
          disabled={!activeRecording || analyzing}
          onClick={analyzeSelected}
          className="ml-2 hidden h-8 items-center gap-2 rounded-sm bg-accent px-4 text-sm font-semibold text-white hover:bg-[#0072b1] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-45 xl:flex"
        >
          <Play size={14} />
          {analyzing ? "Запуск…" : "Анализировать"}
        </button>
      </div>
    </header>
  );
}
