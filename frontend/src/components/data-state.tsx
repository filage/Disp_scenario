export function DataState({
  children,
  empty,
}: {
  children: React.ReactNode;
  empty?: boolean;
}) {
  if (empty) {
    return (
      <div className="border border-dashed border-line p-8 text-sm text-muted">
        Данных пока нет. Запустите анализ записи, чтобы заполнить этот раздел.
      </div>
    );
  }
  return <>{children}</>;
}

export function StatusBadge({ value }: { value: string }) {
  const normalized = value.toLowerCase();
  const labels: Record<string, string> = {
    analyzed: "проанализировано",
    completed: "завершено",
    processing: "обрабатывается",
    normalizing: "нормализация",
    queued: "в очереди",
    uploaded: "загружено",
    pending_upload: "ожидает загрузки",
    failed: "ошибка",
    cancelled: "отменено",
    resolved: "решено",
  };
  const tone =
    normalized.includes("fail") || normalized.includes("error")
      ? "border-[#fecaca] bg-[#fee2e2] text-danger"
      : normalized.includes("complete") ||
          normalized.includes("analyzed") ||
          normalized.includes("resolved")
        ? "border-[#bbf7d0] bg-[#dcfce7] text-success"
        : normalized.includes("process") || normalized.includes("normaliz")
          ? "border-[#bfdbfe] bg-[#e4f2ff] text-accent"
          : "border-[#fed7aa] bg-[#ffedd5] text-warning";
  return (
    <span
      className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase ${tone}`}
    >
      {labels[normalized] ?? value}
    </span>
  );
}
