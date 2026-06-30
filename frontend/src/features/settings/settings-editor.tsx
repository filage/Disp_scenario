"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

export type KnownScenario = {
  code: string;
  name: string;
  issueType: string;
  entityType?: string;
  startActions: string[];
  requiredActions: string[];
  optionalActions: string[];
  endActions: string[];
  forbiddenActions: string[];
  timeoutMs: number;
  version: string;
  enabled: boolean;
};

export type BoundaryRule = {
  id: string;
  name: string;
  priority: number;
  type: string;
  conditions: Record<string, unknown>;
  version: string;
  enabled: boolean;
};

export type SettingsData = {
  versions: Record<string, string>;
  knownScenarios: KnownScenario[];
  boundaryRules: BoundaryRule[];
  actionCatalog: string[];
  dataQualityFlags: string[];
};

export type GeminiCredential = {
  provider: string;
  configured: boolean;
  lastFour?: string;
  updatedAt?: string;
};

const tabs = [
  ["gemini", "Gemini API"],
  ["known", "Известные сценарии"],
  ["prompts", "Промпты"],
  ["boundary", "Правила границ"],
  ["actions", "Каталог действий"],
  ["quality", "Качество данных"],
] as const;

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";

export function SettingsEditor({
  settings,
  geminiCredential,
}: {
  settings: SettingsData;
  geminiCredential: GeminiCredential;
}) {
  const [active, setActive] = useState<(typeof tabs)[number][0]>("gemini");

  return (
    <div className="mt-6 grid gap-4 xl:grid-cols-[15rem_minmax(0,1fr)]">
      <nav className="border border-line bg-panel p-2" aria-label="Settings">
        {tabs.map(([id, label]) => (
          <button
            key={id}
            type="button"
            onClick={() => setActive(id)}
            className={`block w-full border-b border-line px-3 py-3 text-left text-xs ${
              active === id
                ? "bg-accent/10 text-accent"
                : "text-muted hover:text-foreground"
            }`}
          >
            {label}
          </button>
        ))}
      </nav>

      <section className="border border-line bg-panel p-5">
        {active === "gemini" ? (
          <GeminiCredentialForm initial={geminiCredential} />
        ) : null}
        {active === "known" ? (
          <div className="grid gap-4">
            <div className="border-l-2 border-accent bg-accent-soft px-4 py-3 text-xs leading-5 text-muted">
              Здесь администратор может изменять и отключать уже
              зарегистрированные сценарии. Создание нового сценария через
              интерфейс пока не реализовано: для него нужны новая запись в
              каталоге и правила распознавания на backend.
            </div>
            {settings.knownScenarios.map((scenario) => (
              <KnownScenarioForm key={scenario.code} scenario={scenario} />
            ))}
          </div>
        ) : null}
        {active === "prompts" ? (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {Object.entries(settings.versions).map(([name, version]) => (
              <article key={name} className="border border-line p-4">
                <p className="text-xs text-muted">{name}</p>
                <p className="mt-2 font-mono text-sm text-accent">{version}</p>
                <p className="mt-3 text-[11px] leading-5 text-muted">
                  Версия фиксируется в analysis run и report snapshot.
                </p>
              </article>
            ))}
          </div>
        ) : null}
        {active === "boundary" ? (
          <div className="grid gap-4">
            {settings.boundaryRules.map((rule) => (
              <BoundaryRuleForm key={rule.id} rule={rule} />
            ))}
          </div>
        ) : null}
        {active === "actions" ? (
          <Catalog
            title="Канонический каталог действий"
            values={settings.actionCatalog}
          />
        ) : null}
        {active === "quality" ? (
          <Catalog
            title="Флаги качества данных"
            values={settings.dataQualityFlags}
          />
        ) : null}
      </section>
    </div>
  );
}

