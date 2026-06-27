"use client";

import {
  BarChart3,
  Bot,
  FileText,
  Film,
  Gauge,
  GitBranch,
  ListTree,
  Route,
  Settings,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";

const items = [
  { href: "/overview", label: "Обзор", icon: Gauge },
  { href: "/recordings", label: "Записи", icon: Film },
  { href: "/runs", label: "Запуски анализа", icon: BarChart3 },
  { href: "/timeline", label: "Таймлайн", icon: Route },
  { href: "/scenario-map", label: "Карта сценариев", icon: GitBranch },
  { href: "/groups", label: "Группы сценариев", icon: ListTree },
  { href: "/qa", label: "QA-проверка", icon: ShieldCheck },
  { href: "/automation", label: "Автоматизация", icon: Bot },
  { href: "/reports", label: "Отчеты", icon: FileText },
  { href: "/settings", label: "Настройки", icon: Settings },
] as const;

export function Navigation() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const recordingId = searchParams.get("recordingId");

  return (
    <nav aria-label="Основная навигация" className="space-y-1">
      {items.map(({ href, label, icon: Icon }) => {
        const active = pathname.startsWith(href);
        return (
          <Link
            key={href}
            href={recordingId ? `${href}?recordingId=${recordingId}` : href}
            className={[
              "group flex h-10 items-center gap-3 rounded-[3px] border-l-[4px] px-3 text-sm transition-colors",
              active
                ? "border-accent bg-[#e9edf5] font-semibold text-accent"
                : "border-transparent text-[#31435f] hover:border-accent/40 hover:bg-panel-raised hover:text-accent",
            ].join(" ")}
          >
            <Icon
              aria-hidden
              className={active ? "text-accent" : "group-hover:text-accent"}
              size={19}
            />
            {label}
          </Link>
        );
      })}
    </nav>
  );
}
