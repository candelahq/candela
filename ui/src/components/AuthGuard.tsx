"use client";

import { useAuth } from "@/components/AuthProvider";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";

/** Redirects unauthenticated users to /login. Bypasses when Firebase is not configured. */
export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { user, loading, configured } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    // Skip redirect when Firebase isn't configured (local dev without env vars).
    if (!configured) return;
    if (!loading && !user && pathname !== "/login") {
      router.replace("/login");
    }
  }, [user, loading, configured, pathname, router]);

  if (loading) {
    return (
      <div className="auth-loading">
        <div className="spinner" />
      </div>
    );
  }

  // Firebase not configured — render everything without auth (dev mode).
  if (!configured) {
    return <>{children}</>;
  }

  // On /login page, always render children (the login page handles its own redirect).
  if (pathname === "/login") {
    return <>{children}</>;
  }

  // On other pages, only render if authenticated.
  if (!user) {
    return null;
  }

  return <>{children}</>;
}