function GeminiCredentialForm({ initial }: { initial: GeminiCredential }) {
  const [status, setStatus] = useState(initial);
  const [apiKey, setAPIKey] = useState("");
  const [state, setState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [error, setError] = useState("");

  async function save() {
    setState("saving");
    setError("");
    try {
      const response = await fetch(`${apiURL}/v1/settings/gemini-credential`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ apiKey: apiKey.trim() }),
      });
      if (!response.ok) {
        setError(await credentialErrorMessage(response));
        setState("error");
        return;
      }
      setStatus((await response.json()) as GeminiCredential);
      setAPIKey("");
      setState("saved");
    } catch {
      setError("Соединение с API прервалось. Подождите несколько секунд и повторите.");
      setState("error");
    }
  }

  async function remove() {
    setState("saving");
    setError("");
    try {
      const response = await fetch(`${apiURL}/v1/settings/gemini-credential`, {
        method: "DELETE",
      });
      if (!response.ok) {
        setError(await credentialErrorMessage(response));
        setState("error");
        return;
      }
      setStatus({ provider: "gemini", configured: false });
      setState("idle");
    } catch {
      setError("Соединение с API прервалось. Подождите несколько секунд и повторите.");
      setState("error");
    }
  }

  return (
    <div className="max-w-3xl">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-line pb-5">
        <div>
          <p className="font-mono text-[10px] uppercase tracking-[0.16em] text-accent">Личный провайдер</p>
          <h2 className="mt-2 text-lg font-semibold">Gemini API key</h2>
          <p className="mt-2 max-w-xl text-xs leading-5 text-muted">
            Ключ используется только для ваших новых запусков анализа. Он шифруется на сервере и после сохранения больше не показывается.
          </p>
        </div>
        <span className={`border px-3 py-2 text-[10px] uppercase ${status.configured ? "border-success/30 bg-success/5 text-success" : "border-warning/30 bg-warning/5 text-warning"}`}>
          {status.configured ? `Подключён · ••••${status.lastFour}` : "Не настроен"}
        </span>
      </div>
      <div className="mt-6 grid gap-4">
        <label className="grid gap-2 text-xs text-muted">
          Новый API key
          <input
            type="password"
            value={apiKey}
            onChange={(event) => {
              setAPIKey(event.target.value);
              setState("idle");
              setError("");
            }}
            autoComplete="off"
            spellCheck={false}
            placeholder="AIza…"
            className="h-11 border border-line bg-background px-3 font-mono text-sm text-foreground"
          />
        </label>
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            disabled={state === "saving" || apiKey.trim().length < 20}
            onClick={save}
            className="bg-accent px-4 py-2.5 text-xs font-semibold text-white disabled:opacity-40"
          >
            {state === "saving" ? "Сохраняем…" : status.configured ? "Заменить ключ" : "Сохранить ключ"}
          </button>
          {status.configured ? (
            <button type="button" disabled={state === "saving"} onClick={remove} className="border border-line px-4 py-2.5 text-xs text-muted hover:border-danger hover:text-danger">
              Удалить ключ
            </button>
          ) : null}
          {state === "saved" ? <span className="text-xs text-success">Ключ сохранён</span> : null}
          {state === "error" ? <span className="text-xs text-danger">{error}</span> : null}
        </div>
      </div>
      <div className="mt-7 grid gap-3 border-t border-line pt-5 text-xs leading-5 text-muted sm:grid-cols-3">
        <p><strong className="block text-foreground">Где взять</strong>Google AI Studio → API keys.</p>
        <p><strong className="block text-foreground">Где хранится</strong>PostgreSQL, AES-256-GCM.</p>
        <p><strong className="block text-foreground">Кто использует</strong>Только запуски вашей сессии.</p>
      </div>
    </div>
  );
}

async function credentialErrorMessage(response: Response) {
  let message = "";
  try {
    const body = (await response.json()) as {
      error?: string | { message?: string };
    };
    message =
      typeof body.error === "string"
        ? body.error
        : body.error?.message ?? "";
  } catch {
    // The status-specific fallback below remains actionable without a JSON body.
  }
  if (response.status === 503) {
    return message || "Основной API запускается. Повторите через несколько секунд.";
  }
  if (response.status === 401) {
    return "Сессия завершилась. Войдите в систему заново.";
  }
  if (response.status === 400) {
    return "Ключ не принят. Вставьте только значение API key из Google AI Studio.";
  }
  return message || `Не удалось сохранить ключ (HTTP ${response.status}).`;
}

