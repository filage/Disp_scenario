"use client";

import { Upload } from "lucide-react";
import {
  type DragEvent,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useRouter } from "next/navigation";

const apiUrl =
  process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";
const subscribeHydration = () => () => {};

export function RecordingUpload() {
  const inputRef = useRef<HTMLInputElement>(null);
  const router = useRouter();
  const [state, setState] = useState("idle");
  const [error, setError] = useState("");
  const hydrated = useSyncExternalStore(
    subscribeHydration,
    () => true,
    () => false,
  );

  async function upload(file?: File) {
    file ??= inputRef.current?.files?.[0];
    if (!file) return;
    setState("uploading");
    setError("");
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
      const uploadResponse = await fetch(session.uploadUrl, {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      });
      if (!uploadResponse.ok) throw new Error("S3 upload завершился ошибкой");
      const completeResponse = await fetch(
        `${apiUrl}/v1/recordings/${session.recording.id}/uploads/complete`,
        { method: "POST" },
      );
      if (!completeResponse.ok) throw new Error("Не удалось подтвердить upload");
      setState("done");
      if (inputRef.current) inputRef.current.value = "";
      router.refresh();
    } catch (current) {
      setError(current instanceof Error ? current.message : "Upload failed");
      setState("error");
    }
  }

  function drop(event: DragEvent<HTMLLabelElement>) {
    event.preventDefault();
    if (state === "uploading") return;
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
        disabled={!hydrated || state === "uploading"}
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
            {state === "uploading"
              ? "Загрузка записи…"
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
  canAnalyze,
  compact = false,
}: {
  recordingId: string;
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
        {busy === "analyze"
          ? "Запуск…"
          : canAnalyze
            ? "Анализировать"
            : "Анализируется"}
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
