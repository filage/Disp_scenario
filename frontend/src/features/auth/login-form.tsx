"use client";

import { ArrowRight, Eye, EyeOff, LoaderCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

export function LoginForm() {
  const router = useRouter();
  const [showPassword, setShowPassword] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const data = new FormData(event.currentTarget);
    const response = await fetch("/api/session/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: data.get("username"),
        password: data.get("password"),
      }),
    });
    if (!response.ok) {
      const result = (await response.json().catch(() => ({}))) as {
        error?: string;
      };
      setError(result.error ?? "Не удалось войти.");
      setPending(false);
      return;
    }
    router.replace("/overview");
    router.refresh();
  }

  return (
    <form onSubmit={submit} className="mt-9 grid gap-5">
      <label className="grid gap-2 text-xs font-medium text-muted">
        Логин
        <input
          name="username"
          autoComplete="username"
          required
          autoFocus
          className="h-12 border border-line bg-background px-4 text-sm text-foreground transition-colors focus:border-accent"
          placeholder="Введите логин"
        />
      </label>
      <label className="grid gap-2 text-xs font-medium text-muted">
        Пароль
        <span className="relative">
          <input
            name="password"
            type={showPassword ? "text" : "password"}
            autoComplete="current-password"
            required
            className="h-12 w-full border border-line bg-background px-4 pr-12 text-sm text-foreground transition-colors focus:border-accent"
            placeholder="Введите пароль"
          />
          <button
            type="button"
            onClick={() => setShowPassword((value) => !value)}
            aria-label={showPassword ? "Скрыть пароль" : "Показать пароль"}
            className="absolute inset-y-0 right-0 grid w-12 place-items-center text-muted hover:text-accent"
          >
            {showPassword ? <EyeOff size={17} /> : <Eye size={17} />}
          </button>
        </span>
      </label>
      {error ? (
        <p role="alert" className="border-l-2 border-danger bg-danger/5 px-3 py-2 text-xs text-danger">
          {error}
        </p>
      ) : null}
      <button
        type="submit"
        disabled={pending}
        className="flex h-12 items-center justify-between bg-accent px-4 text-sm font-semibold text-white transition-colors hover:bg-[#004f79] disabled:opacity-60"
      >
        <span>{pending ? "Проверяем доступ" : "Войти в систему"}</span>
        {pending ? <LoaderCircle className="animate-spin" size={18} /> : <ArrowRight size={18} />}
      </button>
    </form>
  );
}
