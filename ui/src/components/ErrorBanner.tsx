"use client";

export function ErrorBanner({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className="card animate-in"
      style={{
        borderColor: "var(--error)",
        marginBottom: 24,
        background: "rgba(248,113,113,0.05)",
      }}
    >
      <div className="card-title" style={{ color: "var(--error)" }}>
        {title}
      </div>
      <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
        {children}
      </div>
    </div>
  );
}
