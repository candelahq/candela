import type { Metadata, Viewport } from "next";
import "./globals.css";
import { AuthProvider } from "@/components/AuthProvider";
import { AuthGuard } from "@/components/AuthGuard";
import { AppShell } from "@/components/AppShell";
import { UserScopeProvider } from "@/components/UserScopeProvider";
import { ToastProvider } from "@/components/Toast";

export const metadata: Metadata = {
  title: "Candela — LLM Observability",
  description: "Open-source LLM observability platform. Monitor costs, latency, and quality across all your AI providers.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <ToastProvider>
          <AuthProvider>
            <AuthGuard>
              <UserScopeProvider>
                <AppShell>{children}</AppShell>
              </UserScopeProvider>
            </AuthGuard>
          </AuthProvider>
        </ToastProvider>
      </body>
    </html>
  );
}
