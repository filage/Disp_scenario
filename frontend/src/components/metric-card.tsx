export function MetricCard({
  label,
  value,
  detail,
  tone = "default",
}: {
  label: string;
  value: string | number;
  detail: string;
  tone?: "default" | "accent" | "danger";
}) {
  const toneClass = {
    default: "text-foreground",
    accent: "text-accent",
    danger: "text-danger",
  }[tone];

  return (
    <article className="bg-panel border border-line p-5">
      <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted">
        {label}
      </p>
      <p className={`mt-5 font-mono text-3xl font-semibold ${toneClass}`}>
        {value}
      </p>
      <p className="mt-2 text-xs leading-5 text-muted">{detail}</p>
    </article>
  );
}

