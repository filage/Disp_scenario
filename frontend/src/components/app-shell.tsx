import { Database, RadioTower } from "lucide-react";
import { cookies } from "next/headers";
import { Navigation } from "@/components/navigation";
import { TopBar } from "@/components/top-bar";
import { oidcEnabled } from "@/lib/oidc";

export async function AppShell({ children }: { children: React.ReactNode }) {
  const authEnabled = oidcEnabled();
  const signedIn = Boolean((await cookies()).get("id_token")?.value);
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
            <div className="text-xs text-muted">
              Analyst v2
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
          {authEnabled ? (
            <a
              href={signedIn ? "/api/auth/logout" : "/api/auth/login"}
              className="mt-4 block rounded-sm border border-line px-3 py-2 text-center text-[11px] text-muted hover:border-accent hover:text-accent"
            >
              {signedIn ? "Выйти" : "Войти"}
            </a>
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
