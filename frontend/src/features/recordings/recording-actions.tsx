"use client";

import { Upload } from "lucide-react";
import { type DragEvent, useRef, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";

const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";
const subscribeHydration = () => () => {};
type UploadState =
  | "idle"
  | "creating"
  | "uploading"
  | "confirming"
  | "done"
  | "error";

function uploadToStorage(
  uploadUrl: string,
  file: File,
  onProgress: (progress: number) => void,
) {
  return new Promise<void>((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open("PUT", uploadUrl);
    request.setRequestHeader("Content-Type", file.type);
    request.timeout = 15 * 60 * 1000;
    request.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) {
        onProgress(Math.round((event.loaded / event.total) * 100));
      }
    };
    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        onProgress(100);
        resolve();
        return;
      }
      reject(new Error(`R2 вернул HTTP ${request.status}`));
    };
    request.onerror = () =>
      reject(new Error("Соединение с R2 прервано. Файл не был загружен."));
    request.onabort = () => reject(new Error("Загрузка отменена."));
    request.ontimeout = () =>
      reject(new Error("Загрузка в R2 превысила 15 минут."));
    request.send(file);
  });
}

async function confirmUpload(recordingId: string) {
  let lastResponse: Response | null = null;
  for (let attempt = 0; attempt < 3; attempt += 1) {
    lastResponse = await fetch(
      `${apiUrl}/v1/recordings/${recordingId}/uploads/complete`,
      { method: "POST" },
    );
    if (lastResponse.ok || lastResponse.status < 500) return lastResponse;
    await new Promise((resolve) =>
      window.setTimeout(resolve, 750 * (attempt + 1)),
    );
  }
  return lastResponse;
}

export function RecordingUpload() {
  const inputRef = useRef<HTMLInputElement>(null);
  const router = useRouter();
  const [state, setState] = useState<UploadState>("idle");
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState("");
  const hydrated = useSyncExternalStore(
    subscribeHydration,
    () => true,
    () => false,
  );

  async function upload(file?: File) {
    file ??= inputRef.current?.files?.[0];
    if (!file) return;
    setState("creating");
    setProgress(0);
    setError("");
    let recordingId = "";
    let objectUploaded = false;
    try {
      const sessionResponse = await fetch(`${apiUrl}/v1/recordings/uploads`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          originalName: file.name,
          mimeType: file.type,
          sizeBytes: file.size,
        }),
      });
      if (!sessionResponse.ok) throw new Error("Не удалось создать upload");
      const session = (await sessionResponse.json()) as {
        recording: { id: string };
        uploadUrl: string;
      };
      recordingId = session.recording.id;
      setState("uploading");
      await uploadToStorage(session.uploadUrl, file, setProgress);
      objectUploaded = true;
      setState("confirming");
      const completeResponse = await confirmUpload(recordingId);
      if (!completeResponse)
        throw new Error("API не ответил при подтверждении upload");
      if (!completeResponse.ok)
        throw new Error("Не удалось подтвердить upload");
      setState("done");
      if (inputRef.current) inputRef.current.value = "";
      router.refresh();
    } catch (current) {
      if (recordingId && !objectUploaded) {
        await fetch(`${apiUrl}/v1/recordings/${recordingId}`, {
          method: "DELETE",
        }).catch(() => undefined);
        router.refresh();
      }
      setError(current instanceof Error ? current.message : "Upload failed");
      setState("error");
    }
  }

  function drop(event: DragEvent<HTMLLabelElement>) {
    event.preventDefault();
    if (["creating", "uploading", "confirming"].includes(state)) return;
    const file = event.dataTransfer.files?.[0];
    if (file) void upload(file);
  }

  return (
    <div>
      <input
        ref={inputRef}
        id="recording-upload"
        type="file"
        accept="video/mp4,video/webm,.mp4,.webm"
        disabled={
          !hydrated || ["creating", "uploading", "confirming"].includes(state)
        }
        onChange={(event) => void upload(event.target.files?.[0])}
        className="sr-only"
      />
      <label
        htmlFor="recording-upload"
        onDragOver={(event) => event.preventDefault()}
        onDrop={drop}
        className="grid min-h-36 place-items-center rounded-sm border border-dashed border-[#9aa9bb] bg-panel px-6 py-5 text-center transition-colors hover:border-accent hover:bg-accent-soft/40"
      >
        <span>
          <span className="mx-auto grid size-11 place-items-center rounded-lg bg-[#e9edf5] text-[#53647d]">
            <Upload size={23} />
          </span>
          <strong className="mt-3 block text-sm font-semibold">
            {state === "creating"
              ? "Подготовка загрузки…"
              : state === "uploading"
                ? `Загрузка в R2: ${progress}%`
                : state === "confirming"
                  ? "Проверка загруженного файла…"
                  : "Перетащите файл сюда или нажмите для выбора"}
          </strong>
          <span className="mt-1 block text-xs text-muted">
            Поддерживаются .webm и .mp4
          </span>
        </span>
      </label>
      {error ? <p className="mt-3 text-xs text-danger">{error}</p> : null}
      {state === "done" ? (
        <p className="mt-3 text-xs text-success">Загрузка завершена.</p>
      ) : null}
    </div>
  );
}

export function RecordingActions({
  recordingId,
  recordingStatus,
  canAnalyze,
  compact = false,
}: {
  recordingId: string;
  recordingStatus: string;
  canAnalyze: boolean;
  compact?: boolean;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState("");
  const hydrated = useSyncExternalStore(
    subscribeHydration,
    () => true,
    () => false,
  );
  const analyzeLabel =
    busy === "analyze"
      ? "Запуск…"
      : canAnalyze
        ? "Анализировать"
        : recordingStatus === "PENDING_UPLOAD"
          ? "Ожидает файл"
          : "Анализируется";

  async function mutate(action: "analyze" | "delete") {
    setBusy(action);
    const response = await fetch(
      action === "analyze"
        ? `${apiUrl}/v1/recordings/${recordingId}/analysis-runs`
        : `${apiUrl}/v1/recordings/${recordingId}`,
      { method: action === "analyze" ? "POST" : "DELETE" },
    );
    setBusy("");
    if (response.ok) router.refresh();
  }

  return (
    <div className={compact ? "flex flex-wrap gap-2" : "contents"}>
      <button
        type="button"
        disabled={!hydrated || !canAnalyze || Boolean(busy)}
        onClick={() => mutate("analyze")}
        className="rounded-sm border border-[#b8c7dc] bg-panel px-3 py-1.5 text-xs text-[#41536d] hover:border-accent hover:text-accent active:translate-y-px disabled:opacity-40"
      >
        {analyzeLabel}
      </button>
      {!compact ? (
        <>
          <button
            type="button"
            disabled={!hydrated || !canAnalyze || Boolean(busy)}
            onClick={() => mutate("analyze")}
            className="rounded-sm border border-[#b8c7dc] bg-panel px-3 py-1.5 text-xs text-[#41536d] hover:border-accent hover:text-accent active:translate-y-px disabled:opacity-40"
          >
            Повтор
          </button>
          <button
            type="button"
            disabled={!hydrated || Boolean(busy)}
            onClick={() => mutate("delete")}
            className="order-4 rounded-sm border border-[#fecaca] bg-panel px-3 py-1.5 text-xs text-danger hover:bg-[#fff7f7] active:translate-y-px"
          >
            Удалить
          </button>
        </>
      ) : null}
    </div>
  );
}
