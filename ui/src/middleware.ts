import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

interface DecodedToken {
  sub?: string;
  email?: string;
  role?: string | number;
  admin?: boolean;
  candela_role?: string;
  exp?: number;
  [key: string]: unknown;
}

/**
 * Parses JWT payload without third-party dependencies for Next.js edge/server runtime.
 */
export function parseJwtPayload(token: string): DecodedToken | null {
  try {
    const parts = token.split(".");
    if (parts.length < 2) return null;
    const base64Url = parts[1];
    const base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
    const jsonPayload = atob(padded);
    return JSON.parse(jsonPayload);
  } catch {
    return null;
  }
}

/**
 * Returns a standalone 403 Forbidden HTML response with Access Denied notice.
 * Crucially, no Next.js admin page React components or client JavaScript bundles are shipped.
 */
export function renderAccessDenied(): NextResponse {
  const html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Access Denied | Candela</title>
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <style>
    body {
      margin: 0;
      background-color: #0b0f19;
      color: #f3f4f6;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
    }
    .admin-guard {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      text-align: center;
      padding: 2.5rem;
      background: #111827;
      border: 1px solid #1f2937;
      border-radius: 0.75rem;
      max-width: 480px;
      width: 90%;
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    }
    .admin-guard-icon {
      font-size: 3rem;
      margin-bottom: 1rem;
    }
    h2 {
      font-size: 1.5rem;
      font-weight: 600;
      margin: 0 0 0.75rem;
      color: #f9fafb;
    }
    .admin-guard-message {
      color: #9ca3af;
      margin: 0 0 0.5rem;
      font-size: 0.95rem;
    }
    .admin-guard-hint {
      color: #6b7280;
      font-size: 0.85rem;
      margin: 0 0 1.5rem;
    }
    .admin-guard-link {
      color: #3b82f6;
      text-decoration: none;
      font-size: 0.9rem;
    }
    .admin-guard-link:hover {
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <div class="admin-guard">
    <div class="admin-guard-icon">🔒</div>
    <h2>Access Denied</h2>
    <p class="admin-guard-message">You need admin privileges to access this area.</p>
    <p class="admin-guard-hint">Contact an administrator to request access.</p>
    <a href="/" class="admin-guard-link">Return to Dashboard</a>
  </div>
</body>
</html>`;

  return new NextResponse(html, {
    status: 403,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "x-candela-guard": "access-denied",
    },
  });
}

/**
 * Server-side Route Guard Middleware.
 * Protects /admin routes before Next.js page components or JS bundles are served.
 */
export async function middleware(request: NextRequest): Promise<NextResponse> {
  const { pathname } = request.nextUrl;

  // Only guard /admin routes
  if (!pathname.startsWith("/admin")) {
    return NextResponse.next();
  }

  const sessionCookie = request.cookies.get("__session")?.value;
  const roleCookie = request.cookies.get("candela_role")?.value;
  const authHeader = request.headers.get("authorization");
  const bearerToken = authHeader?.startsWith("Bearer ")
    ? authHeader.slice(7)
    : null;

  const token = sessionCookie || bearerToken;

  const isTestOrDevWithoutFirebase =
    process.env.NODE_ENV === "test" ||
    !process.env.NEXT_PUBLIC_FIREBASE_API_KEY;

  if (isTestOrDevWithoutFirebase) {
    // In test or local dev without Firebase, check explicit admin indicator
    if (roleCookie === "admin") {
      return NextResponse.next();
    }

    if (token) {
      const claims = parseJwtPayload(token);
      if (
        claims &&
        (claims.role === "admin" ||
          claims.role === 2 ||
          claims.admin === true ||
          claims.candela_role === "admin")
      ) {
        return NextResponse.next();
      }
    }

    // Explicit non-admin or unauthenticated
    return renderAccessDenied();
  }

  // Production with Firebase configured: require valid token
  if (!token) {
    const loginUrl = new URL("/login", request.url);
    loginUrl.searchParams.set("from", pathname);
    return NextResponse.redirect(loginUrl);
  }

  const claims = parseJwtPayload(token);
  if (!claims) {
    const loginUrl = new URL("/login", request.url);
    loginUrl.searchParams.set("from", pathname);
    return NextResponse.redirect(loginUrl);
  }

  // Reject expired tokens
  if (claims.exp && claims.exp * 1000 < Date.now()) {
    const loginUrl = new URL("/login", request.url);
    loginUrl.searchParams.set("from", pathname);
    return NextResponse.redirect(loginUrl);
  }

  // Check role from token claims first
  let isAdmin =
    claims.role === "admin" ||
    claims.admin === true ||
    claims.candela_role === "admin" ||
    claims.role === 2;

  // If not in token claims, verify against the backend service
  if (!isAdmin) {
    const backendUrl = process.env.BACKEND_URL || "http://localhost:8080";
    try {
      const res = await fetch(
        `${backendUrl}/candela.v1.UserService/GetCurrentUser`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${token}`,
            "Content-Type": "application/json",
          },
          body: "{}",
        },
      );
      if (res.ok) {
        const data = await res.json();
        const role = data?.user?.role;
        isAdmin =
          role === "USER_ROLE_ADMIN" ||
          role === 2 ||
          role === "admin";
      }
    } catch {
      // Backend unreachable: fail-closed
      isAdmin = false;
    }
  }

  if (!isAdmin) {
    return renderAccessDenied();
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/admin/:path*"],
};
