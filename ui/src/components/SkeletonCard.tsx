/**
 * Shared skeleton loading card — used by Dashboard and Costs pages
 * during initial data fetch.
 */
export function SkeletonCard() {
  return (
    <div className="card">
      <div className="skeleton skeleton-text" style={{ width: 80 }} />
      <div className="skeleton skeleton-value" style={{ width: 100, marginTop: 8 }} />
      <div className="skeleton skeleton-text" style={{ width: 60, marginTop: 8 }} />
    </div>
  );
}
