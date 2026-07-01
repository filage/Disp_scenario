import { Database, LogOut, RadioTower } from "lucide-react";
import { Navigation } from "@/components/navigation";
import { TopBar } from "@/components/top-bar";
import { currentSession } from "@/lib/session";

export async function AppShell({ children }: { children: React.ReactNode }) {
  const session = await currentSession();
  return (
    <div className="min-h-[100dvh] bg-background lg:grid lg:grid-cols-[240px_minmax(0,1fr)]">
      <aside className="border-line bg-panel border-b lg:sticky lg:top-0 lg:flex lg:h-[100dvh] lg:flex-col lg:border-r lg:border-b-0">
        <div className="flex h-[88px] items-center gap-3 px-4 lg:h-[112px]">
          <div className="bg-accent grid size-9 place-items-center rounded-md text-sm font-extrabold text-white">
            DS
          </div>
          <div>
            <div className="text-[17px] font-bold tracking-tight text-accent">
              DispScenario
            </div>
          </div>
        </div>
        <div className="overflow-x-auto px-2 pb-3 lg:overflow-visible">
          <Navigation />
        </div>
        <div className="mt-auto hidden border-t border-line p-4 lg:block">
          <div className="grid grid-cols-2 gap-2 text-[10px] text-muted">
            <span className="flex items-center gap-1.5">
              <Database size={12} /> POSTGRES
            </span>
            <span className="flex items-center gap-1.5">
              <RadioTower size={12} /> WORKER
            </span>
          </div>
          {session ? (
            <form action="/api/session/logout" method="post" className="mt-4">
              <button
                type="submit"
                className="flex w-full items-center justify-between border border-line px-3 py-2 text-left text-[11px] text-muted hover:border-accent hover:text-accent"
              >
                <span className="min-w-0 truncate">{session.subject}</span>
                <LogOut size={13} aria-label="Выйти" />
              </button>
            </form>
          ) : null}
        </div>
      </aside>
      <section className="min-w-0">
        <TopBar />
        <main className="min-w-0">{children}</main>
      </section>
    </div>
  );
}