function KnownScenarioForm({ scenario }: { scenario: KnownScenario }) {
  const router = useRouter();
  const [draft, setDraft] = useState(scenario);
  const [state, setState] = useState("idle");

  async function save() {
    setState("saving");
    const response = await fetch(
      `${apiURL}/v1/settings/known-scenarios/${scenario.code}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(draft),
      },
    );
    setState(response.ok ? "saved" : "error");
    if (response.ok) router.refresh();
  }

  return (
    <article className="border border-line bg-panel-raised p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="font-mono text-[10px] text-accent">
            {scenario.code} · {scenario.version}
          </p>
          <h2 className="mt-2 text-base font-semibold">{draft.name}</h2>
        </div>
        <label className="flex items-center gap-2 text-xs text-muted">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                enabled: change.target.checked,
              }))
            }
          />
          включён
        </label>
      </div>
      <div className="mt-5 grid gap-4 md:grid-cols-2">
        <Field
          label="Название"
          value={draft.name}
          onChange={(name) => setDraft((current) => ({ ...current, name }))}
        />
        <Field
          label="Тип проблемы"
          value={draft.issueType}
          onChange={(issueType) =>
            setDraft((current) => ({ ...current, issueType }))
          }
        />
        <Field
          label="Тип сущности"
          value={draft.entityType ?? ""}
          onChange={(entityType) =>
            setDraft((current) => ({ ...current, entityType }))
          }
        />
        <Field
          label="Таймаут, минут"
          type="number"
          value={String(Math.round(draft.timeoutMs / 60000))}
          onChange={(minutes) =>
            setDraft((current) => ({
              ...current,
              timeoutMs: Number(minutes) * 60000,
            }))
          }
        />
        {(
          [
            ["startActions", "Стартовые действия"],
            ["requiredActions", "Обязательные действия"],
            ["optionalActions", "Опциональные действия"],
            ["endActions", "Завершающие действия"],
            ["forbiddenActions", "Запрещённые действия"],
          ] as const
        ).map(([key, label]) => (
          <Field
            key={key}
            label={label}
            value={draft[key].join(", ")}
            onChange={(value) =>
              setDraft((current) => ({
                ...current,
                [key]: splitList(value),
              }))
            }
          />
        ))}
      </div>
      <FormActions
        state={state}
        onReset={() => setDraft(scenario)}
        onSave={save}
      />
    </article>
  );
}

function BoundaryRuleForm({ rule }: { rule: BoundaryRule }) {
  const router = useRouter();
  const [draft, setDraft] = useState(rule);
  const [conditions, setConditions] = useState(
    JSON.stringify(rule.conditions, null, 2),
  );
  const [state, setState] = useState("idle");

  async function save() {
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(conditions) as Record<string, unknown>;
    } catch {
      setState("invalid-json");
      return;
    }
    setState("saving");
    const response = await fetch(
      `${apiURL}/v1/settings/boundary-rules/${rule.id}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...draft, conditions: parsed }),
      },
    );
    setState(response.ok ? "saved" : "error");
    if (response.ok) router.refresh();
  }

  return (
    <article className="border border-line bg-panel-raised p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="font-mono text-[10px] text-accent">
            {rule.id} · {rule.version}
          </p>
          <h2 className="mt-2 text-base font-semibold">{draft.name}</h2>
        </div>
        <label className="flex items-center gap-2 text-xs text-muted">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(change) =>
              setDraft((current) => ({
                ...current,
                enabled: change.target.checked,
              }))
            }
          />
          включено
        </label>
      </div>
      <div className="mt-5 grid gap-4 md:grid-cols-3">
        <Field
          label="Название"
          value={draft.name}
          onChange={(name) => setDraft((current) => ({ ...current, name }))}
        />
        <Field
          label="Тип"
          value={draft.type}
          onChange={(type) => setDraft((current) => ({ ...current, type }))}
        />
        <Field
          label="Приоритет"
          type="number"
          value={String(draft.priority)}
          onChange={(priority) =>
            setDraft((current) => ({
              ...current,
              priority: Number(priority),
            }))
          }
        />
      </div>
      <label className="mt-4 grid gap-2 text-xs text-muted">
        Условия JSON
        <textarea
          value={conditions}
          onChange={(change) => setConditions(change.target.value)}
          rows={8}
          spellCheck={false}
          className="resize-y border border-line bg-background px-3 py-2 font-mono text-xs text-foreground"
        />
      </label>
      <FormActions
        state={state}
        onReset={() => {
          setDraft(rule);
          setConditions(JSON.stringify(rule.conditions, null, 2));
        }}
        onSave={save}
      />
    </article>
  );
}

function Field({
  label,
  value,
  onChange,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: "text" | "number";
}) {
  return (
    <label className="grid gap-2 text-xs text-muted">
      {label}
      <input
        type={type}
        value={value}
        onChange={(change) => onChange(change.target.value)}
        className="border border-line bg-background px-3 py-2 text-sm text-foreground"
      />
    </label>
  );
}

function FormActions({
  state,
  onReset,
  onSave,
}: {
  state: string;
  onReset: () => void;
  onSave: () => void;
}) {
  return (
    <div className="mt-5 flex flex-wrap items-center gap-2">
      <button
        type="button"
        onClick={onReset}
        className="border border-line px-3 py-2 text-[10px] uppercase text-muted"
      >
        Отмена
      </button>
      <button
        type="button"
        disabled={state === "saving"}
        onClick={onSave}
        className="border border-accent px-3 py-2 text-[10px] uppercase text-accent disabled:opacity-50"
      >
        Сохранить изменения
      </button>
      {state === "saved" ? (
        <span className="text-xs text-accent">Сохранено</span>
      ) : null}
      {state === "error" || state === "invalid-json" ? (
        <span className="text-xs text-danger">
          {state === "invalid-json"
            ? "Некорректный JSON"
            : "Не удалось сохранить"}
        </span>
      ) : null}
    </div>
  );
}

function Catalog({ title, values }: { title: string; values: string[] }) {
  return (
    <div>
      <h2 className="font-mono text-[10px] uppercase text-accent">{title}</h2>
      <div className="mt-4 flex flex-wrap gap-2">
        {values.map((value) => (
          <span
            key={value}
            className="border border-line bg-background px-3 py-2 font-mono text-[10px]"
          >
            {value}
          </span>
        ))}
      </div>
    </div>
  );
}

function splitList(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}
