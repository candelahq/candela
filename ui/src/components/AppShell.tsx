"use client";

import { useState, useEffect } from "react";
import { usePathname } from "next/navigation";
import { Sidebar } from "@/components/sidebar";
import { BudgetAlert } from "@/components/BudgetAlert";

/** Renders the app layout with sidebar for authenticated pages,
 *  or just the children for the login page. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  // Auto-close sidebar on route change
  useEffect(() => {
    setSidebarOpen(false);
  }, [pathname]);

  if (pathname === "/login") {
    return <>{children}</>;
  }

  return (
    <div className="app-layout">
      <BudgetAlert />
      <div
        className={`sidebar-backdrop ${sidebarOpen ? "open" : ""}`}
        onClick={() => setSidebarOpen(false)}
        aria-hidden={!sidebarOpen}
      />
      <Sidebar isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} />
      <main className="main-content">
        <div className="mobile-header">
          <button
            type="button"
            className="mobile-menu-btn"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open navigation menu"
          >
            ☰
          </button>
          <div className="mobile-brand">
            <span className="mobile-brand-icon">🕯</span>
            <span className="mobile-brand-name">Candela</span>
          </div>
          <div className="mobile-header-spacer" />
        </div>
        {children}
      </main>
    </div>
  );
}
