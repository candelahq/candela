"use client";

import { usePathname } from "next/navigation";
import { Sidebar } from "@/components/sidebar";
import { BudgetAlert } from "@/components/BudgetAlert";

/** Renders the app layout with sidebar for authenticated pages,
 *  or just the children for the login page. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  if (pathname === "/login") {
    return <>{children}</>;
  }

  return (
    <div className="app-layout">
      <BudgetAlert />
      <Sidebar />
      <main className="main-content">{children}</main>
    </div>
  );
}
