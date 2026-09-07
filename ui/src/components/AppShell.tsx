"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { Sidebar } from "@/components/sidebar";
import { BudgetAlert } from "@/components/BudgetAlert";

/** Renders the app layout with sidebar for authenticated pages,
 *  or just the children for the login page. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [prevPathname, setPrevPathname] = useState(pathname);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  // Auto-close sidebar on route change without triggering cascading renders in effect
  if (pathname !== prevPathname) {
    setPrevPathname(pathname);
    setSidebarOpen(false);
  }

  // Handle escape key to close drawer and restore focus
  useEffect(() => {
    if (!sidebarOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setSidebarOpen(false);
        menuButtonRef.current?.focus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [sidebarOpen]);

  if (pathname === "/login") {
    return <>{children}</>;
  }

  return (
    <div className="app-layout">
      <BudgetAlert />
      <div
        className={`sidebar-backdrop ${sidebarOpen ? "open" : ""}`}
        onClick={() => {
          setSidebarOpen(false);
          menuButtonRef.current?.focus();
        }}
        aria-hidden={!sidebarOpen}
      />
      <Sidebar
        isOpen={sidebarOpen}
        onClose={() => {
          setSidebarOpen(false);
          menuButtonRef.current?.focus();
        }}
      />
      <main className="main-content">
        <div className="mobile-header">
          <button
            ref={menuButtonRef}
            type="button"
            className="mobile-menu-btn"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open navigation menu"
            aria-expanded={sidebarOpen}
            aria-controls="app-sidebar"
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
