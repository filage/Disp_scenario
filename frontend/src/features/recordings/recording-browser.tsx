"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { StatusBadge } from "@/components/data-state";
import {
  RecordingActions,
  RecordingUpload,
} from "@/features/recordings/recording-actions";
import type { Recording } from "@/lib/data";

const LIVE_RECORDING_STATUSES = new Set(["PENDING_UPLOAD", "PROCESSING"]);
const RECORDING_REFRESH_INTERVAL_MS = 3_000;

export function RecordingBrowser({
  recordings,
  playbackBase,
  exportBase,
}: {
  recordings: Recording[];
  playbackBase: string;
  exportBase: string;
}) {
  const router = useRouter();
  const [selectedId, setSelectedId] = useState(recordings[0]?.id ?? "");
  const selected =
    recordings.find((recording) => recording.id === selectedId) ??
    recordings[0] ??
    null;
  const hasLiveRecordings = recordings.some((recording) =>
    LIVE_RECORDING_STATUSES.has(recording.status),
  );

  useEffect(() => {
    if (!hasLiveRecordings) return;

    const interval = window.setInterval(() => {
      router.refresh();
    }, RECORDING_REFRESH_INTERVAL_MS);

    return () => window.clearInterval(interval);
  }, [hasLiveRecordings, router]);

  return (
    <>
      <RecordingUpload />
      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <section className="min-w-0 rounded-sm border border-line bg-panel">
          <header className="flex h-12 items-center justify-between border-b border-line bg-panel-raised px-4">
            <h2 className="text-sm font-semibold">Список записей</h2>
            <span className="text-xs text-muted">{recordings.length} записей</span>
          </header>
          <div className="overflow-x-auto">
          <table className="w-full min-w-[60rem] text-left text-sm">
            <thead className="border-b border-line bg-[#f1f4fa] text-[10px] uppercase tracking-wide text-muted">
              <tr>
                <th className="px-4 py-3">Имя файла</th>
                <th className="px-4 py-3">Длительность</th>
                <th className="px-4 py-3">Размер</th>
                <th className="px-4 py-3">Загружено</th>
                <th className="px-4 py-3">Статус</th>
                <th className="px-4 py-3">Хранилище</th>
                <th className="px-4 py-3">Действие</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {recordings.map((recording) => (
                <tr
                  key={recording.id}
                  onClick={() => setSelectedId(recording.id)}
                  className={`cursor-pointer transition-colors ${
                    selected?.id === recording.id
                      ? "bg-[#cce5ff]"
                      : "hover:bg-[#f8fafc]"
                  }`}
                >
                  <td className="px-4 py-4 font-medium">
                    {recording.originalName}
                  </td>
                  <td className="px-4 py-4 font-mono text-xs text-muted">
                    {recording.durationSec
                      ? `${recording.durationSec.toFixed(1)} s`
                      : "—"}
                  </td>
                  <td className="px-4 py-4 font-mono text-xs text-muted">
                    {(recording.sizeBytes / 1024 / 1024).toFixed(1)} MB
                  </td>
                  <td className="px-4 py-4 text-xs text-muted">
                    {formatRecordingDate(recording.createdAt)}
                  </td>
                  <td className="px-4 py-4">
                    <StatusBadge value={recording.status} />
                  </td>
                  <td className="px-4 py-4 text-xs text-muted">S3 / MinIO</td>
                  <td className="px-4 py-4">
                      <RecordingActions
                        recordingId={recording.id}
                      canAnalyze={[
                        "UPLOADED",
                        "ANALYZED",
                        "FAILED",
                      ].includes(recording.status)}
                      compact
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {!recordings.length ? (
            <p className="p-8 text-sm text-muted">Записей пока нет.</p>
          ) : null}
          </div>
        </section>

        <aside className="rounded-sm border border-line bg-panel">
          <div className="flex items-start justify-between gap-3 border-b border-line bg-panel-raised p-4">
            <div>
              <h2 className="break-all text-sm font-semibold">
                {selected?.originalName ?? "Запись не выбрана"}
              </h2>
              <p className="mt-1 font-mono text-[10px] text-muted">
                ID: {selected?.id.slice(0, 8) ?? "—"}
              </p>
            </div>
            {selected ? <StatusBadge value={selected.status} /> : null}
          </div>
          <div className="p-4">
            <PlaybackPreview
              key={selected?.id ?? "empty"}
              recording={selected}
              playbackBase={playbackBase}
            />
            {selected ? (
              <>
              <dl className="mt-4 grid grid-cols-[1fr_auto] gap-3 text-xs">
                <dt className="text-muted">ID</dt>
                <dd className="max-w-40 truncate font-mono" title={selected.id}>
                  {selected.id}
                </dd>
                <dt className="text-muted">Формат</dt>
                <dd className="font-mono">{selected.mimeType}</dd>
                <dt className="text-muted">Длительность</dt>
                <dd className="font-mono">
                  {selected.durationSec
                    ? `${selected.durationSec.toFixed(1)} s`
                    : "—"}
                </dd>
                <dt className="text-muted">Размер</dt>
                <dd className="font-mono">
                  {(selected.sizeBytes / 1024 / 1024).toFixed(1)} MB
                </dd>
              </dl>
              <div className="mt-5 grid grid-cols-2 gap-2 border-t border-line pt-4">
                <RecordingActions
                  recordingId={selected.id}
                  canAnalyze={["UPLOADED", "ANALYZED", "FAILED"].includes(
                    selected.status,
                  )}
                />
                <a
                  href={`${exportBase}/${selected.id}/exports/report.json`}
                  className="order-3 rounded-sm border border-[#b8c7dc] bg-panel px-3 py-1.5 text-center text-xs text-[#41536d] hover:border-accent hover:text-accent"
                >
                  Экспорт
                </a>
              </div>
              </>
            ) : null}
          </div>
        </aside>
      </div>
    </>
  );
}

function PlaybackPreview({
  recording,
  playbackBase,
}: {
  recording: Recording | null;
  playbackBase: string;
}) {
  const [playbackUrl, setPlaybackUrl] = useState("");
  const [playbackError, setPlaybackError] = useState("");
  const available = Boolean(
    recording && !["PENDING_UPLOAD", "DELETED"].includes(recording.status),
  );

  useEffect(() => {
    let active = true;
    if (!available || !recording) {
      return () => {
        active = false;
      };
    }
    void fetch(`${playbackBase}/${recording.id}/playback`, {
      headers: { Accept: "application/json" },
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) throw new Error(`Видео недоступно: HTTP ${response.status}`);
        return (await response.json()) as { url: string };
      })
      .then((payload) => {
        if (active) setPlaybackUrl(payload.url);
      })
      .catch((error: unknown) => {
        if (active) {
          setPlaybackError(
            error instanceof Error ? error.message : "Воспроизведение недоступно",
          );
        }
      });
    return () => {
      active = false;
    };
  }, [available, playbackBase, recording]);

  if (playbackUrl) {
    return (
      <video
        src={playbackUrl}
        controls
        preload="metadata"
        className="aspect-video w-full rounded-sm border border-line bg-[#111827] object-contain"
      />
    );
  }
  return (
    <div className="grid aspect-video place-items-center rounded-sm border border-line bg-[#f8fafc] text-xs text-muted">
      {playbackError ||
        (available ? "Загрузка превью…" : "Превью недоступно")}
    </div>
  );
}

const recordingDateFormatter = new Intl.DateTimeFormat("ru-RU", {
  timeZone: "Europe/Minsk",
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function formatRecordingDate(value: string) {
  return recordingDateFormatter.format(new Date(value));
}
