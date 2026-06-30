import { Activity, LockKeyhole, ScanLine } from "lucide-react";
import { redirect } from "next/navigation";
import { LoginForm } from "@/features/auth/login-form";
import { currentSession } from "@/lib/session";

export default async function LoginPage() {
  if (await currentSession()) redirect("/overview");
  return (
    <main className="grid min-h-[100dvh] bg-background lg:grid-cols-[minmax(0,1.2fr)_minmax(420px,0.8fr)]">
      <section className="relative hidden overflow-hidden bg-[#092438] p-12 text-white lg:flex lg:flex-col lg:justify-between">
        <div className="absolute inset-0 opacity-20 [background-image:linear-gradient(rgba(255,255,255,.16)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,.16)_1px,transparent_1px)] [background-size:48px_48px]" />
        <div className="relative flex items-center gap-3">
          <span className="grid size-10 place-items-center bg-white font-extrabold text-accent">DS</span>
          <div>
            <p className="font-semibold tracking-tight">DispScenario</p>
            <p className="text-xs text-white/55">Video operations analyst</p>
          </div>
        </div>
        <div className="relative max-w-xl">
          <div className="mb-8 flex items-center gap-4 text-[10px] uppercase tracking-[0.22em] text-[#7fd1ff]">
            <span className="h-px w-12 bg-[#7fd1ff]" /> Контур анализа
          </div>
          <h1 className="max-w-lg text-5xl font-semibold leading-[1.05] tracking-[-0.045em]">
            Превращаем видеозаписи в проверяемые сценарии.
          </h1>
          <div className="mt-10 grid max-w-lg grid-cols-3 border-y border-white/15 py-5 text-xs text-white/65">
            <span className="flex items-center gap-2"><ScanLine size={15} /> События</span>
            <span className="flex items-center gap-2"><Activity size={15} /> Метрики</span>
            <span className="flex items-center gap-2"><LockKeyhole size={15} /> QA-контроль</span>
          </div>
        </div>
        <p className="relative font-mono text-[10px] uppercase tracking-[0.18em] text-white/40">
          Analyst console / v2
        </p>
      </section>
      <section className="flex items-center justify-center p-6 sm:p-10">
        <div className="w-full max-w-md">
          <div className="mb-12 flex items-center gap-3 lg:hidden">
            <span className="grid size-9 place-items-center bg-accent text-sm font-extrabold text-white">DS</span>
            <strong>DispScenario</strong>
          </div>
          <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-accent">Защищённый доступ</p>
          <h2 className="mt-4 text-3xl font-semibold tracking-[-0.035em]">Вход в рабочую область</h2>
          <p className="mt-3 max-w-sm text-sm leading-6 text-muted">
            Войдите, чтобы работать с записями, запускать анализ и управлять ключом Gemini.
          </p>
          <LoginForm />
          <p className="mt-8 text-[11px] leading-5 text-muted">
            Сессия хранится только в защищённой HttpOnly cookie. Пароль не передаётся в основной Go API.
          </p>
        </div>
      </section>
    </main>
  );
}
