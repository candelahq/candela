import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/components/AuthProvider";
import { AuthGuard } from "@/components/AuthGuard";
import { AppShell } from "@/components/AppShell";
import { UserScopeProvider } from "@/components/UserScopeProvider";

export const metadata: Metadata = {
  title: "Candela — LLM Observability",
  description: "Open-source LLM observability platform. Monitor costs, latency, and quality across all your AI providers.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <AuthProvider>
          <AuthGuard>
            <UserScopeProvider>
              <AppShell>{children}</AppShell>
            </UserScopeProvider>
          </AuthGuard>
        </AuthProvider>
      </body>
    </html>
  );
}
