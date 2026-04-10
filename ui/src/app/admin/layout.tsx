"use client";

import { useCurrentUser } from "@/hooks/useCurrentUser";

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isAdmin, isLoading, error } = useCurrentUser();

  if (isLoading) {
    return (
      <div className="admin-guard">
        <div className="admin-guard-spinner" />
        <p>Loading...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="admin-guard">
        <div className="admin-guard-icon">⚠️</div>
        <h2>Authentication Error</h2>
        <p className="admin-guard-message">{error}</p>
      </div>
    );
  }

  if (!isAdmin) {
    return (
      <div className="admin-guard">
        <div className="admin-guard-icon">🔒</div>
        <h2>Access Denied</h2>
        <p className="admin-guard-message">
          You need admin privileges to access this area.
        </p>
        <p className="admin-guard-hint">
          Contact an administrator to request access.
        </p>
      </div>
    );
  }

  return (
    <div className="admin-container">
      <div className="admin-header">
        <h1 className="admin-title">Administration</h1>
      </div>
      {children}
    </div>
  );
}
