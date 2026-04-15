"use client";

import { usePathname } from "next/navigation";
import { Sidebar } from "@/components/sidebar";

/** Renders the app layout with sidebar for authenticated pages,
 *  or just the children for the login page. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  if (pathname === "/login") {
    return <>{children}</>;
  }

  return (
    <div className="app-layout">
      <Sidebar />
      <main className="main-content">{children}</main>
    </div>
  );
}
