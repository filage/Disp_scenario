export function formatDuration(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.round(milliseconds / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes} мин ${seconds} с` : `${seconds} с`;
}

export function formatClock(milliseconds = 0): string {
  const total = Math.max(0, Math.round(Number(milliseconds || 0) / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function formatPercent(value: number): string {
  return `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`;
}

export function formatScenarioStatus(value?: string | null): string {
  const statuses: Record<string, string> = {
    candidate: "Кандидат",
    new: "Новый",
    accepted: "Принят",
    rejected: "Отклонён",
    active: "Активен",
    archived: "В архиве",
  };
  if (!value) return statuses.candidate;
  return statuses[value] ?? value;
}

export function formatQAStatus(value?: string | null): string {
  const statuses: Record<string, string> = {
    confirmed: "подтверждено",
    edited: "исправлено",
    ignored: "шум",
    reviewed: "проверено",
    unreviewed: "не проверено",
  };
  if (!value) return statuses.unreviewed;
  return statuses[value] ?? value;
}

export function formatIssueType(value?: string | null): string {
  const labels: Record<string, string> = {
    "Late pickup": "Опоздание на забор",
    "Unassigned courier": "Курьер не назначен",
    "Delivery destination change": "Смена точки окончания доставки",
    "Recipient contact update": "Обновление контакта получателя",
    "Delivery note update": "Добавление комментария к доставке",
    Unknown: "Неизвестно",
  };
  if (!value) return "—";
  return labels[value] ?? value;
}

export function formatQualityIssueType(value?: string | null): string {
  const labels: Record<string, string> = {
    LOW_CONFIDENCE: "Низкая уверенность",
    UNKNOWN_TARGET: "Неизвестная цель",
    OUT_OF_ORDER_TIMESTAMP: "Нарушен порядок времени",
    DUPLICATE_ACTION: "Повтор действия",
    AMBIGUOUS_BOUNDARY: "Спорная граница сценария",
    ACTION_FAILED: "Неудачное действие",
    MISSING_SCENARIO_END: "Не найдено завершение сценария",
    GEMINI_PARSE_FALLBACK: "Fallback-парсер Gemini",
    GEMINI_BOUNDARY_REVIEW: "Проверка границ Gemini",
    IGNORED_NOISE: "Шум",
  };
  if (!value) return "—";
  return labels[value] ?? value;
}

export function formatQualityIssueMessage(value?: string | null): string {
  const text = String(value ?? "");
  const exact: Record<string, string> = {
    "Gemini proposes different scenario boundary than deterministic rules.":
      "Gemini предлагает другую границу сценария, чем детерминированные правила.",
    "Gemini proposes a scenario boundary not matched by deterministic rules.":
      "Gemini предлагает границу сценария, которую не нашли детерминированные правила.",
    "QA marked event boundary as ambiguous":
      "QA отметил границу события как спорную.",
    "QA marked event as noise": "QA отметил событие как шум.",
    "QA kept low-confidence event under review":
      "QA оставил событие с низкой уверенностью на проверке.",
    "QA kept event with unknown target under review":
      "QA оставил событие с неизвестной целью на проверке.",
    "No quality issues detected": "Проблем качества не найдено",
  };
  if (exact[text]) return exact[text];
  return text
    .replace(/\bUnassigned courier\b/g, "Курьер не назначен")
    .replace(/\bLate pickup\b/g, "Опоздание на забор")
    .replace(/\binterrupted\b/g, "прервано")
    .replace(/\bunresolved\b/g, "не решено")
    .replace(/\bresolved\b/g, "решено")
    .replace(/\bvalidation error\b/g, "ошибка валидации")
    .replace(/\bQA should review\b/g, "QA нужно проверить")
    .replace(/Gemini boundary review failed:/gi, "Проверка границ Gemini не удалась:")
    .replace(/QA flag:/gi, "Флаг QA:");
}

export function formatTechnicalMetricName(value: string): string {
  const labels: Record<string, string> = {
    level: "Уровень",
    frequency: "Частота",
    occurrences: "Повторения",
    durationImpactMs: "Эффект по времени",
    durationImpact: "Эффект по времени",
    medianDurationMs: "Медианная длительность",
    averageDurationMs: "Средняя длительность",
    p95DurationMs: "P95 длительность",
    manualCheckCount: "Ручные проверки",
    repeatedActionCount: "Повторные действия",
    confidenceAverage: "Средняя уверенность",
    dataQualityConfidence: "Качество данных",
    automationReadiness: "Готовность к автоматизации",
    factors: "Факторы",
    weights: "Веса",
  };
  return labels[value] ?? value;
}
