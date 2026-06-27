export function PageFrame({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0 overflow-x-hidden px-4 py-5 md:px-6 md:py-6">
      <header className="mb-4">
        <span className="sr-only">{eyebrow}</span>
        <h1 className="text-xl font-semibold tracking-[-0.01em]">
          {title}
        </h1>
        <p className="mt-1 max-w-4xl text-sm leading-5 text-muted">
          {description}
        </p>
      </header>
      {children}
    </div>
  );
}
