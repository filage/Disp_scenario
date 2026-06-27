import { PageFrame } from "@/components/page-frame";

export function PlaceholderPage({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description: string;
}) {
  return (
    <PageFrame eyebrow={eyebrow} title={title} description={description}>
      <div className="bg-panel border border-dashed border-line p-8">
        <p className="font-mono text-xs uppercase tracking-[0.16em] text-muted">
          Модуль подключается к v2 API
        </p>
      </div>
    </PageFrame>
  );
}

